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

import corev1 "k8s.io/api/core/v1"

var (
	grpc                   = "grpc"
	http                   = "http"
	unsetPort        int32 = 0
	scraperReceivers       = map[string]struct{}{
		"prometheus":        {},
		"kubeletstats":      {},
		"sshcheck":          {},
		"cloudfoundry":      {},
		"vcenter":           {},
		"oracledb":          {},
		"snmp":              {},
		"googlecloudpubsub": {},
		"chrony":            {},
		"jmx":               {},
		"podman_stats":      {},
		"pulsar":            {},
		"docker_stats":      {},
		"aerospike":         {},
		"zookeeper":         {},
		"prometheus_simple": {},
		"saphana":           {},
		"riak":              {},
		"redis":             {},
		"rabbitmq":          {},
		"purefb":            {},
		"postgresql":        {},
		"nsxt":              {},
		"nginx":             {},
		"mysql":             {},
		"memcached":         {},
		"httpcheck":         {},
		"haproxy":           {},
		"flinkmetrics":      {},
		"couchdb":           {},
	}
	genericReceivers = map[string][]PortBuilderOption{
		"awsxray": {
			WithDefaultPort(2000),
		},
		"carbon": {
			WithDefaultPort(2003),
		},
		"collectd": {
			WithDefaultPort(8081),
		},
		"fluentforward": {
			WithDefaultPort(8006),
		},
		"influxdb": {
			WithDefaultPort(8086),
		},
		"opencensus": {
			WithDefaultPort(55678),
			WithAppProtocol(nil),
		},
		"sapm": {
			WithDefaultPort(7276),
		},
		"signalfx": {
			WithDefaultPort(9943),
		},
		"splunk_hec": {
			WithDefaultPort(8088),
		},
		"statsd": {
			WithDefaultPort(8125),
			WithProtocol(corev1.ProtocolUDP),
		},
		"tcplog": {
			WithDefaultPort(unsetPort),
			WithProtocol(corev1.ProtocolTCP),
		},
		"udplog": {
			WithDefaultPort(unsetPort),
			WithProtocol(corev1.ProtocolUDP),
		},
		"wavefront": {
			WithDefaultPort(2003),
		},
		"zipkin": {
			WithDefaultPort(9411),
			WithAppProtocol(&http), WithProtocol(corev1.ProtocolTCP),
		},
	}
	genericMultiPortReceivers = map[string]map[string][]PortBuilderOption{
		"otlp": {
			grpc: {
				WithDefaultPort(4317),
				WithAppProtocol(&grpc),
				WithTargetPort(4317),
			},
			http: {
				WithDefaultPort(4318),
				WithAppProtocol(&http),
				WithTargetPort(4318),
			},
		},
		"skywalking": {
			grpc: {
				WithDefaultPort(11800),
				WithTargetPort(11800),
				WithAppProtocol(&grpc),
			},
			http: {
				WithDefaultPort(12800),
				WithTargetPort(12800),
				WithAppProtocol(&http),
			},
		},
		"jaeger": {
			grpc: {
				WithDefaultPort(14250),
				WithProtocol(corev1.ProtocolTCP),
				WithAppProtocol(&grpc),
			},
			"thrift_http": {
				WithDefaultPort(14268),
				WithProtocol(corev1.ProtocolTCP),
				WithAppProtocol(&http),
			},
			"thrift_compact": {
				WithDefaultPort(6831),
				WithProtocol(corev1.ProtocolUDP),
			},
			"thrift_binary": {
				WithProtocol(corev1.ProtocolUDP),
				WithDefaultPort(6832),
			},
		},
		"loki": {
			grpc: {
				WithDefaultPort(9095),
				WithTargetPort(9095),
				WithAppProtocol(&grpc),
			},
			http: {
				WithDefaultPort(3100),
				WithTargetPort(3100),
				WithAppProtocol(&http),
			},
		},
	}
)
