// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	languageInitContainerPrefix = "opentelemetry-auto-instrumentation-%s"
	languageMountPathPrefix     = "/otel-auto-instrumentation-%s"
	languageVolumeName          = "opentelemetry-auto-instrumentation-%s"
)

var defaultVolumeSize = resource.MustParse("200Mi")

func InitContainerName(language string) string {
	return fmt.Sprintf(languageInitContainerPrefix, language)
}

func MountPath(language string) string {
	return fmt.Sprintf(languageMountPathPrefix, language)
}

func VolumeName(language string) string {
	return fmt.Sprintf(languageVolumeName, language)
}

func AgentVolume(language string, claim corev1.PersistentVolumeClaimTemplate, sizeLimit *resource.Quantity) corev1.Volume {
	if !reflect.ValueOf(claim).IsZero() {
		return corev1.Volume{
			Name: VolumeName(language),
			VolumeSource: corev1.VolumeSource{
				Ephemeral: &corev1.EphemeralVolumeSource{
					VolumeClaimTemplate: &claim,
				},
			},
		}
	}
	limit := sizeLimit
	if limit == nil {
		size := defaultVolumeSize
		limit = &size
	}
	return corev1.Volume{
		Name: VolumeName(language),
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: limit},
		},
	}
}

func AgentVolumeMount(language string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      VolumeName(language),
		MountPath: MountPath(language),
	}
}
