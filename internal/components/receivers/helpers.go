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

package receivers

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/internal/components"
)

// registry holds a record of all known receiver parsers.
var registry = make(map[string]components.Parser)

// Register adds a new parser builder to the list of known builders.
func Register(name string, p components.Parser) {
	registry[name] = p
}

// IsRegistered checks whether a parser is registered with the given name.
func IsRegistered(name string) bool {
	_, ok := registry[components.ComponentType(name)]
	return ok
}

// ReceiverFor returns a parser builder for the given exporter name.
func ReceiverFor(name string) components.Parser {
	if parser, ok := registry[components.ComponentType(name)]; ok {
		return parser
	}
	return components.NewSilentSinglePortParser(components.ComponentType(name), components.UnsetPort)
}

// NewScraperParser is an instance of a generic parser that returns nothing when called and never fails.
func NewScraperParser(name string) *components.GenericParser[any] {
	return components.NewGenericParser[any](name, components.UnsetPort)
}

var (
	componentParsers = []components.Parser{
		components.NewMultiPortReceiver("otlp",
			components.WithPortMapping(
				"grpc",
				4317,
				components.WithAppProtocol[*components.MultiProtocolEndpointConfig](&components.GrpcProtocol),
				components.WithTargetPort[*components.MultiProtocolEndpointConfig](4317),
			), components.WithPortMapping(
				"http",
				4318,
				components.WithAppProtocol[*components.MultiProtocolEndpointConfig](&components.HttpProtocol),
				components.WithTargetPort[*components.MultiProtocolEndpointConfig](4318),
			),
		),
		components.NewMultiPortReceiver("skywalking",
			components.WithPortMapping(components.GrpcProtocol, 11800,
				components.WithTargetPort[*components.MultiProtocolEndpointConfig](11800),
				components.WithAppProtocol[*components.MultiProtocolEndpointConfig](&components.GrpcProtocol),
			),
			components.WithPortMapping(components.HttpProtocol, 12800,
				components.WithTargetPort[*components.MultiProtocolEndpointConfig](12800),
				components.WithAppProtocol[*components.MultiProtocolEndpointConfig](&components.HttpProtocol),
			)),
		components.NewMultiPortReceiver("jaeger",
			components.WithPortMapping(components.GrpcProtocol, 14250,
				components.WithTargetPort[*components.MultiProtocolEndpointConfig](14250),
				components.WithProtocol[*components.MultiProtocolEndpointConfig](corev1.ProtocolTCP),
				components.WithAppProtocol[*components.MultiProtocolEndpointConfig](&components.GrpcProtocol),
			),
			components.WithPortMapping("thrift_http", 14268,
				components.WithTargetPort[*components.MultiProtocolEndpointConfig](14268),
				components.WithProtocol[*components.MultiProtocolEndpointConfig](corev1.ProtocolTCP),
				components.WithAppProtocol[*components.MultiProtocolEndpointConfig](&components.HttpProtocol),
			),
			components.WithPortMapping("thrift_compact", 6831,
				components.WithTargetPort[*components.MultiProtocolEndpointConfig](6831),
				components.WithProtocol[*components.MultiProtocolEndpointConfig](corev1.ProtocolUDP),
			),
			components.WithPortMapping("thrift_binary", 6832,
				components.WithTargetPort[*components.MultiProtocolEndpointConfig](6832),
				components.WithProtocol[*components.MultiProtocolEndpointConfig](corev1.ProtocolUDP),
			),
		),
		components.NewMultiPortReceiver("loki",
			components.WithPortMapping(components.GrpcProtocol, 9095,
				components.WithTargetPort[*components.MultiProtocolEndpointConfig](9095),
				components.WithAppProtocol[*components.MultiProtocolEndpointConfig](&components.GrpcProtocol),
			),
			components.WithPortMapping(components.HttpProtocol, 3100,
				components.WithTargetPort[*components.MultiProtocolEndpointConfig](3100),
				components.WithAppProtocol[*components.MultiProtocolEndpointConfig](&components.HttpProtocol),
			),
		),
		components.NewSinglePortParser("awsxray", 2000, components.WithTargetPort[*components.SingleEndpointConfig](2000)),
		components.NewSinglePortParser("carbon", 2003, components.WithTargetPort[*components.SingleEndpointConfig](2003)),
		components.NewSinglePortParser("collectd", 8081, components.WithTargetPort[*components.SingleEndpointConfig](8081)),
		components.NewSinglePortParser("fluentforward", 8006, components.WithTargetPort[*components.SingleEndpointConfig](8006)),
		components.NewSinglePortParser("influxdb", 8086, components.WithTargetPort[*components.SingleEndpointConfig](8086)),
		components.NewSinglePortParser("opencensus", 55678, components.WithAppProtocol[*components.SingleEndpointConfig](nil), components.WithTargetPort[*components.SingleEndpointConfig](55678)),
		components.NewSinglePortParser("sapm", 7276, components.WithTargetPort[*components.SingleEndpointConfig](7276)),
		components.NewSinglePortParser("signalfx", 9943, components.WithTargetPort[*components.SingleEndpointConfig](9943)),
		components.NewSinglePortParser("splunk_hec", 8088, components.WithTargetPort[*components.SingleEndpointConfig](8088)),
		components.NewSinglePortParser("statsd", 8125, components.WithProtocol[*components.SingleEndpointConfig](corev1.ProtocolUDP), components.WithTargetPort[*components.SingleEndpointConfig](8125)),
		components.NewSinglePortParser("tcplog", components.UnsetPort, components.WithProtocol[*components.SingleEndpointConfig](corev1.ProtocolTCP)),
		components.NewSinglePortParser("udplog", components.UnsetPort, components.WithProtocol[*components.SingleEndpointConfig](corev1.ProtocolUDP)),
		components.NewSinglePortParser("wavefront", 2003, components.WithTargetPort[*components.SingleEndpointConfig](2003)),
		components.NewSinglePortParser("zipkin", 9411, components.WithAppProtocol[*components.SingleEndpointConfig](&components.HttpProtocol), components.WithProtocol[*components.SingleEndpointConfig](corev1.ProtocolTCP), components.WithTargetPort[*components.SingleEndpointConfig](3100)),
		NewScraperParser("prometheus"),
		NewScraperParser("kubeletstats"),
		NewScraperParser("sshcheck"),
		NewScraperParser("cloudfoundry"),
		NewScraperParser("vcenter"),
		NewScraperParser("oracledb"),
		NewScraperParser("snmp"),
		NewScraperParser("googlecloudpubsub"),
		NewScraperParser("chrony"),
		NewScraperParser("jmx"),
		NewScraperParser("podman_stats"),
		NewScraperParser("pulsar"),
		NewScraperParser("docker_stats"),
		NewScraperParser("aerospike"),
		NewScraperParser("zookeeper"),
		NewScraperParser("prometheus_simple"),
		NewScraperParser("saphana"),
		NewScraperParser("riak"),
		NewScraperParser("redis"),
		NewScraperParser("rabbitmq"),
		NewScraperParser("purefb"),
		NewScraperParser("postgresql"),
		NewScraperParser("nsxt"),
		NewScraperParser("nginx"),
		NewScraperParser("mysql"),
		NewScraperParser("memcached"),
		NewScraperParser("httpcheck"),
		NewScraperParser("haproxy"),
		NewScraperParser("flinkmetrics"),
		NewScraperParser("couchdb"),
		NewScraperParser("filelog"),
	}
)

func init() {
	for _, parser := range componentParsers {
		Register(parser.ParserType(), parser)
	}
}
