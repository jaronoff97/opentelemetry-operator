package instrumentationv2

import (
	"sort"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/pkg/constants"
)

type Resource struct {
	res map[string]string
}

// NewResource returns a new instance of a Resource based on a
// key value pair string of the form k=v,...
func NewResource(resourceAttrs string) *Resource {
	resource := map[string]string{}
	if len(resourceAttrs) == 0 {
		return &Resource{res: resource}
	}
	existingResArr := strings.Split(resourceAttrs, ",")
	for _, kv := range existingResArr {
		keyValueArr := strings.Split(strings.TrimSpace(kv), "=")
		if len(keyValueArr) != 2 {
			continue
		}
		resource[keyValueArr[0]] = keyValueArr[1]
	}
	return &Resource{res: resource}
}

func (r *Resource) AsString() string {
	keys := make([]string, 0, len(r.res))
	for k := range r.res {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, k := range keys {
		if builder.Len() > 0 {
			builder.WriteString(",")
		}
		builder.WriteString(k)
		builder.WriteString("=")
		builder.WriteString(r.res[k])
	}
	return builder.String()
}

func (r *Resource) addKubernetesSemconvResource(ns corev1.Namespace, pod corev1.Pod, index int) {
	k8sResources := map[attribute.Key]string{}
	k8sResources[semconv.K8SNamespaceNameKey] = ns.Name
	k8sResources[semconv.K8SContainerNameKey] = pod.Spec.Containers[index].Name
	// Some fields might be empty - node name, pod name
	// The pod name might be empty if the pod is created form deployment template
	k8sResources[semconv.K8SPodNameKey] = pod.Name
	k8sResources[semconv.K8SPodUIDKey] = string(pod.UID)
	k8sResources[semconv.K8SNodeNameKey] = pod.Spec.NodeName
	// TODO: k8sResources[semconv.ServiceInstanceIDKey] = createServiceInstanceId(ns.Name, fmt.Sprintf("$(%s)", constants.EnvPodName), pod.Spec.Containers[index].Name)
	//i.addParentResourceLabels(ctx, otelinst.Spec.Resource.AddK8sUIDAttributes, ns, pod.ObjectMeta, k8sResources)
	for key, v := range k8sResources {
		if len(v) == 0 {
			continue
		}
		r.SetIfNotPresent(string(key), v)
	}
}

func (r *Resource) addPodAnnotationsResource(pod corev1.Pod) {
	for k, v := range pod.GetAnnotations() {
		if strings.HasPrefix(k, constants.OtelAnnotationNamespace) {
			key := strings.TrimSpace(strings.TrimPrefix(k, constants.OtelAnnotationNamespace))
			r.SetIfNotPresent(key, v)
		}
	}
}

func (r *Resource) addInstrumentationResource(inst *v1alpha1.Instrumentation) {
	for k, v := range inst.Spec.Resource.Attributes {
		r.SetIfNotPresent(k, v)
	}
}

func (r *Resource) SetIfNotPresent(key, value string) {
	if _, ok := r.res[key]; !ok {
		r.res[key] = value
	}
}
