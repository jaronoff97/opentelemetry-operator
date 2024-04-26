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
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
)

//func TestParseEndpoint(t *testing.T) {
//	// prepare
//	// there's no parser registered to handle "myreceiver", so, it falls back to the generic parser
//	builder, err := parser.NewSinglePortParser("myreceiver", map[string]interface{}{
//		"endpoint": "0.0.0.0:1234",
//	})
//	assert.NoError(t, err)
//
//	// test
//	ports, err := builder.Ports(logger)
//
//	// verify
//	assert.NoError(t, err)
//	assert.Len(t, ports, 1)
//	assert.EqualValues(t, 1234, ports[0].Port)
//}
//
//func TestFailedToParseEndpoint(t *testing.T) {
//	// prepare
//	// there's no parser registered to handle "myreceiver", so, it falls back to the generic parser
//	builder, err := parser.NewSinglePortParser("myreceiver", map[string]interface{}{
//		"endpoint": "0.0.0.0",
//	})
//	assert.NoError(t, err)
//
//	// test
//	ports, err := builder.Ports(logger)
//
//	// verify
//	assert.Error(t, err)
//	assert.Len(t, ports, 0)
//}

func TestDownstreamParsers(t *testing.T) {
	for _, tt := range []struct {
		desc         string
		receiverName string
		parserName   string
		defaultPort  int
	}{
		{"zipkin", "zipkin", "__zipkin", 9411},
		{"opencensus", "opencensus", "__opencensus", 55678},

		// contrib receivers
		{"carbon", "carbon", "__carbon", 2003},
		{"collectd", "collectd", "__collectd", 8081},
		{"sapm", "sapm", "__sapm", 7276},
		{"signalfx", "signalfx", "__signalfx", 9943},
		{"wavefront", "wavefront", "__wavefront", 2003},
		{"fluentforward", "fluentforward", "__fluentforward", 8006},
		{"statsd", "statsd", "__statsd", 8125},
		{"influxdb", "influxdb", "__influxdb", 8086},
		{"splunk_hec", "splunk_hec", "__splunk_hec", 8088},
		{"awsxray", "awsxray", "__awsxray", 2000},
	} {
		t.Run(tt.receiverName, func(t *testing.T) {
			t.Run("assigns the expected port", func(t *testing.T) {
				// test
				ports, err := SinglePortParser(logr.Discard(), tt.receiverName, nil, genericReceivers[tt.receiverName]...)

				// verify
				assert.NoError(t, err)
				assert.Len(t, ports, 1)
				assert.EqualValues(t, tt.defaultPort, ports[0].Port)
				assert.Equal(t, naming.PortName(tt.receiverName, int32(tt.defaultPort)), ports[0].Name)

			})

			t.Run("allows port to be overridden", func(t *testing.T) {
				cfg := &v1beta1.AnyConfig{
					map[string]interface{}{
						"endpoint": "0.0.0.0:65535",
					},
				}
				ports, err := SinglePortParser(logr.Discard(), tt.receiverName, cfg, genericReceivers[tt.receiverName]...)

				// verify
				assert.NoError(t, err)
				assert.Len(t, ports, 1)
				assert.EqualValues(t, 65535, ports[0].Port)
				assert.Equal(t, naming.PortName(tt.receiverName, int32(tt.defaultPort)), ports[0].Name)
			})
		})
	}
}
