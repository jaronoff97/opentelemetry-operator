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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-telemetry/opentelemetry-operator/internal/manifests"
	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
	"github.com/open-telemetry/opentelemetry-operator/pkg/constants"
)

func VersionedConfigMap(params manifests.Params) *corev1.ConfigMap {
	configHash := GetConfigMapSHA(params.OtelCol.Spec.Config)
	name := naming.VersionedConfigMap(params.OtelCol.Name, configHash)
	labels := Labels(params.OtelCol, name, params.Config.LabelsFilter())
	labels[constants.CollectorValidationJobLabelName] = naming.Job(params.OtelCol.Name, configHash)

	replacedConf, err := ReplaceConfig(params.OtelCol)
	if err != nil {
		params.Log.V(2).Info("failed to update prometheus config to use sharded targets: ", "err", err)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   params.OtelCol.Namespace,
			Labels:      labels,
			Annotations: params.OtelCol.Annotations,
		},
		Data: map[string]string{
			"collector.yaml": replacedConf,
		},
	}
}

func ConfigMap(params manifests.Params) *corev1.ConfigMap {
	name := naming.ConfigMap(params.OtelCol.Name)
	labels := Labels(params.OtelCol, name, []string{})

	replacedConf, err := ReplaceConfig(params.OtelCol)
	if err != nil {
		params.Log.V(2).Info("failed to update prometheus config to use sharded targets: ", "err", err)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   params.OtelCol.Namespace,
			Labels:      labels,
			Annotations: params.OtelCol.Annotations,
		},
		Data: map[string]string{
			"collector.yaml": replacedConf,
		},
	}
}
