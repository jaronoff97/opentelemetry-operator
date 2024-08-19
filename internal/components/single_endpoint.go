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

package components

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
)

var (
	_ Parser = &GenericParser[*SingleEndpointConfig]{}
)

// SingleEndpointConfig represents the minimal struct for a given YAML configuration input containing either
// endpoint or listen_address.
type SingleEndpointConfig struct {
	Endpoint      string `mapstructure:"endpoint,omitempty"`
	ListenAddress string `mapstructure:"listen_address,omitempty"`
}

func (g *SingleEndpointConfig) GetPortNumOrDefault(logger logr.Logger, p int32) int32 {
	num, err := g.GetPortNum()
	if err != nil {
		logger.V(3).Info("no port set, using default: %d", p)
		return p
	}
	return num
}

// GetPortNum attempts to get the port for the given config. If it cannot, the UnsetPort and the given missingPortError
// are returned.
func (g *SingleEndpointConfig) GetPortNum() (int32, error) {
	if len(g.Endpoint) > 0 {
		return PortFromEndpoint(g.Endpoint)
	} else if len(g.ListenAddress) > 0 {
		return PortFromEndpoint(g.ListenAddress)
	}
	return UnsetPort, PortNotFoundErr
}

func ParseSingleEndpointSilent(logger logr.Logger, name string, o *Option, singleEndpointConfig *SingleEndpointConfig) ([]corev1.ServicePort, error) {
	return internalParseSingleEndpoint(logger, name, true, o, singleEndpointConfig)
}

func ParseSingleEndpoint(logger logr.Logger, name string, o *Option, singleEndpointConfig *SingleEndpointConfig) ([]corev1.ServicePort, error) {
	return internalParseSingleEndpoint(logger, name, false, o, singleEndpointConfig)
}

func internalParseSingleEndpoint(logger logr.Logger, name string, failSilently bool, o *Option, singleEndpointConfig *SingleEndpointConfig) ([]corev1.ServicePort, error) {
	if singleEndpointConfig == nil {
		return nil, nil
	}
	if _, err := singleEndpointConfig.GetPortNum(); err != nil && o.port == UnsetPort {
		if failSilently {
			logger.WithValues("receiver", o.name).V(4).Info("couldn't parse the endpoint's port and no default port set", "error", err)
			err = nil
		} else {
			logger.WithValues("receiver", o.name).Error(err, "couldn't parse the endpoint's port and no default port set")
		}
		return []corev1.ServicePort{}, err
	}
	port := singleEndpointConfig.GetPortNumOrDefault(logger, o.port)
	svcPort := o.GetServicePort()
	svcPort.Name = naming.PortName(name, port)
	return []corev1.ServicePort{ConstructServicePort(svcPort, port)}, nil
}

func NewSinglePortParser(name string, port int32, opts ...PortBuilderOption) *GenericParser[*SingleEndpointConfig] {
	return NewGenericParser(name, port, ParseSingleEndpoint, nil, opts...)
}

// NewSilentSinglePortParser returns a ParseSingleEndpoint that errors silently on failure to find a port.
func NewSilentSinglePortParser(name string, port int32, opts ...PortBuilderOption) *GenericParser[*SingleEndpointConfig] {
	return NewGenericParser(name, port, ParseSingleEndpointSilent, nil, opts...)
}
