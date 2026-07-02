// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
)

type ContainerKind string

const (
	RegularContainer ContainerKind = "container"
	InitContainer    ContainerKind = "initContainer"
	AddedContainer   ContainerKind = "addedContainer"
)

type ContainerRef struct {
	Kind  ContainerKind
	Index int
	Name  string
}

func (r ContainerRef) String() string {
	return fmt.Sprintf("%s/%s[%d]", r.Kind, r.Name, r.Index)
}

type PodIndex struct {
	pod       corev1.Pod
	regularBy map[string]ContainerRef
	initBy    map[string]ContainerRef
}

func NewPodIndex(pod corev1.Pod) PodIndex {
	idx := PodIndex{
		pod:       pod,
		regularBy: make(map[string]ContainerRef, len(pod.Spec.Containers)),
		initBy:    make(map[string]ContainerRef, len(pod.Spec.InitContainers)),
	}
	for i, c := range pod.Spec.Containers {
		idx.regularBy[c.Name] = ContainerRef{Kind: RegularContainer, Index: i, Name: c.Name}
	}
	for i, c := range pod.Spec.InitContainers {
		idx.initBy[c.Name] = ContainerRef{Kind: InitContainer, Index: i, Name: c.Name}
	}
	return idx
}

func (p PodIndex) DefaultTarget() (ContainerRef, bool) {
	if len(p.pod.Spec.Containers) == 0 {
		return ContainerRef{}, false
	}
	c := p.pod.Spec.Containers[0]
	return ContainerRef{Kind: RegularContainer, Index: 0, Name: c.Name}, true
}

func (p PodIndex) Find(name string) (ContainerRef, bool) {
	if ref, ok := p.initBy[name]; ok {
		return ref, true
	}
	ref, ok := p.regularBy[name]
	return ref, ok
}

func (p PodIndex) Container(ref ContainerRef) (corev1.Container, bool) {
	switch ref.Kind {
	case RegularContainer:
		if ref.Index >= 0 && ref.Index < len(p.pod.Spec.Containers) {
			return p.pod.Spec.Containers[ref.Index], true
		}
	case InitContainer:
		if ref.Index >= 0 && ref.Index < len(p.pod.Spec.InitContainers) {
			return p.pod.Spec.InitContainers[ref.Index], true
		}
	}
	return corev1.Container{}, false
}

func (p PodIndex) Targets(names []string) []ContainerRef {
	if len(names) == 0 {
		if ref, ok := p.DefaultTarget(); ok {
			return []ContainerRef{ref}
		}
		return nil
	}

	refs := make([]ContainerRef, 0, len(names))
	for _, name := range names {
		if ref, ok := p.Find(name); ok {
			refs = append(refs, ref)
		}
	}

	slices.SortStableFunc(refs, func(a, b ContainerRef) int {
		if a.Kind == b.Kind {
			return a.Index - b.Index
		}
		if a.Kind == InitContainer {
			return -1
		}
		return 1
	})
	return refs
}
