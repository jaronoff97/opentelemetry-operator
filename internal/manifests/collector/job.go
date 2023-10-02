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

package collector

import (
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/internal/config"
	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
)

var (
	// backoffLimit is set to one because we don't need to retry this job, it either fails or succeeds.
	backoffLimit int32 = 1
)

func Job(cfg config.Config, logger logr.Logger, otelcol v1alpha1.OpenTelemetryCollector) *batchv1.Job {
	confMapSha := GetConfigMapSHA(otelcol.Spec.Config)
	name := naming.Job(otelcol.Name, confMapSha)
	labels := Labels(otelcol, name, cfg.LabelsFilter())

	annotations := Annotations(otelcol)
	podAnnotations := PodAnnotations(otelcol)
	// manualSelector is explicitly false because we don't want to cause a potential conflict between the job
	// and the replicaset
	manualSelector := false

	c := Container(cfg, logger, otelcol, true)
	c.Args = append([]string{"validate"}, c.Args...)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   otelcol.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.JobSpec{
			ManualSelector: &manualSelector,
			BackoffLimit:   &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:                 corev1.RestartPolicyNever,
					ServiceAccountName:            ServiceAccountName(otelcol),
					InitContainers:                otelcol.Spec.InitContainers,
					Containers:                    append(otelcol.Spec.AdditionalContainers, c),
					Volumes:                       Volumes(cfg, otelcol, naming.VersionedConfigMap(otelcol.Name, confMapSha)),
					DNSPolicy:                     getDNSPolicy(otelcol),
					HostNetwork:                   otelcol.Spec.HostNetwork,
					Tolerations:                   otelcol.Spec.Tolerations,
					NodeSelector:                  otelcol.Spec.NodeSelector,
					SecurityContext:               otelcol.Spec.PodSecurityContext,
					PriorityClassName:             otelcol.Spec.PriorityClassName,
					Affinity:                      otelcol.Spec.Affinity,
					TerminationGracePeriodSeconds: otelcol.Spec.TerminationGracePeriodSeconds,
					TopologySpreadConstraints:     otelcol.Spec.TopologySpreadConstraints,
				},
			},
		},
	}
}
