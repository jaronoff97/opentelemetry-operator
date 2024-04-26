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

package parsers

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
)

func GetReceiverPorts(logger logr.Logger, cd v1beta1.ComponentDefinitions) ([]corev1.ServicePort, error) {
	var ports []corev1.ServicePort
	for componentName, config := range cd {
		if opts, ok := genericReceivers[componentName]; ok {
			port, err := SinglePortParser(logger, componentName, config, opts...)
			if err != nil {
				return nil, err
			}
			ports = append(ports, port...)
		}
	}
	return ports, nil
}
