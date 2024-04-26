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
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
)

const (
	protocolKey = "protocols"
)

type singleEndpointConfig struct {
	Endpoint      string `json:"endpoint,omitempty"`
	ListenAddress string `json:"listen_address,omitempty"`
}

type multiProtocolEndpointConfig struct {
	Protocols map[string]*singleEndpointConfig `json:"protocols"`
}

func MultiplePortParser(logger logr.Logger, componentName string, config *v1beta1.AnyConfig, optMap map[string][]PortBuilderOption) ([]corev1.ServicePort, error) {
	if config == nil {
		return nil, errors.New("cannot parse nil config")
	} else if _, ok := config.Object[protocolKey]; !ok {
		return nil, errors.New("no protocols specified")
	}
	protocols := &multiProtocolEndpointConfig{}
	err := LoadMap(config.Object, protocols)
	if err != nil {
		return nil, err
	}
	var ports []corev1.ServicePort
	for protocolName, endpointConfig := range protocols.Protocols {
		if opts, ok := optMap[protocolName]; !ok {
			return nil, errors.New("unknown protocol")
		} else {
			svcPort := &corev1.ServicePort{}
			for _, opt := range opts {
				opt(svcPort)
			}
			portOverride := svcPort.Port
			var portErr error
			if endpointConfig != nil {
				if len(endpointConfig.Endpoint) > 0 {
					portOverride, portErr = portFromEndpoint(endpointConfig.Endpoint)
				}
				if len(endpointConfig.ListenAddress) > 0 {
					portOverride, portErr = portFromEndpoint(endpointConfig.ListenAddress)
				}
			}
			if portOverride == 0 && portErr != nil {
				logger.WithValues("receiver", componentName).Error(portErr, "couldn't parse the endpoint's port and no default port set")
				return []corev1.ServicePort{}, portErr
			}
			svcPort.Name = naming.PortName(fmt.Sprintf("%s-%s", componentName, protocolName), portOverride)
			ports = append(ports, ConstructServicePort(svcPort, portOverride))
		}
	}
	logger.Info("test", protocolKey, protocols)
	return ports, nil
}
