// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
)

type v1alpha1Instrumentation = v1alpha1.Instrumentation

type Request struct {
	Instrumentation v1alpha1.Instrumentation
	Namespace       corev1.Namespace
	Pod             corev1.Pod
	Annotations     map[string]string
}

type Target struct {
	App   ContainerRef
	Agent ContainerRef
}

type LanguagePlanner interface {
	Name() string
	Plan(context.Context, *PlanBuilder, Request, Target) error
}
