// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/pkg/constants"
)

func TestPythonPlanAppliesLanguageAndCommonSDK(t *testing.T) {
	ctx := context.Background()
	ns := corev1.Namespace{}
	ns.Name = "workloads"
	pod := corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name:  "app",
		Image: "example/app:1.2.3",
		Env:   []corev1.EnvVar{{Name: "PYTHONPATH", Value: "/user/site-packages"}},
	}}}}
	pod.Name = "app-pod"

	inst := v1alpha1.Instrumentation{}
	inst.Spec.Python.Image = "ghcr.io/example/python-agent:latest"
	inst.Spec.Exporter.Endpoint = "http://collector:4318"
	inst.Spec.Propagators = []v1alpha1.Propagator{"tracecontext", "baggage"}
	inst.Spec.Sampler.Type = "parentbased_always_on"

	builder := NewPlanBuilder(ctx, NewSDKCommon(nil, logr.Discard()), ns, pod)
	target := ContainerRef{Kind: RegularContainer, Index: 0, Name: "app"}
	err := (Python{}).Plan(ctx, builder, Request{Instrumentation: inst, Namespace: ns, Pod: pod}, Target{App: target, Agent: target})
	require.NoError(t, err)

	require.NoError(t, builder.Plan().Apply(&pod))

	container := pod.Spec.Containers[0]
	assert.Equal(t,
		"/otel-auto-instrumentation-python/opentelemetry/instrumentation/auto_instrumentation:/user/site-packages:/otel-auto-instrumentation-python",
		envValue(container.Env, "PYTHONPATH"),
	)
	assert.Equal(t, "http/protobuf", envValue(container.Env, "OTEL_EXPORTER_OTLP_PROTOCOL"))
	assert.Equal(t, "http://collector:4318", envValue(container.Env, constants.EnvOTELExporterOTLPEndpoint))
	assert.Equal(t, "app-pod", envValue(container.Env, constants.EnvOTELServiceName))
	assert.Equal(t, "tracecontext,baggage", envValue(container.Env, constants.EnvOTELPropagators))
	assert.Equal(t, "parentbased_always_on", envValue(container.Env, constants.EnvOTELTracesSampler))
	assert.Equal(t, "$(OTEL_RESOURCE_ATTRIBUTES_POD_NAME)", envResourceValue(container.Env, "k8s.pod.name"))

	require.Len(t, pod.Spec.InitContainers, 1)
	assert.Equal(t, "opentelemetry-auto-instrumentation-python", pod.Spec.InitContainers[0].Name)
	assert.Equal(t, []string{"cp", "-r", "/autoinstrumentation/.", "/otel-auto-instrumentation-python"}, pod.Spec.InitContainers[0].Command)
	require.Len(t, pod.Spec.Volumes, 1)
	require.NotNil(t, pod.Spec.Volumes[0].EmptyDir)
	require.Len(t, container.VolumeMounts, 1)
	assert.Equal(t, "/otel-auto-instrumentation-python", container.VolumeMounts[0].MountPath)
}

func TestPythonPlanInsertsAgentBeforeTargetInitContainer(t *testing.T) {
	ctx := context.Background()
	ns := corev1.Namespace{}
	pod := corev1.Pod{Spec: corev1.PodSpec{
		InitContainers: []corev1.Container{{Name: "init-app"}},
		Containers:     []corev1.Container{{Name: "app"}},
	}}
	inst := v1alpha1.Instrumentation{}

	builder := NewPlanBuilder(ctx, NewSDKCommon(nil, logr.Discard()), ns, pod)
	target := ContainerRef{Kind: InitContainer, Index: 0, Name: "init-app"}
	err := (Python{}).Plan(ctx, builder, Request{Instrumentation: inst, Namespace: ns, Pod: pod}, Target{App: target, Agent: target})
	require.NoError(t, err)
	require.NoError(t, builder.Plan().Apply(&pod))

	assert.Equal(t, []string{"opentelemetry-auto-instrumentation-python", "init-app"}, containerNames(pod.Spec.InitContainers))
	assert.Equal(t, "/otel-auto-instrumentation-python/opentelemetry/instrumentation/auto_instrumentation:/otel-auto-instrumentation-python", envValue(pod.Spec.InitContainers[1].Env, "PYTHONPATH"))
}

func TestPythonPlanRejectsUnknownPlatform(t *testing.T) {
	ctx := context.Background()
	pod := corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}}
	builder := NewPlanBuilder(ctx, NewSDKCommon(nil, logr.Discard()), corev1.Namespace{}, pod)
	target := ContainerRef{Kind: RegularContainer, Index: 0, Name: "app"}

	err := (Python{Platform: "windows"}).Plan(ctx, builder, Request{Instrumentation: v1alpha1.Instrumentation{}, Pod: pod}, Target{App: target, Agent: target})
	require.Error(t, err)
}

func envValue(envs []corev1.EnvVar, name string) string {
	for _, e := range envs {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

func envResourceValue(envs []corev1.EnvVar, name string) string {
	attrs := envValue(envs, constants.EnvOTELResourceAttrs)
	for kv := range strings.SplitSeq(attrs, ",") {
		key, value, ok := strings.Cut(kv, "=")
		if ok && key == name {
			return value
		}
	}
	return ""
}

func containerNames(containers []corev1.Container) []string {
	names := make([]string, len(containers))
	for i, c := range containers {
		names[i] = c.Name
	}
	return names
}
