// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-telemetry/opentelemetry-operator/internal/config"
)

const (
	AnnotationInjectContainerName        = "instrumentation.opentelemetry.io/container-names"
	AnnotationInjectPython               = "instrumentation.opentelemetry.io/inject-python"
	AnnotationInjectPythonContainersName = "instrumentation.opentelemetry.io/python-container-names"
	AnnotationPythonPlatform             = "instrumentation.opentelemetry.io/otel-python-platform"
)

type LanguageDescriptor struct {
	Name                 string
	InjectAnnotation     string
	ContainersAnnotation string
	ExtraAnnotations     []string
	Enabled              func(config.Config) bool
	NewInjector          func(config.Config, map[string]string) LanguagePlanner
}

func (d LanguageDescriptor) IsEnabled(cfg config.Config) bool {
	if d.Enabled == nil {
		return true
	}
	return d.Enabled(cfg)
}

func DefaultRegistry() []LanguageDescriptor {
	return []LanguageDescriptor{
		{
			Name:                 pythonLanguage,
			InjectAnnotation:     AnnotationInjectPython,
			ContainersAnnotation: AnnotationInjectPythonContainersName,
			ExtraAnnotations:     []string{AnnotationPythonPlatform},
			Enabled: func(cfg config.Config) bool {
				return cfg.EnablePythonAutoInstrumentation
			},
			NewInjector: func(_ config.Config, annotations map[string]string) LanguagePlanner {
				return Python{Platform: annotations[AnnotationPythonPlatform]}
			},
		},
	}
}

func EffectiveAnnotationValue(ns, pod metav1.ObjectMeta, annotation string) string {
	podValue := pod.Annotations[annotation]
	nsValue := ns.Annotations[annotation]

	if nsValue == "" {
		return podValue
	}
	if podValue == "" {
		return nsValue
	}
	if !strings.EqualFold(podValue, "true") {
		return podValue
	}
	if strings.EqualFold(nsValue, "false") {
		return podValue
	}
	return nsValue
}
