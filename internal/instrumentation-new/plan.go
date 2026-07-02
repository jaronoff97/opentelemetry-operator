// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

type MutationPlan struct {
	containers     map[ContainerRef]*ContainerPatch
	Volumes        []corev1.Volume
	InitContainers []InitContainerPatch
	AddContainers  []corev1.Container
}

type InitContainerPatch struct {
	Container corev1.Container
	Before    ContainerRef
}

func (p *MutationPlan) Container(ref ContainerRef) *ContainerPatch {
	if p.containers == nil {
		p.containers = make(map[ContainerRef]*ContainerPatch)
	}
	cp, ok := p.containers[ref]
	if !ok {
		cp = &ContainerPatch{}
		p.containers[ref] = cp
	}
	return cp
}

func (p *MutationPlan) AddVolume(volume corev1.Volume) {
	p.Volumes = append(p.Volumes, volume)
}

func (p *MutationPlan) AddInitContainer(init InitContainerPatch) {
	p.InitContainers = append(p.InitContainers, init)
}

func (p *MutationPlan) AddContainer(container corev1.Container) ContainerRef {
	ref := ContainerRef{Kind: AddedContainer, Index: len(p.AddContainers), Name: container.Name}
	p.AddContainers = append(p.AddContainers, container)
	return ref
}

func (p *MutationPlan) Merge(other MutationPlan) {
	for ref, cp := range other.containers {
		p.Container(ref).merge(*cp)
	}
	p.Volumes = append(p.Volumes, other.Volumes...)
	p.InitContainers = append(p.InitContainers, other.InitContainers...)
	p.AddContainers = append(p.AddContainers, other.AddContainers...)
}

func (p MutationPlan) Apply(pod *corev1.Pod) error {
	appendAddedContainers(pod, p.AddContainers)
	appendVolumes(pod, p.Volumes)
	appendInitContainers(pod, p.InitContainers)

	for ref, cp := range p.containers {
		container := resolveContainer(pod, ref)
		if container == nil {
			continue
		}
		env, err := applyEnvOps(container.Env, cp.envOps)
		if err != nil {
			return err
		}
		container.Env = env
		container.VolumeMounts = appendVolumeMounts(container.VolumeMounts, cp.VolumeMounts)
	}
	return nil
}

func appendAddedContainers(pod *corev1.Pod, containers []corev1.Container) {
	existing := make(map[string]struct{}, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		existing[c.Name] = struct{}{}
	}
	for _, c := range containers {
		if _, ok := existing[c.Name]; ok {
			continue
		}
		existing[c.Name] = struct{}{}
		pod.Spec.Containers = append(pod.Spec.Containers, c)
	}
}

func appendVolumes(pod *corev1.Pod, volumes []corev1.Volume) {
	existing := make(map[string]struct{}, len(pod.Spec.Volumes))
	for _, v := range pod.Spec.Volumes {
		existing[v.Name] = struct{}{}
	}
	for _, v := range volumes {
		if _, ok := existing[v.Name]; ok {
			continue
		}
		existing[v.Name] = struct{}{}
		pod.Spec.Volumes = append(pod.Spec.Volumes, v)
	}
}

func appendInitContainers(pod *corev1.Pod, inits []InitContainerPatch) {
	existing := make(map[string]struct{}, len(pod.Spec.InitContainers))
	for _, init := range pod.Spec.InitContainers {
		existing[init.Name] = struct{}{}
	}
	for _, init := range inits {
		if _, ok := existing[init.Container.Name]; ok {
			continue
		}
		existing[init.Container.Name] = struct{}{}
		pod.Spec.InitContainers = insertInitContainer(pod, init.Container, init.Before)
	}
}

func appendVolumeMounts(existing []corev1.VolumeMount, additions []corev1.VolumeMount) []corev1.VolumeMount {
	seen := make(map[string]struct{}, len(existing))
	for _, mount := range existing {
		seen[mount.Name+"\x00"+mount.MountPath] = struct{}{}
	}
	for _, mount := range additions {
		key := mount.Name + "\x00" + mount.MountPath
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		existing = append(existing, mount)
	}
	return existing
}

func resolveContainer(pod *corev1.Pod, ref ContainerRef) *corev1.Container {
	switch ref.Kind {
	case RegularContainer:
		if ref.Index >= 0 && ref.Index < len(pod.Spec.Containers) && pod.Spec.Containers[ref.Index].Name == ref.Name {
			return &pod.Spec.Containers[ref.Index]
		}
	case InitContainer:
		if ref.Index >= 0 && ref.Index < len(pod.Spec.InitContainers) && pod.Spec.InitContainers[ref.Index].Name == ref.Name {
			return &pod.Spec.InitContainers[ref.Index]
		}
	}

	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == ref.Name {
			return &pod.Spec.Containers[i]
		}
	}
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == ref.Name {
			return &pod.Spec.InitContainers[i]
		}
	}
	return nil
}

func insertInitContainer(pod *corev1.Pod, toInsert corev1.Container, before ContainerRef) []corev1.Container {
	if before.Kind != InitContainer {
		return append(pod.Spec.InitContainers, toInsert)
	}
	for i, c := range pod.Spec.InitContainers {
		if c.Name == before.Name {
			pod.Spec.InitContainers = append(pod.Spec.InitContainers[:i+1], pod.Spec.InitContainers[i:]...)
			pod.Spec.InitContainers[i] = toInsert
			return pod.Spec.InitContainers
		}
	}
	return append(pod.Spec.InitContainers, toInsert)
}

type PlanBuilder struct {
	ctx       context.Context
	index     PodIndex
	namespace corev1.Namespace
	pod       corev1.Pod
	plan      MutationPlan
	sdk       *SDKCommon
}

func NewPlanBuilder(ctx context.Context, sdk *SDKCommon, namespace corev1.Namespace, pod corev1.Pod) *PlanBuilder {
	return &PlanBuilder{
		ctx:       ctx,
		index:     NewPodIndex(pod),
		namespace: namespace,
		pod:       pod,
		sdk:       sdk,
	}
}

func (b *PlanBuilder) Plan() MutationPlan {
	return b.plan
}

func (b *PlanBuilder) Container(ref ContainerRef) *ContainerPatch {
	return b.plan.Container(ref)
}

func (b *PlanBuilder) AddVolume(volume corev1.Volume) {
	b.plan.AddVolume(volume)
}

func (b *PlanBuilder) AddInitContainer(init InitContainerPatch) {
	b.plan.AddInitContainer(init)
}

func (b *PlanBuilder) ContainerSnapshot(ref ContainerRef) (corev1.Container, bool) {
	return b.index.Container(ref)
}

func (b *PlanBuilder) CommonSDK(inst v1alpha1Instrumentation, agent, app ContainerRef) {
	if b.sdk == nil {
		return
	}
	agentContainer, ok := b.index.Container(agent)
	if !ok && agent.Kind == AddedContainer {
		for _, c := range b.plan.AddContainers {
			if c.Name == agent.Name {
				agentContainer = c
				ok = true
				break
			}
		}
	}
	appContainer, appOK := b.index.Container(app)
	if !ok || !appOK {
		return
	}
	b.sdk.Add(b.ctx, &b.plan, SDKConfigInput{
		Instrumentation: inst,
		Namespace:       b.namespace,
		Pod:             b.pod,
		Agent:           agent,
		App:             app,
		AgentContainer:  agentContainer,
		AppContainer:    appContainer,
	})
}
