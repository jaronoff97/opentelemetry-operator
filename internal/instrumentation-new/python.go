// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

const (
	pythonLanguage = "python"

	envPythonPath               = "PYTHONPATH"
	envOtelTracesExporter       = "OTEL_TRACES_EXPORTER"
	envOtelMetricsExporter      = "OTEL_METRICS_EXPORTER"
	envOtelLogsExporter         = "OTEL_LOGS_EXPORTER"
	envOtelExporterOTLPProtocol = "OTEL_EXPORTER_OTLP_PROTOCOL"

	pythonGlibcSrc   = "/autoinstrumentation/."
	pythonMuslSrc    = "/autoinstrumentation-musl/."
	pythonPathPrefix = "/otel-auto-instrumentation-python/opentelemetry/instrumentation/auto_instrumentation"
	pythonPathSuffix = "/otel-auto-instrumentation-python"

	pythonPlatformGlibc = "glibc"
	pythonPlatformMusl  = "musl"
)

type Python struct {
	Platform string
}

func (p Python) Name() string { return pythonLanguage }

func (p Python) Plan(_ context.Context, b *PlanBuilder, req Request, target Target) error {
	src, err := pythonAutoinstrumentationSrc(p.Platform)
	if err != nil {
		return err
	}
	container, ok := b.ContainerSnapshot(target.App)
	if !ok {
		return nil
	}

	cp := b.Container(target.App)
	cp.RejectValueFrom(envPythonPath)
	cp.AddEnvVars(req.Instrumentation.Spec.Python.Env...)
	cp.AddEnvVars(defaultPythonEnvVars()...)
	cp.PrependEnvValueWithSep(envPythonPath, pythonPathPrefix, ":")
	cp.AppendEnvValueWithSep(envPythonPath, pythonPathSuffix, ":")
	cp.AddVolumeMount(AgentVolumeMount(pythonLanguage))

	b.AddVolume(AgentVolume(
		pythonLanguage,
		req.Instrumentation.Spec.Python.VolumeClaimTemplate,
		req.Instrumentation.Spec.Python.VolumeSizeLimit,
	))

	b.AddInitContainer(InitContainerPatch{
		Before: target.App,
		Container: corev1.Container{
			Name:      InitContainerName(pythonLanguage),
			Image:     req.Instrumentation.Spec.Python.Image,
			Command:   []string{"cp", "-r", src, MountPath(pythonLanguage)},
			Resources: req.Instrumentation.Spec.Python.Resources,
			VolumeMounts: []corev1.VolumeMount{{
				Name:      VolumeName(pythonLanguage),
				MountPath: MountPath(pythonLanguage),
			}},
			SecurityContext: resolveInitContainerSecurityContext(req.Instrumentation.Spec.InitContainerSecurityContext, container.SecurityContext),
			ImagePullPolicy: req.Instrumentation.Spec.ImagePullPolicy,
		},
	})

	agent := target.Agent
	if agent.Name == "" {
		agent = target.App
	}
	b.CommonSDK(req.Instrumentation, agent, target.App)
	return nil
}

func pythonAutoinstrumentationSrc(platform string) (string, error) {
	switch platform {
	case "", pythonPlatformGlibc:
		return pythonGlibcSrc, nil
	case pythonPlatformMusl:
		return pythonMuslSrc, nil
	default:
		return "", fmt.Errorf("provided instrumentation.opentelemetry.io/otel-python-platform annotation value '%s' is not supported", platform)
	}
}

func defaultPythonEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: envOtelExporterOTLPProtocol, Value: "http/protobuf"},
		{Name: envOtelTracesExporter, Value: "otlp"},
		{Name: envOtelMetricsExporter, Value: "otlp"},
		{Name: envOtelLogsExporter, Value: "otlp"},
	}
}

func resolveInitContainerSecurityContext(specSecurityContext, containerSecurityContext *corev1.SecurityContext) *corev1.SecurityContext {
	if specSecurityContext != nil {
		return specSecurityContext
	}
	return containerSecurityContext
}
