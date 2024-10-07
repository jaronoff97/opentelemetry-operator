package instrumentationv2

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	languagePrefix   = "opentelemetry-auto-instrumentation-%s"
	annotationPrefix = "instrumentation.opentelemetry.io/inject-%s"
	mountPathPrefix  = "/otel-auto-instrumentation-%s"
)

var (
	defaultSize = resource.MustParse("200Mi")
)

func volumeSize(quantity *resource.Quantity) *resource.Quantity {
	if quantity == nil {
		return &defaultSize
	}
	return quantity
}

func multiContainerAnnotationName(l Language) string {
	return fmt.Sprintf("instrumentation.opentelemetry.io/%s-container-names", l.Name())
}

func mountPath(l Language) string {
	return fmt.Sprintf(mountPathPrefix, l.Name())
}

func prefixedName(l Language) string {
	return fmt.Sprintf(languagePrefix, l.Name())
}

func annotationName(l Language) string {
	return fmt.Sprintf(annotationPrefix, l.Name())
}

func getIndexOfEnv(envs []corev1.EnvVar, name string) int {
	for i := range envs {
		if envs[i].Name == name {
			return i
		}
	}
	return -1
}

// Calculate if we already inject InitContainers.
func isInitContainerMissing(pod corev1.Pod, containerName string) bool {
	for _, initContainer := range pod.Spec.InitContainers {
		if initContainer.Name == containerName {
			return false
		}
	}
	return true
}

func validateContainerEnv(envs []corev1.EnvVar, envsToBeValidated ...string) error {
	for _, envToBeValidated := range envsToBeValidated {
		for _, containerEnv := range envs {
			if containerEnv.Name == envToBeValidated {
				if containerEnv.ValueFrom != nil {
					return fmt.Errorf("the container defines env var value via ValueFrom, envVar: %s", containerEnv.Name)
				}
				break
			}
		}
	}
	return nil
}
