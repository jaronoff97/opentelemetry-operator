// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
	"github.com/open-telemetry/opentelemetry-operator/pkg/constants"
)

type SDKCommon struct {
	resources *ResourceCollector
}

type SDKConfigInput struct {
	Instrumentation v1alpha1.Instrumentation
	Namespace       corev1.Namespace
	Pod             corev1.Pod
	Agent           ContainerRef
	App             ContainerRef
	AgentContainer  corev1.Container
	AppContainer    corev1.Container
}

func NewSDKCommon(c client.Client, logger logr.Logger) *SDKCommon {
	return &SDKCommon{resources: NewResourceCollector(c, logger)}
}

func (s *SDKCommon) Add(ctx context.Context, plan *MutationPlan, in SDKConfigInput) {
	cp := plan.Container(in.Agent)
	inst := in.Instrumentation
	useLabels := inst.Spec.Defaults.UseLabelsForResourceAttributes

	cp.AddEnvFirst(constants.EnvPodIP, fieldRef("status.podIP"))
	cp.AddEnvFirst(constants.EnvNodeIP, fieldRef("status.hostIP"))
	cp.AddEnvVars(inst.Spec.Env...)

	resourceAttrs := s.resources.Build(ctx, inst, in.Namespace, in.Pod, in.AppContainer)
	serviceName := chooseServiceName(in.Pod, useLabels, resourceAttrs, in.AppContainer)
	resourceAttrs.Add(ResourceKubernetes, string(semconv.K8SPodNameKey), fmt.Sprintf("$(%s)", constants.EnvPodName))
	if inst.Spec.Resource.AddK8sUIDAttributes && string(in.Pod.UID) == "" {
		cp.AddEnvFromField(constants.EnvPodUID, "metadata.uid")
		resourceAttrs.Add(ResourceKubernetes, string(semconv.K8SPodUIDKey), fmt.Sprintf("$(%s)", constants.EnvPodUID))
	}
	if in.Pod.Spec.NodeName == "" {
		cp.AddEnvFromField(constants.EnvNodeName, "spec.nodeName")
		resourceAttrs.Add(ResourceKubernetes, string(semconv.K8SNodeNameKey), fmt.Sprintf("$(%s)", constants.EnvNodeName))
	}

	cp.AddEnv(constants.EnvOTELServiceName, serviceName)
	addExporterConfig(plan, cp, inst.Spec.Exporter)

	cp.AddEnvFromField(constants.EnvPodName, "metadata.name")
	cp.AddResourceAttributes(resourceAttrs)

	if len(inst.Spec.Propagators) > 0 {
		propagators := make([]string, 0, len(inst.Spec.Propagators))
		for _, p := range inst.Spec.Propagators {
			propagators = append(propagators, string(p))
		}
		cp.AddEnv(constants.EnvOTELPropagators, strings.Join(propagators, ","))
	}

	if inst.Spec.Sampler.Type != "" {
		envs := []corev1.EnvVar{{Name: constants.EnvOTELTracesSampler, Value: string(inst.Spec.Sampler.Type)}}
		if inst.Spec.Sampler.Argument != "" {
			envs = append(envs, corev1.EnvVar{Name: constants.EnvOTELTracesSamplerArg, Value: inst.Spec.Sampler.Argument})
		}
		cp.AddEnvVarsIfAllAbsent([]string{constants.EnvOTELTracesSampler, constants.EnvOTELTracesSamplerArg}, envs...)
	}
}

func fieldRef(fieldPath string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{FieldPath: fieldPath},
	}
}

func addExporterConfig(plan *MutationPlan, cp *ContainerPatch, exporter v1alpha1.Exporter) {
	if exporter.Endpoint != "" {
		cp.AddEnv(constants.EnvOTELExporterOTLPEndpoint, exporter.Endpoint)
	}
	if exporter.TLS == nil {
		return
	}

	secretVolumeName := naming.Truncate("otel-auto-secret-%s", 63, exporter.TLS.SecretName)
	secretMountPath := fmt.Sprintf("/otel-auto-instrumentation-secret-%s", exporter.TLS.SecretName)
	configMapVolumeName := naming.Truncate("otel-auto-configmap-%s", 63, exporter.TLS.ConfigMapName)
	configMapMountPath := fmt.Sprintf("/otel-auto-instrumentation-configmap-%s", exporter.TLS.ConfigMapName)

	if exporter.TLS.CA != "" {
		mountPath := secretMountPath
		if exporter.TLS.ConfigMapName != "" {
			mountPath = configMapMountPath
		}
		cp.AddEnv(constants.EnvOTELExporterCertificate, exporterPath(mountPath, exporter.TLS.CA))
	}
	if exporter.TLS.Cert != "" {
		cp.AddEnv(constants.EnvOTELExporterClientCertificate, exporterPath(secretMountPath, exporter.TLS.Cert))
	}
	if exporter.TLS.Key != "" {
		cp.AddEnv(constants.EnvOTELExporterClientKey, exporterPath(secretMountPath, exporter.TLS.Key))
	}

	if exporter.TLS.SecretName != "" {
		plan.AddVolume(corev1.Volume{
			Name: secretVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: exporter.TLS.SecretName},
			},
		})
		cp.AddVolumeMount(corev1.VolumeMount{
			Name:      secretVolumeName,
			MountPath: secretMountPath,
			ReadOnly:  true,
		})
	}

	if exporter.TLS.ConfigMapName != "" {
		plan.AddVolume(corev1.Volume{
			Name: configMapVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: exporter.TLS.ConfigMapName},
				},
			},
		})
		cp.AddVolumeMount(corev1.VolumeMount{
			Name:      configMapVolumeName,
			MountPath: configMapMountPath,
			ReadOnly:  true,
		})
	}
}

func exporterPath(mountPath, value string) string {
	if filepath.IsAbs(value) {
		return value
	}
	return fmt.Sprintf("%s/%s", mountPath, value)
}
