// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestPodIndexTargetsInitContainersFirst(t *testing.T) {
	pod := corev1.Pod{Spec: corev1.PodSpec{
		InitContainers: []corev1.Container{{Name: "init-b"}, {Name: "init-a"}},
		Containers:     []corev1.Container{{Name: "app-b"}, {Name: "app-a"}},
	}}

	targets := NewPodIndex(pod).Targets([]string{"app-a", "init-a", "app-b", "init-b"})
	require.Len(t, targets, 4)
	assert.Equal(t, []ContainerRef{
		{Kind: InitContainer, Index: 0, Name: "init-b"},
		{Kind: InitContainer, Index: 1, Name: "init-a"},
		{Kind: RegularContainer, Index: 0, Name: "app-b"},
		{Kind: RegularContainer, Index: 1, Name: "app-a"},
	}, targets)
}

func TestPlanAppliesToInitContainerRefs(t *testing.T) {
	pod := corev1.Pod{Spec: corev1.PodSpec{
		InitContainers: []corev1.Container{{Name: "init"}},
		Containers:     []corev1.Container{{Name: "app"}},
	}}
	ref := ContainerRef{Kind: InitContainer, Index: 0, Name: "init"}

	var plan MutationPlan
	plan.Container(ref).AddEnv("A", "B")
	require.NoError(t, plan.Apply(&pod))

	assert.Equal(t, "B", envValue(pod.Spec.InitContainers[0].Env, "A"))
}
