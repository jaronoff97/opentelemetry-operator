// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestEnvOpsKeepSliceWorkInApply(t *testing.T) {
	pod := corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name: "app",
		Env: []corev1.EnvVar{
			{Name: "PYTHONPATH", Value: "/user"},
			{Name: "OTEL_RESOURCE_ATTRIBUTES", Value: "service.namespace=user"},
		},
	}}}}
	ref := ContainerRef{Kind: RegularContainer, Index: 0, Name: "app"}

	var attrs ResourceAttributes
	attrs.Add(ResourceCR, "service.namespace", "cr")
	attrs.Add(ResourceKubernetes, "k8s.pod.name", "$(POD_NAME)")

	var plan MutationPlan
	cp := plan.Container(ref)
	cp.AddEnvFromField("POD_NAME", "metadata.name")
	cp.PrependEnvValueWithSep("PYTHONPATH", "/prefix", ":")
	cp.AppendEnvValueWithSep("PYTHONPATH", "/suffix", ":")
	cp.AddResourceAttributes(attrs)

	require.NoError(t, plan.Apply(&pod))

	env := pod.Spec.Containers[0].Env
	assert.Equal(t, "/prefix:/user:/suffix", envValue(env, "PYTHONPATH"))
	assert.Equal(t, "service.namespace=user,k8s.pod.name=$(POD_NAME)", envValue(env, "OTEL_RESOURCE_ATTRIBUTES"))
	assert.Equal(t, "OTEL_RESOURCE_ATTRIBUTES", env[len(env)-1].Name)
}

func TestEnvOpsRejectValueFromAtApply(t *testing.T) {
	pod := corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name: "app",
		Env: []corev1.EnvVar{{
			Name: "PYTHONPATH",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
			},
		}},
	}}}}
	ref := ContainerRef{Kind: RegularContainer, Index: 0, Name: "app"}

	var plan MutationPlan
	plan.Container(ref).RejectValueFrom("PYTHONPATH")

	err := plan.Apply(&pod)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrEnvFromValueFrom))
}

func TestEnvSetIfAllAbsent(t *testing.T) {
	pod := corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name: "app",
		Env:  []corev1.EnvVar{{Name: "OTEL_TRACES_SAMPLER_ARG", Value: "0.5"}},
	}}}}
	ref := ContainerRef{Kind: RegularContainer, Index: 0, Name: "app"}

	var plan MutationPlan
	plan.Container(ref).AddEnvVarsIfAllAbsent(
		[]string{"OTEL_TRACES_SAMPLER", "OTEL_TRACES_SAMPLER_ARG"},
		corev1.EnvVar{Name: "OTEL_TRACES_SAMPLER", Value: "parentbased_always_on"},
	)

	require.NoError(t, plan.Apply(&pod))
	assert.Empty(t, envValue(pod.Spec.Containers[0].Env, "OTEL_TRACES_SAMPLER"))
}
