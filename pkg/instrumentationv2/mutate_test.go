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

package instrumentationv2_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/internal/config"
	"github.com/open-telemetry/opentelemetry-operator/pkg/constants"
	"github.com/open-telemetry/opentelemetry-operator/pkg/instrumentationv2"
)

func getFakeClient(t *testing.T, lists ...client.ObjectList) client.WithWatch {
	schemeBuilder := runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
		s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.Instrumentation{}, &v1alpha1.InstrumentationList{})
		s.AddKnownTypes(v1.SchemeGroupVersion, &v1.Pod{}, &v1.PodList{})
		s.AddKnownTypes(v1.SchemeGroupVersion, &v1.Namespace{}, &v1.NamespaceList{})
		metav1.AddToGroupVersion(s, v1alpha1.GroupVersion)
		return nil
	})
	scheme := runtime.NewScheme()
	err := schemeBuilder.AddToScheme(scheme)
	require.NoError(t, err, "Should be able to add custom types")
	c := fake.NewClientBuilder().WithLists(lists...).WithScheme(scheme)
	return c.Build()
}

func TestMutator_Mutate(t *testing.T) {
	type fields struct {
		cfg   config.Config
		insts []v1alpha1.Instrumentation
	}
	type args struct {
		ns  v1.Namespace
		pod v1.Pod
	}
	const testNamespace = "test-namespace"
	var defaultConfig = config.New(
		config.WithEnableMultiInstrumentation(true),
		config.WithEnableApacheHttpdInstrumentation(true),
		config.WithEnableDotNetInstrumentation(true),
		config.WithEnableGoInstrumentation(true),
		config.WithEnableNginxInstrumentation(true),
		config.WithEnableJavaInstrumentation(true),
		config.WithEnablePythonInstrumentation(true),
		config.WithEnableNodeJSInstrumentation(true),
	)
	defaultVolumeLimitSize := resource.MustParse("200Mi")
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    v1.Pod
		wantErr bool
	}{
		{
			name: "Pod is already instrumented",
			fields: fields{
				cfg:   config.Config{},
				insts: nil,
			},
			args: args{
				ns: v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}},
				pod: v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "instrumented-pod",
						Namespace: testNamespace, // Using the constant namespace
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{Name: "otel-java"}, // Already instrumented container
						},
					},
				},
			},
			want: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "instrumented-pod", Namespace: testNamespace},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "otel-java"}, // No changes expected
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Pod needs instrumentation, single instance available",
			fields: fields{
				cfg: defaultConfig,
				insts: []v1alpha1.Instrumentation{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "otel-java-instance", Namespace: testNamespace},
						Spec: v1alpha1.InstrumentationSpec{
							Java: v1alpha1.Java{
								Image: "foo/bar:1",
							},
						},
					},
				},
			},
			args: args{
				ns: v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}},
				pod: v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "non-instrumented-pod",
						Namespace:   testNamespace, // Using the constant namespace
						Annotations: map[string]string{"instrumentation.opentelemetry.io/inject-java": "true"},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{Name: "app-container"},
						},
					},
				},
			},
			want: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-instrumented-pod",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"instrumentation.opentelemetry.io/inject-java": "true",
					},
				},
				Spec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							Name: "opentelemetry-auto-instrumentation-java",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: &defaultVolumeLimitSize,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:    "opentelemetry-auto-instrumentation-java",
							Image:   "foo/bar:1",
							Command: []string{"cp", "/javaagent.jar", "/otel-auto-instrumentation-java/javaagent.jar"},
							VolumeMounts: []v1.VolumeMount{{
								Name:      "opentelemetry-auto-instrumentation-java",
								MountPath: "/otel-auto-instrumentation-java",
							}},
						},
					},
					Containers: []v1.Container{
						{
							Name: "app-container",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "opentelemetry-auto-instrumentation-java",
									MountPath: "/otel-auto-instrumentation-java",
								},
							},
							Env: []v1.EnvVar{
								{
									Name: "OTEL_POD_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.podIP",
										},
									},
								},
								{
									Name: "OTEL_NODE_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.hostIP",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_NODE_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "spec.nodeName",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_POD_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "metadata.name",
										},
									},
								},
								{
									Name:  "OTEL_RESOURCE_ATTRIBUTES",
									Value: "k8s.container.name=app-container,k8s.namespace.name=test-namespace,k8s.node.ip=$(OTEL_NODE_IP),k8s.node.name=$(OTEL_RESOURCE_ATTRIBUTES_NODE_NAME),k8s.pod.ip=$(OTEL_POD_IP),k8s.pod.name=non-instrumented-pod",
								},
								{
									Name:  "JAVA_TOOL_OPTIONS",
									Value: " -javaagent:/otel-auto-instrumentation-java/javaagent.jar",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Pod needs java instrumentation, multiple containers",
			fields: fields{
				cfg: defaultConfig,
				insts: []v1alpha1.Instrumentation{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "otel-java-instance", Namespace: testNamespace},
					},
				},
			},
			args: args{
				ns: v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}},
				pod: v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "non-instrumented-pod",
						Namespace: testNamespace, // Using the constant namespace
						Annotations: map[string]string{
							"instrumentation.opentelemetry.io/inject-java":     "true",
							"instrumentation.opentelemetry.io/container-names": "app-container,another-container",
						},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{Name: "app-container"},
							{Name: "another-container"},
						},
					},
				},
			},
			want: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-instrumented-pod",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"instrumentation.opentelemetry.io/container-names": "app-container,another-container",
						"instrumentation.opentelemetry.io/inject-java":     "true",
					},
				},
				Spec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							Name: "opentelemetry-auto-instrumentation-java",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(209715200, resource.BinarySI),
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:  "opentelemetry-auto-instrumentation-java",
							Image: "", // specify image if necessary
							Command: []string{
								"cp",
								"/javaagent.jar",
								"/otel-auto-instrumentation-java/javaagent.jar",
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "opentelemetry-auto-instrumentation-java",
									MountPath: "/otel-auto-instrumentation-java",
								},
							},
						},
					},
					Containers: []v1.Container{
						{
							Name:  "app-container",
							Image: "", // specify image if necessary
							Env: []v1.EnvVar{

								{
									Name: "OTEL_POD_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.podIP",
										},
									},
								},
								{
									Name: "OTEL_NODE_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.hostIP",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_NODE_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "spec.nodeName",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_POD_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "metadata.name",
										},
									},
								},
								{
									Name:  "OTEL_RESOURCE_ATTRIBUTES",
									Value: "k8s.container.name=app-container,k8s.namespace.name=test-namespace,k8s.node.ip=$(OTEL_NODE_IP),k8s.node.name=$(OTEL_RESOURCE_ATTRIBUTES_NODE_NAME),k8s.pod.ip=$(OTEL_POD_IP),k8s.pod.name=non-instrumented-pod",
								},
								{
									Name:  "JAVA_TOOL_OPTIONS",
									Value: " -javaagent:/otel-auto-instrumentation-java/javaagent.jar",
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "opentelemetry-auto-instrumentation-java",
									MountPath: "/otel-auto-instrumentation-java",
								},
							},
						},
						{
							Name:  "another-container",
							Image: "", // specify image if necessary
							Env: []v1.EnvVar{

								{
									Name: "OTEL_POD_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.podIP",
										},
									},
								},
								{
									Name: "OTEL_NODE_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.hostIP",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_NODE_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "spec.nodeName",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_POD_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "metadata.name",
										},
									},
								},
								{
									Name:  "OTEL_RESOURCE_ATTRIBUTES",
									Value: "k8s.container.name=another-container,k8s.namespace.name=test-namespace,k8s.node.ip=$(OTEL_NODE_IP),k8s.node.name=$(OTEL_RESOURCE_ATTRIBUTES_NODE_NAME),k8s.pod.ip=$(OTEL_POD_IP),k8s.pod.name=non-instrumented-pod",
								},
								{
									Name:  "JAVA_TOOL_OPTIONS",
									Value: " -javaagent:/otel-auto-instrumentation-java/javaagent.jar",
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "opentelemetry-auto-instrumentation-java",
									MountPath: "/otel-auto-instrumentation-java",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Pod needs multiple instrumentation, multiple containers",
			fields: fields{
				cfg: defaultConfig,
				insts: []v1alpha1.Instrumentation{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "otel-java-instance", Namespace: testNamespace},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "otel-nodejs-instance", Namespace: testNamespace},
					},
				},
			},
			args: args{
				ns: v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}},
				pod: v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "non-instrumented-pod",
						Namespace: testNamespace, // Using the constant namespace
						Annotations: map[string]string{
							"instrumentation.opentelemetry.io/inject-java":            "otel-java-instance",
							"instrumentation.opentelemetry.io/java-container-names":   "app-container,another-container",
							"instrumentation.opentelemetry.io/inject-nodejs":          "otel-nodejs-instance",
							"instrumentation.opentelemetry.io/nodejs-container-names": "node-container",
						},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{Name: "app-container"},
							{Name: "node-container"},
							{Name: "another-container"},
						},
					},
				},
			},
			want: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-instrumented-pod",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"instrumentation.opentelemetry.io/inject-java":            "otel-java-instance",
						"instrumentation.opentelemetry.io/inject-nodejs":          "otel-nodejs-instance",
						"instrumentation.opentelemetry.io/java-container-names":   "app-container,another-container",
						"instrumentation.opentelemetry.io/nodejs-container-names": "node-container",
					},
				},
				Spec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							Name: "opentelemetry-auto-instrumentation-nodejs",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(209715200, resource.BinarySI),
								},
							},
						},
						{
							Name: "opentelemetry-auto-instrumentation-java",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(209715200, resource.BinarySI),
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:    "opentelemetry-auto-instrumentation-nodejs",
							Command: []string{"cp", "-r", "/autoinstrumentation/.", "/otel-auto-instrumentation-nodejs"},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "opentelemetry-auto-instrumentation-nodejs",
									MountPath: "/otel-auto-instrumentation-nodejs",
								},
							},
						},
						{
							Name:    "opentelemetry-auto-instrumentation-java",
							Command: []string{"cp", "/javaagent.jar", "/otel-auto-instrumentation-java/javaagent.jar"},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "opentelemetry-auto-instrumentation-java",
									MountPath: "/otel-auto-instrumentation-java",
								},
							},
						},
					},
					Containers: []v1.Container{
						{
							Name: "app-container",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "opentelemetry-auto-instrumentation-java",
									MountPath: "/otel-auto-instrumentation-java",
								},
							},
							Env: []v1.EnvVar{
								{
									Name: "OTEL_POD_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.podIP",
										},
									},
								},
								{
									Name: "OTEL_NODE_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.hostIP",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_NODE_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "spec.nodeName",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_POD_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "metadata.name",
										},
									},
								},
								{
									Name:  "OTEL_RESOURCE_ATTRIBUTES",
									Value: "k8s.container.name=app-container,k8s.namespace.name=test-namespace,k8s.node.ip=$(OTEL_NODE_IP),k8s.node.name=$(OTEL_RESOURCE_ATTRIBUTES_NODE_NAME),k8s.pod.ip=$(OTEL_POD_IP),k8s.pod.name=non-instrumented-pod",
								},
								{
									Name:  "JAVA_TOOL_OPTIONS",
									Value: " -javaagent:/otel-auto-instrumentation-java/javaagent.jar",
								},
							},
						},
						{
							Name: "node-container",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "opentelemetry-auto-instrumentation-nodejs",
									MountPath: "/otel-auto-instrumentation-nodejs",
								},
							},
							Env: []v1.EnvVar{
								{
									Name: "OTEL_POD_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.podIP",
										},
									},
								},
								{
									Name: "OTEL_NODE_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.hostIP",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_NODE_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "spec.nodeName",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_POD_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "metadata.name",
										},
									},
								},
								{
									Name:  "OTEL_RESOURCE_ATTRIBUTES",
									Value: "k8s.container.name=node-container,k8s.namespace.name=test-namespace,k8s.node.ip=$(OTEL_NODE_IP),k8s.node.name=$(OTEL_RESOURCE_ATTRIBUTES_NODE_NAME),k8s.pod.ip=$(OTEL_POD_IP),k8s.pod.name=non-instrumented-pod",
								},
								{
									Name:  "NODE_OPTIONS",
									Value: " --require /otel-auto-instrumentation-nodejs/autoinstrumentation.js",
								},
							},
						},
						{
							Name: "another-container",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "opentelemetry-auto-instrumentation-java",
									MountPath: "/otel-auto-instrumentation-java",
								},
							},
							Env: []v1.EnvVar{

								{
									Name: "OTEL_POD_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.podIP",
										},
									},
								},
								{
									Name: "OTEL_NODE_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.hostIP",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_NODE_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "spec.nodeName",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_POD_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "metadata.name",
										},
									},
								},
								{
									Name:  "OTEL_RESOURCE_ATTRIBUTES",
									Value: "k8s.container.name=another-container,k8s.namespace.name=test-namespace,k8s.node.ip=$(OTEL_NODE_IP),k8s.node.name=$(OTEL_RESOURCE_ATTRIBUTES_NODE_NAME),k8s.pod.ip=$(OTEL_POD_IP),k8s.pod.name=non-instrumented-pod",
								},
								{
									Name:  "JAVA_TOOL_OPTIONS",
									Value: " -javaagent:/otel-auto-instrumentation-java/javaagent.jar",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "pod needs instrumentation, many fields",
			fields: fields{
				cfg: defaultConfig,
				insts: []v1alpha1.Instrumentation{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "otel-instance", Namespace: testNamespace},
						Spec: v1alpha1.InstrumentationSpec{
							Env: []v1.EnvVar{
								{Name: "CUSTOM_ENV_VAR", Value: "custom_value"},
							},
							Java: v1alpha1.Java{
								Image: "foo/bar:1",
							},
							Exporter: v1alpha1.Exporter{
								Endpoint: "http://exporter-endpoint",
							},
							Propagators: []v1alpha1.Propagator{"tracecontext", "baggage"},
							Sampler: v1alpha1.Sampler{
								Type:     "parentbased_always_on",
								Argument: "0.25",
							},
							Resource: v1alpha1.Resource{
								Attributes: map[string]string{
									"service.name":  "otel-service",
									"service.owner": "owner",
								},
								AddK8sUIDAttributes: true,
							},
						},
					},
				},
			},
			args: args{
				ns: v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}},
				pod: v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "instrumented-pod",
						Namespace: testNamespace,
						Annotations: map[string]string{
							"instrumentation.opentelemetry.io/inject-java": "true",
							"resource.opentelemetry.io/example":            "yes",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "test-node",
						Containers: []v1.Container{
							{
								Name: "app-container",
								Env: []v1.EnvVar{
									{
										Name:  "OTEL_RESOURCE_ATTRIBUTES",
										Value: "another-example=test",
									},
								},
							},
						},
					},
				},
			},
			want: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "instrumented-pod",
					Namespace: testNamespace,
					Annotations: map[string]string{
						"instrumentation.opentelemetry.io/inject-java": "true",
						"resource.opentelemetry.io/example":            "yes",
					},
				},
				Spec: v1.PodSpec{
					NodeName: "test-node",
					Volumes: []v1.Volume{
						{
							Name: "opentelemetry-auto-instrumentation-java",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: &defaultVolumeLimitSize,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:    "opentelemetry-auto-instrumentation-java",
							Image:   "foo/bar:1",
							Command: []string{"cp", "/javaagent.jar", "/otel-auto-instrumentation-java/javaagent.jar"},
							VolumeMounts: []v1.VolumeMount{{
								Name:      "opentelemetry-auto-instrumentation-java",
								MountPath: "/otel-auto-instrumentation-java",
							}},
						},
					},
					Containers: []v1.Container{
						{
							Name: "app-container",
							Env: []v1.EnvVar{
								{
									Name:  constants.EnvOTELTracesSamplerArg,
									Value: "0.25",
								},
								{
									Name: "OTEL_POD_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.podIP",
										},
									},
								},
								{
									Name: "OTEL_NODE_IP",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "status.hostIP",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_NODE_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											APIVersion: "",
											FieldPath:  "spec.nodeName",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_POD_NAME",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "OTEL_RESOURCE_ATTRIBUTES_POD_UID",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "metadata.uid",
										},
									},
								},
								{
									Name:  "CUSTOM_ENV_VAR",
									Value: "custom_value",
								},
								{
									Name:  "OTEL_EXPORTER_OTLP_ENDPOINT",
									Value: "http://exporter-endpoint",
								},
								{
									Name:  constants.EnvOTELPropagators,
									Value: "tracecontext,baggage",
								},
								{
									Name:  constants.EnvOTELTracesSampler,
									Value: "parentbased_always_on",
								},
								{
									Name:  "OTEL_RESOURCE_ATTRIBUTES",
									Value: "another-example=test,example=yes,k8s.container.name=app-container,k8s.namespace.name=test-namespace,k8s.node.ip=$(OTEL_NODE_IP),k8s.node.name=test-node,k8s.pod.ip=$(OTEL_POD_IP),k8s.pod.name=instrumented-pod,k8s.pod.uid=$(OTEL_RESOURCE_ATTRIBUTES_POD_UID),service.name=otel-service,service.owner=owner",
								},
								{
									Name:  "JAVA_TOOL_OPTIONS",
									Value: " -javaagent:/otel-auto-instrumentation-java/javaagent.jar",
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "opentelemetry-auto-instrumentation-java",
									MountPath: "/otel-auto-instrumentation-java",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Pod needs instrumentation, no instances available",
			fields: fields{
				cfg:   config.Config{},
				insts: nil, // No specific instrumentation instance
			},
			args: args{
				ns: v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}},
				pod: v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "non-instrumented-pod",
						Namespace:   testNamespace, // Using the constant namespace
						Annotations: map[string]string{"instrumentation.opentelemetry.io/inject-java": "true"},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{Name: "app-container"},
						},
					},
				},
			},
			want: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "non-instrumented-pod",
					Namespace:   testNamespace, // Using the constant namespace
					Annotations: map[string]string{"instrumentation.opentelemetry.io/inject-java": "true"},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "app-container"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid container annotations for instrumentation",
			fields: fields{
				cfg: defaultConfig,
				insts: []v1alpha1.Instrumentation{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "otel-java-instance", Namespace: testNamespace},
					},
				},
			},
			args: args{
				ns: v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}},
				pod: v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "non-instrumented-pod",
						Namespace: testNamespace, // Using the constant namespace
						Annotations: map[string]string{
							"instrumentation.opentelemetry.io/container-names": "app-container,°*00sdoivnijpa",
							"instrumentation.opentelemetry.io/inject-java":     "true",
						},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{Name: "app-container"},
							{Name: "another-container"},
						},
					},
				},
			},
			want: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-instrumented-pod",
					Namespace: testNamespace, // Using the constant namespace
					Annotations: map[string]string{
						"instrumentation.opentelemetry.io/container-names": "app-container,°*00sdoivnijpa",
						"instrumentation.opentelemetry.io/inject-java":     "true",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "app-container"},
						{Name: "another-container"},
					},
				},
			}, // No mutation due to invalid container annotation
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := getFakeClient(t)
			mutator := instrumentationv2.NewMutator(logr.Discard(), k8sClient, record.NewFakeRecorder(100), tt.fields.cfg)
			require.NotNil(t, mutator)
			err := k8sClient.Create(context.Background(), &tt.args.ns)
			require.NoError(t, err)
			defer func() {
				_ = k8sClient.Delete(context.Background(), &tt.args.ns)
			}()
			for _, inst := range tt.fields.insts {
				i := inst
				err = k8sClient.Create(context.Background(), &i)
				require.NoError(t, err)
			}

			got, err := mutator.Mutate(context.Background(), tt.args.ns, tt.args.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("Mutate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.EqualExportedValues(t, tt.want, got)
		})
	}
}
