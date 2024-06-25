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
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/open-telemetry/opentelemetry-operator/pkg/constants"
)

var (
	GrpcProtocol          = "grpc"
	HttpProtocol          = "http"
	UnsetPort       int32 = 0
	PortNotFoundErr       = errors.New("port should not be empty")
)

type PortRetriever interface {
	GetPortNum() (int32, error)
	GetPortNumOrDefault(logr.Logger, int32) int32
}

type PortBuilderOption func(*corev1.ServicePort)

func WithTargetPort(targetPort int32) PortBuilderOption {
	return func(servicePort *corev1.ServicePort) {
		servicePort.TargetPort = intstr.FromInt32(targetPort)
	}
}

func WithAppProtocol(proto *string) PortBuilderOption {
	return func(servicePort *corev1.ServicePort) {
		servicePort.AppProtocol = proto
	}
}

func WithProtocol(proto corev1.Protocol) PortBuilderOption {
	return func(servicePort *corev1.ServicePort) {
		servicePort.Protocol = proto
	}
}

// ComponentType returns the type for a given component name.
// components have a name like:
// - mycomponent/custom
// - mycomponent
// we extract the "mycomponent" part and see if we have a parser for the component.
func ComponentType(name string) string {
	if strings.Contains(name, "/") {
		return name[:strings.Index(name, "/")]
	}
	return name
}

func PortFromEndpoint(endpoint string) (int32, error) {
	var err error
	var port int64

	r := regexp.MustCompile(":[0-9]+")

	if r.MatchString(endpoint) {
		portStr := r.FindString(endpoint)
		cleanedPortStr := strings.Replace(portStr, ":", "", -1)
		port, err = strconv.ParseInt(cleanedPortStr, 10, 32)

		if err != nil {
			return UnsetPort, err
		}
	}

	if port == 0 {
		return UnsetPort, PortNotFoundErr
	}

	return int32(port), err
}

type SinglePortBuilder func(name string, port int32, opts ...PortBuilderOption) ComponentPortParser
type PortsRetriever func(logger logr.Logger, name string, config interface{}) ([]corev1.ServicePort, error)

type ComponentPortParser interface {
	// Ports returns the service ports parsed based on the component's configuration where name is the component's name
	// of the form "name" or "type/name"
	Ports(logger logr.Logger, name string, config interface{}) ([]corev1.ServicePort, error)

	// ParserType returns the type of this parser
	ParserType() string

	// ParserName is an internal name for the parser
	ParserName() string
}

// registry holds a record of all known receiver parsers.
var registry = make(map[constants.ComponentType]map[string]ComponentPortParser)

// defaulter holds a generator for a default parser
var defaulter = make(map[constants.ComponentType]SinglePortBuilder)

// RegisterDefaulter registers a default ports cType for a given component type
func RegisterDefaulter(cType constants.ComponentType, p SinglePortBuilder) {
	defaulter[cType] = p
}

// Register adds a new parser builder to the list of known builders.
func Register(cType constants.ComponentType, p ComponentPortParser) {
	if registry[cType] == nil {
		registry[cType] = make(map[string]ComponentPortParser)
	}
	registry[cType][p.ParserType()] = p
}

// IsRegistered checks whether a parser is registered with the given name.
func IsRegistered(cType constants.ComponentType, name string) bool {
	if _, ok := registry[cType]; !ok {
		return false
	}
	_, ok := registry[cType][ComponentType(name)]
	return ok
}

// PortsFor retrieves the parser for the given name and gets the ports from the given config
func PortsFor(logger logr.Logger, cType constants.ComponentType, name string, config interface{}) ([]corev1.ServicePort, error) {
	return ParserFor(cType, name).Ports(logger, name, config)
}

// ParserFor returns a parser builder for the given name and type.
func ParserFor(cType constants.ComponentType, name string) ComponentPortParser {
	if IsRegistered(cType, name) {
		return registry[cType][ComponentType(name)]
	}
	defaulter, ok := defaulter[cType]
	if !ok {
		// give a default if nothing is set
		return NewSinglePortParser(ComponentType(name), UnsetPort)
	}
	return defaulter(ComponentType(name), UnsetPort)
}

func ConstructServicePort(current *corev1.ServicePort, port int32) corev1.ServicePort {
	return corev1.ServicePort{
		Name:        current.Name,
		Port:        port,
		TargetPort:  current.TargetPort,
		NodePort:    current.NodePort,
		AppProtocol: current.AppProtocol,
		Protocol:    current.Protocol,
	}
}

func GetPortsForConfig(logger logr.Logger, config map[string]interface{}, cType constants.ComponentType) ([]corev1.ServicePort, error) {
	var ports []corev1.ServicePort
	for componentName, componentDef := range config {
		if parsedPorts, err := PortsFor(logger, cType, componentName, componentDef); err != nil {
			return nil, err
		} else {
			ports = append(ports, parsedPorts...)
		}
	}
	return ports, nil
}
