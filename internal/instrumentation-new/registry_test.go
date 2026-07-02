// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-telemetry/opentelemetry-operator/internal/config"
)

func TestDefaultRegistryHasPythonDescriptor(t *testing.T) {
	registry := DefaultRegistry()
	assert.Len(t, registry, 1)
	assert.Equal(t, pythonLanguage, registry[0].Name)
	assert.False(t, registry[0].IsEnabled(config.Config{}))
	assert.True(t, registry[0].IsEnabled(config.Config{EnablePythonAutoInstrumentation: true}))
}

func TestEffectiveAnnotationValue(t *testing.T) {
	ns := metav1.ObjectMeta{Annotations: map[string]string{AnnotationInjectPython: "default-inst"}}
	pod := metav1.ObjectMeta{Annotations: map[string]string{AnnotationInjectPython: "true"}}

	assert.Equal(t, "default-inst", EffectiveAnnotationValue(ns, pod, AnnotationInjectPython))

	pod.Annotations[AnnotationInjectPython] = "false"
	assert.Equal(t, "false", EffectiveAnnotationValue(ns, pod, AnnotationInjectPython))
}
