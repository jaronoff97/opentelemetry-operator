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
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/internal/config"
	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
	"github.com/open-telemetry/opentelemetry-operator/internal/webhook/podmutation"
	"github.com/open-telemetry/opentelemetry-operator/pkg/constants"
	"github.com/open-telemetry/opentelemetry-operator/pkg/instrumentation"
)

var (
	errMultipleInstancesPossible = errors.New("multiple OpenTelemetry Instrumentation instances available, cannot determine which one to select")
	errNoInstancesAvailable      = errors.New("no OpenTelemetry Instrumentation instances available")
	defaultEnvVars               = map[string]string{
		constants.EnvPodIP:    "status.podIP",
		constants.EnvNodeIP:   "status.hostIP",
		constants.EnvPodName:  "metadata.name",
		constants.EnvPodUID:   "metadata.uid",
		constants.EnvNodeName: "spec.nodeName",
	}
	sortedKeys = []string{
		constants.EnvPodIP,
		constants.EnvNodeIP,
		constants.EnvNodeName,
		constants.EnvPodName,
		constants.EnvPodUID,
	}
	semconvFromConstant = map[string]string{
		constants.EnvPodName:  string(semconv.K8SPodNameKey),
		constants.EnvPodUID:   string(semconv.K8SPodUIDKey),
		constants.EnvNodeName: string(semconv.K8SNodeNameKey),
		constants.EnvPodIP:    "k8s.pod.ip",
		constants.EnvNodeIP:   "k8s.node.ip",
	}
)

type Triple[L, M, R any] struct {
	left   L
	middle M
	right  R
}

func NewTriple[L, M, R any](left L, middle M, right R) Triple[L, M, R] {
	return Triple[L, M, R]{left: left, middle: middle, right: right}
}

type Language interface {
	Name() string
	IsSidecar() bool
	Enabled() bool
	Inject(logger logr.Logger, ns corev1.Namespace, pod corev1.Pod, index int, inst *v1alpha1.Instrumentation) (corev1.Pod, error)
}

type Mutator struct {
	logger    logr.Logger
	client    client.Client
	recorder  record.EventRecorder
	cfg       config.Config
	languages map[string]Language
}

var _ podmutation.PodMutator = &Mutator{}

func NewMutator(logger logr.Logger, c client.Client, recorder record.EventRecorder, cfg config.Config) *Mutator {
	languages := map[string]Language{}
	for _, language := range []Language{
		newNodejs(cfg),
		newJava(cfg),
	} {
		languages[language.Name()] = language
	}
	return &Mutator{
		logger:    logger,
		client:    c,
		recorder:  recorder,
		cfg:       cfg,
		languages: languages,
	}
}

func (m *Mutator) Mutate(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	logger := m.logger.WithValues("namespace", pod.Namespace)
	if pod.Name != "" {
		logger = logger.WithValues("name", pod.Name)
	} else if pod.GenerateName != "" {
		logger = logger.WithValues("generateName", pod.GenerateName)
	}

	// We check if Pod is already instrumented.
	if m.isAutoInstrumentationInjected(pod) {
		logger.Info("Skipping pod instrumentation - already instrumented")
		return pod, nil
	}
	var inst *v1alpha1.Instrumentation
	var err error

	/*
		Plan:
		* What instrumentation(s) should be used?
		* Get the container names to be injected
		* Match the multi-instrumentations if they're used
		* Do injection

		We need a list of tuples from (container index, language)

		Injection plan:
		* inject default env vars
		* inject SDK config
		* language specific stuff
	*/
	// insts is a triple of the container index, the instrumentation object, and the language key
	var insts []Triple[int, *v1alpha1.Instrumentation, string]
	for _, language := range m.languages {
		if !language.Enabled() {
			continue
		}
		// We bail out if any annotation fails to process.
		if inst, err = m.getInstrumentationInstance(ctx, ns, pod, annotationName(language)); err != nil {
			// we still allow the pod to be created, but we log a message to the operator's logs
			logger.Error(err, "failed to select an OpenTelemetry Instrumentation instance for this pod")
			m.recorder.Event(pod.DeepCopy(), "Warning", "InstrumentationRequestRejected", err.Error())
			return pod, err
		} else if inst == nil {
			logger.Error(nil, "support for auto instrumentation is not enabled", "language", language.Name())
			m.recorder.Event(pod.DeepCopy(), "Warning", "InstrumentationRequestRejected", fmt.Sprintf("support for %s auto instrumentation is not enabled", language.Name()))
			continue
		}
		// Find container name, default to the first container
		if len(pod.Spec.Containers) == 1 {
			insts = append(insts, NewTriple(0, inst, language.Name()))
			continue
		}
		containerNames, err := m.getContainerNames(ns, pod, language)
		if err != nil {
			logger.Error(nil, "invalid container name", "language", language.Name(), "containers", containerNames)
			m.recorder.Event(pod.DeepCopy(), "Warning", "InstrumentationRequestRejected", fmt.Sprintf("invalid containers annotations for %s auto instrumentation: %s", language.Name(), containerNames))
			return pod, err
		}
		for _, containerName := range containerNames {
			for i, container := range pod.Spec.Containers {
				if container.Name == containerName {
					insts = append(insts, NewTriple(i, inst, language.Name()))
				}
			}
		}
	}
	// TODO: Set pod defaults
	/*
		pod = i.injectCommonEnvVar(otelinst, pod, index)
		pod = i.injectCommonSDKConfig(ctx, otelinst, ns, pod, index, index)
		pod = i.setInitContainerSecurityContext(pod, pod.Spec.Containers[index].SecurityContext, javaInitContainerName)
	*/
	for _, t := range insts {
		m.setCommonEnvironmentVariables(ns, pod, t.left, t.middle)
		p, err := m.languages[t.right].Inject(m.logger, ns, pod, t.left, t.middle)
		if err != nil {
			return pod, err
		}
		pod = p
	}

	return pod, nil
}

func (m *Mutator) setCommonEnvironmentVariables(ns corev1.Namespace, pod corev1.Pod, index int, inst *v1alpha1.Instrumentation) corev1.Pod {
	container := &pod.Spec.Containers[index]
	envReverseIndex := map[string]int{}
	for i, envVar := range container.Env {
		envReverseIndex[envVar.Name] = i
	}
	var resourceAttrs string
	if v, ok := envReverseIndex[constants.EnvOTELResourceAttrs]; ok {
		resourceAttrs = container.Env[v].Value
	}
	resource := NewResource(resourceAttrs)
	resource.addInstrumentationResource(inst)
	resource.addKubernetesSemconvResource(ns, pod, index)
	resource.addPodAnnotationsResource(pod)
	for _, constantKey := range sortedKeys {
		fieldPath := defaultEnvVars[constantKey]
		if constantKey == constants.EnvPodUID && !inst.Spec.Resource.AddK8sUIDAttributes {
			continue
		}
		if _, ok := envReverseIndex[constantKey]; !ok {
			container.Env = append(container.Env, corev1.EnvVar{
				Name: constantKey,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: fieldPath,
					},
				},
			})
			resource.SetIfNotPresent(semconvFromConstant[constantKey], fmt.Sprintf("$(%s)", constantKey))
		}
	}
	container.Env = append(container.Env, m.getEnvsFromInst(envReverseIndex, inst, resource)...)
	if idx, ok := envReverseIndex[constants.EnvOTELResourceAttrs]; !ok {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  constants.EnvOTELResourceAttrs,
			Value: resource.AsString(),
		})
	} else {
		container.Env[idx].Value = resource.AsString()
	}
	// Move OTEL_RESOURCE_ATTRIBUTES to last position on env list.
	// When OTEL_RESOURCE_ATTRIBUTES environment variable uses other env vars
	// as attributes value they have to be configured before.
	// It is mandatory to set right order to avoid attributes with value
	// pointing to the name of used environment variable instead of its value.
	idx := getIndexOfEnv(container.Env, constants.EnvOTELResourceAttrs)
	lastIdx := len(container.Env) - 1
	container.Env[idx], container.Env[lastIdx] = container.Env[lastIdx], container.Env[idx]
	return pod
}

func (m *Mutator) getEnvsFromInst(envReverseIndex map[string]int, inst *v1alpha1.Instrumentation, res *Resource) []corev1.EnvVar {
	var envs []corev1.EnvVar
	for _, env := range inst.Spec.Env {
		if _, ok := envReverseIndex[env.Name]; !ok {
			envs = append(envs, env)
		}
	}
	if inst.Spec.Exporter.Endpoint != "" {
		if _, ok := envReverseIndex[constants.EnvOTELExporterOTLPEndpoint]; !ok {
			envs = append(envs, corev1.EnvVar{
				Name:  constants.EnvOTELExporterOTLPEndpoint,
				Value: inst.Spec.Endpoint,
			})
		}
	}
	if len(inst.Spec.Propagators) > 0 {
		if _, ok := envReverseIndex[constants.EnvOTELPropagators]; !ok {
			var sb strings.Builder
			first := true
			for _, propagator := range inst.Spec.Propagators {
				if !first {
					sb.WriteString(",")
				} else {
					first = false
				}
				sb.WriteString(string(propagator))
			}
			envs = append(envs, corev1.EnvVar{
				Name:  constants.EnvOTELPropagators,
				Value: sb.String(),
			})
		}
	}
	if len(inst.Spec.Sampler.Type) > 0 {
		if _, ok := envReverseIndex[constants.EnvOTELTracesSamplerArg]; !ok {
			envs = append(envs, corev1.EnvVar{
				Name:  constants.EnvOTELTracesSampler,
				Value: string(inst.Spec.Sampler.Type),
			})
			if len(inst.Spec.Sampler.Argument) > 0 {
				envs = append(envs, corev1.EnvVar{
					Name:  constants.EnvOTELTracesSamplerArg,
					Value: inst.Spec.Sampler.Argument,
				})
			}
		}
	}
	return envs
}

func (m *Mutator) getContainerNames(ns corev1.Namespace, pod corev1.Pod, language Language) ([]string, error) {
	containersAnnotation := instrumentation.AnnotationValue(ns.ObjectMeta, pod.ObjectMeta, instrumentation.AnnotationInjectContainerName)
	containersLanguageAnnotation := instrumentation.AnnotationValue(ns.ObjectMeta, pod.ObjectMeta, multiContainerAnnotationName(language))
	if len(containersAnnotation) == 0 && len(containersLanguageAnnotation) == 0 {
		return nil, nil
	}
	annot := containersAnnotation
	if len(containersLanguageAnnotation) > 0 {
		annot = containersLanguageAnnotation
	}
	if err := instrumentation.IsValidContainersAnnotation(annot); err != nil {
		return []string{annot}, err
	}
	return strings.Split(annot, ","), nil
}

func (m *Mutator) getInstrumentationInstance(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, instAnnotation string) (*v1alpha1.Instrumentation, error) {
	instValue := instrumentation.AnnotationValue(ns.ObjectMeta, pod.ObjectMeta, instAnnotation)

	if len(instValue) == 0 || strings.EqualFold(instValue, "false") {
		return nil, nil
	}

	if strings.EqualFold(instValue, "true") {
		return m.selectInstrumentationInstanceFromNamespace(ctx, ns)
	}

	var instNamespacedName types.NamespacedName
	if instNamespace, instName, namespaced := strings.Cut(instValue, "/"); namespaced {
		instNamespacedName = types.NamespacedName{Name: instName, Namespace: instNamespace}
	} else {
		instNamespacedName = types.NamespacedName{Name: instValue, Namespace: ns.Name}
	}

	otelInst := &v1alpha1.Instrumentation{}
	err := m.client.Get(ctx, instNamespacedName, otelInst)
	if err != nil {
		return nil, err
	}

	return otelInst, nil
}

func (m *Mutator) selectInstrumentationInstanceFromNamespace(ctx context.Context, ns corev1.Namespace) (*v1alpha1.Instrumentation, error) {
	var otelInsts v1alpha1.InstrumentationList
	if err := m.client.List(ctx, &otelInsts, client.InNamespace(ns.Name)); err != nil {
		return nil, err
	}

	switch s := len(otelInsts.Items); {
	case s == 0:
		return nil, errNoInstancesAvailable
	case s > 1:
		return nil, errMultipleInstancesPossible
	default:
		return &otelInsts.Items[0], nil
	}
}

func (m *Mutator) isAutoInstrumentationInjected(pod corev1.Pod) bool {
	for _, language := range m.languages {
		if language.IsSidecar() {
			for _, cont := range pod.Spec.Containers {
				if strings.EqualFold(cont.Name, prefixedName(language)) {
					return true
				}
				// This environment variable is set in the sidecar and in the
				// collector containers. We look for it in any container that is not
				// the sidecar container to check if we already injected the
				// instrumentation or not
				if cont.Name != naming.Container() {
					for _, envVar := range cont.Env {
						if envVar.Name == constants.EnvNodeName {
							return true
						}
					}
				}
			}
			continue
		}

		for _, cont := range pod.Spec.InitContainers {
			if strings.Contains(prefixedName(language), cont.Name) {
				return true
			}
		}
	}
	return false
}
