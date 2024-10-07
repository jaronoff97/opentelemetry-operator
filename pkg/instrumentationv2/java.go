// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package instrumentationv2

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/internal/config"
)

const (
	envJavaToolsOptions = "JAVA_TOOL_OPTIONS"
	javaAgent           = " -javaagent:/otel-auto-instrumentation-java/javaagent.jar"
)

type java struct {
	cfg config.Config
}

func newJava(cfg config.Config) *java {
	return &java{cfg: cfg}
}

func (j java) Name() string {
	return "java"
}

func (j java) IsSidecar() bool {
	return false
}

func (j java) Enabled() bool {
	return j.cfg.EnableJavaAutoInstrumentation()
}

func (j java) Inject(logger logr.Logger, ns corev1.Namespace, pod corev1.Pod, index int, inst *v1alpha1.Instrumentation) (corev1.Pod, error) {
	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[index]

	err := validateContainerEnv(container.Env, envJavaToolsOptions)
	if err != nil {
		return pod, err
	}

	// inject Java instrumentation spec env vars.
	for _, env := range inst.Spec.Java.Env {
		idx := getIndexOfEnv(container.Env, env.Name)
		if idx == -1 {
			container.Env = append(container.Env, env)
		}
	}

	javaJVMArgument := javaAgent
	if len(inst.Spec.Java.Extensions) > 0 {
		javaJVMArgument = javaAgent + fmt.Sprintf(" -Dotel.javaagent.extensions=%s/extensions", mountPath(j))
	}

	idx := getIndexOfEnv(container.Env, envJavaToolsOptions)
	if idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envJavaToolsOptions,
			Value: javaJVMArgument,
		})
	} else {
		container.Env[idx].Value = container.Env[idx].Value + javaJVMArgument
	}

	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      prefixedName(j),
		MountPath: mountPath(j),
	})

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, prefixedName(j)) {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: prefixedName(j),
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: volumeSize(inst.Spec.Java.VolumeSizeLimit),
				},
			}})

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name:      prefixedName(j),
			Image:     inst.Spec.Java.Image,
			Command:   []string{"cp", "/javaagent.jar", mountPath(j) + "/javaagent.jar"},
			Resources: inst.Spec.Java.Resources,
			VolumeMounts: []corev1.VolumeMount{{
				Name:      prefixedName(j),
				MountPath: mountPath(j),
			}},
		})

		for i, extension := range inst.Spec.Java.Extensions {
			pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
				Name:      prefixedName(j) + fmt.Sprintf("-extension-%d", i),
				Image:     extension.Image,
				Command:   []string{"cp", "-r", extension.Dir + "/.", mountPath(j) + "/extensions"},
				Resources: inst.Spec.Java.Resources,
				VolumeMounts: []corev1.VolumeMount{{
					Name:      prefixedName(j),
					MountPath: mountPath(j),
				}},
			})
		}

	}
	return pod, err
}

var _ Language = &java{}
