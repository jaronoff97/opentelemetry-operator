// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/pkg/constants"
)

type ResourceLayer int

const (
	ResourceCR ResourceLayer = iota
	ResourceKubernetes
	ResourceService
	ResourcePodAnnotation
)

type ResourceAttributes struct {
	layers []resourceLayer
}

type resourceLayer struct {
	precedence ResourceLayer
	attrs      map[string]string
}

type resourceValue struct {
	value      string
	precedence ResourceLayer
}

func (r *ResourceAttributes) Add(precedence ResourceLayer, key, value string) {
	if value == "" {
		return
	}
	if len(r.layers) == 0 || r.layers[len(r.layers)-1].precedence != precedence {
		r.layers = append(r.layers, resourceLayer{
			precedence: precedence,
			attrs:      map[string]string{},
		})
	}
	r.layers[len(r.layers)-1].attrs[key] = value
}

func (r *ResourceAttributes) AddMap(precedence ResourceLayer, attrs map[string]string) {
	for k, v := range attrs {
		r.Add(precedence, k, v)
	}
}

func (r ResourceAttributes) PlannedValue(key string) string {
	values := r.values(nil)
	return values[key].value
}

func (r ResourceAttributes) HasPlanned(key string) bool {
	values := r.values(nil)
	_, ok := values[key]
	return ok
}

func (r ResourceAttributes) Build(existing string) string {
	locked := parseResourceKeys(existing)
	values := r.values(locked)
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var sb strings.Builder
	for _, k := range keys {
		if sb.Len() > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(values[k].value)
	}
	return sb.String()
}

func (r ResourceAttributes) values(locked map[string]struct{}) map[string]resourceValue {
	values := map[string]resourceValue{}
	for _, layer := range r.layers {
		for k, v := range layer.attrs {
			if v == "" {
				continue
			}
			if _, ok := locked[k]; ok {
				continue
			}
			current, ok := values[k]
			if !ok || layer.precedence >= current.precedence {
				values[k] = resourceValue{value: v, precedence: layer.precedence}
			}
		}
	}
	return values
}

func parseResourceKeys(value string) map[string]struct{} {
	keys := map[string]struct{}{}
	for kv := range strings.SplitSeq(value, ",") {
		key, _, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if !ok || key == "" {
			continue
		}
		keys[key] = struct{}{}
	}
	return keys
}

type ResourceCollector struct {
	client client.Client
	logger logr.Logger
}

func NewResourceCollector(c client.Client, logger logr.Logger) *ResourceCollector {
	return &ResourceCollector{client: c, logger: logger}
}

func (r *ResourceCollector) Build(ctx context.Context, inst v1alpha1.Instrumentation, ns corev1.Namespace, pod corev1.Pod, container corev1.Container) ResourceAttributes {
	var attrs ResourceAttributes
	attrs.AddMap(ResourceCR, inst.Spec.Resource.Attributes)

	attrs.Add(ResourceKubernetes, string(semconv.K8SNamespaceNameKey), ns.Name)
	attrs.Add(ResourceKubernetes, string(semconv.K8SContainerNameKey), container.Name)
	attrs.Add(ResourceKubernetes, string(semconv.K8SPodNameKey), pod.Name)
	attrs.Add(ResourceKubernetes, string(semconv.K8SPodUIDKey), string(pod.UID))
	attrs.Add(ResourceKubernetes, string(semconv.K8SNodeNameKey), pod.Spec.NodeName)
	attrs.Add(ResourceKubernetes, string(semconv.ServiceInstanceIDKey), createServiceInstanceID(pod, ns.Name, fmt.Sprintf("$(%s)", constants.EnvPodName), container.Name))

	parents := map[attribute.Key]string{}
	r.collectParentLabels(ctx, inst.Spec.Resource.AddK8sUIDAttributes, ns, pod.ObjectMeta, parents)
	for key, value := range parents {
		attrs.Add(ResourceKubernetes, string(key), value)
	}

	for k, v := range pod.GetAnnotations() {
		key, ok := strings.CutPrefix(k, constants.ResourceAttributeAnnotationPrefix)
		if !ok || key == string(semconv.ServiceNameKey) {
			continue
		}
		attrs.Add(ResourcePodAnnotation, key, v)
	}

	if namespace := chooseServiceNamespace(pod, inst.Spec.Defaults.UseLabelsForResourceAttributes, ns.Name); namespace != "" {
		attrs.Add(ResourceService, string(semconv.ServiceNamespaceKey), namespace)
	}
	if version := chooseServiceVersion(pod, inst.Spec.Defaults.UseLabelsForResourceAttributes, container); version != "" {
		attrs.Add(ResourceService, string(semconv.ServiceVersionKey), version)
	}
	return attrs
}

func (r *ResourceCollector) collectParentLabels(ctx context.Context, includeUID bool, ns corev1.Namespace, meta metav1.ObjectMeta, out map[attribute.Key]string) {
	for _, owner := range meta.OwnerReferences {
		switch strings.ToLower(owner.Kind) {
		case "replicaset":
			out[semconv.K8SReplicaSetNameKey] = owner.Name
			if includeUID {
				out[semconv.K8SReplicaSetUIDKey] = string(owner.UID)
			}
			if r.client == nil {
				continue
			}
			rs := appsv1.ReplicaSet{}
			r.lookupOwner(ctx, ns.Name, owner.Name, &rs, "replicaset")
			r.collectParentLabels(ctx, includeUID, ns, rs.ObjectMeta, out)
		case "deployment":
			out[semconv.K8SDeploymentNameKey] = owner.Name
			if includeUID {
				out[semconv.K8SDeploymentUIDKey] = string(owner.UID)
			}
		case "statefulset":
			out[semconv.K8SStatefulSetNameKey] = owner.Name
			if includeUID {
				out[semconv.K8SStatefulSetUIDKey] = string(owner.UID)
			}
		case "daemonset":
			out[semconv.K8SDaemonSetNameKey] = owner.Name
			if includeUID {
				out[semconv.K8SDaemonSetUIDKey] = string(owner.UID)
			}
		case "job":
			out[semconv.K8SJobNameKey] = owner.Name
			if includeUID {
				out[semconv.K8SJobUIDKey] = string(owner.UID)
			}
			if r.client == nil {
				continue
			}
			job := batchv1.Job{}
			r.lookupOwner(ctx, ns.Name, owner.Name, &job, "job")
			r.collectParentLabels(ctx, includeUID, ns, job.ObjectMeta, out)
		case "cronjob":
			out[semconv.K8SCronJobNameKey] = owner.Name
			if includeUID {
				out[semconv.K8SCronJobUIDKey] = string(owner.UID)
			}
		}
	}
}

func (r *ResourceCollector) lookupOwner(ctx context.Context, namespace, name string, obj client.Object, kind string) {
	backoff := wait.Backoff{Duration: 10 * time.Millisecond, Factor: 1.5, Jitter: 0.1, Steps: 20, Cap: 2 * time.Second}
	get := func() error {
		return r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj)
	}
	if err := retry.OnError(backoff, apierrors.IsNotFound, get); err != nil {
		r.logger.Error(err, "failed to look up owner reference", "kind", kind, "name", name, "namespace", namespace)
	}
}

func chooseServiceName(pod corev1.Pod, useLabels bool, attrs ResourceAttributes, container corev1.Container) string {
	if name := chooseLabelOrAnnotation(pod, useLabels, semconv.ServiceNameKey, constants.LabelAppName); name != "" {
		return name
	}
	for _, key := range []string{
		string(semconv.K8SDeploymentNameKey),
		string(semconv.K8SReplicaSetNameKey),
		string(semconv.K8SStatefulSetNameKey),
		string(semconv.K8SDaemonSetNameKey),
		string(semconv.K8SCronJobNameKey),
		string(semconv.K8SJobNameKey),
		string(semconv.K8SPodNameKey),
	} {
		if v := attrs.PlannedValue(key); v != "" {
			return v
		}
	}
	return container.Name
}

func chooseLabelOrAnnotation(pod corev1.Pod, useLabels bool, key attribute.Key, labelKeys []string) string {
	if v := pod.GetAnnotations()[(constants.ResourceAttributeAnnotationPrefix + string(key))]; v != "" {
		return v
	}
	if useLabels {
		for _, lk := range labelKeys {
			if v := pod.GetLabels()[lk]; v != "" {
				return v
			}
		}
	}
	return ""
}

func chooseServiceVersion(pod corev1.Pod, useLabels bool, container corev1.Container) string {
	if v := chooseLabelOrAnnotation(pod, useLabels, semconv.ServiceVersionKey, constants.LabelAppVersion); v != "" {
		return v
	}
	v, err := parseServiceVersionFromImage(container.Image)
	if err != nil {
		return ""
	}
	return v
}

func chooseServiceNamespace(pod corev1.Pod, useLabels bool, fallbackNs string) string {
	if ns := chooseLabelOrAnnotation(pod, useLabels, semconv.ServiceNamespaceKey, nil); ns != "" {
		return ns
	}
	return fallbackNs
}

var errCannotRetrieveImage = errors.New("cannot retrieve image name")

func parseServiceVersionFromImage(image string) (string, error) {
	ref, err := reference.Parse(image)
	if err != nil {
		return "", err
	}
	named, ok := ref.(reference.Named)
	if !ok {
		return "", errCannotRetrieveImage
	}
	var tag, digest string
	if t, ok := named.(reference.Tagged); ok {
		tag = t.Tag()
	}
	if d, ok := named.(reference.Digested); ok {
		digest = d.Digest().String()
	}
	switch {
	case digest != "" && tag != "":
		return fmt.Sprintf("%s@%s", tag, digest), nil
	case digest != "":
		return digest, nil
	case tag != "":
		return tag, nil
	}
	return "", errCannotRetrieveImage
}

func createServiceInstanceID(pod corev1.Pod, namespace, podName, containerName string) string {
	if v := chooseLabelOrAnnotation(pod, false, semconv.ServiceInstanceIDKey, nil); v != "" {
		return v
	}
	if namespace == "" || podName == "" || containerName == "" {
		return ""
	}
	return strings.Join([]string{namespace, podName, containerName}, ".")
}
