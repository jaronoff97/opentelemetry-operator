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
	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
)

const (
	endpoint      = "endpoint"
	listenAddress = "listen_address"
)

func SinglePortParser(logger logr.Logger, componentName string, config *v1beta1.AnyConfig, opts ...PortBuilderOption) ([]corev1.ServicePort, error) {
	svc := &corev1.ServicePort{}
	for _, opt := range opts {
		opt(svc)
	}
	portOverride := svc.Port
	var err error
	if configEndpoint := v1beta1.GetOrDefault(config, endpoint, ""); len(configEndpoint) > 0 {
		portOverride, err = portFromEndpoint(configEndpoint)
	}
	if configListenAddress := v1beta1.GetOrDefault(config, listenAddress, ""); len(configListenAddress) > 0 {
		portOverride, err = portFromEndpoint(configListenAddress)
	}
	if portOverride == 0 && err != nil {
		logger.WithValues("receiver", componentName).Error(err, "couldn't parse the endpoint's port and no default port set")
		return []corev1.ServicePort{}, err
	}

	svc.Name = naming.PortName(componentName, portOverride)
	return []corev1.ServicePort{ConstructServicePort(svc, portOverride)}, nil
}
