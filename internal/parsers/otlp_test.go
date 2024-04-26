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
)

func TestOTLPPortsOverridden(t *testing.T) {
	// prepare
	cfg := &v1beta1.AnyConfig{
		Object: map[string]interface{}{
			"protocols": map[string]interface{}{
				"grpc": map[string]interface{}{
					"endpoint": "0.0.0.0:1234",
				},
				"http": map[string]interface{}{
					"endpoint": "0.0.0.0:1235",
				},
			},
		},
	}

	expectedResults := map[string]struct {
		portNumber int32
		seen       bool
	}{
		"otlp-grpc": {portNumber: 1234},
		"otlp-http": {portNumber: 1235},
	}

	// test
	ports, err := MultiplePortParser(logr.Discard(), "otlp", cfg, genericMultiPortReceivers["otlp"])

	// verify
	assert.NoError(t, err)
	assert.Len(t, ports, len(expectedResults))

	for _, port := range ports {
		r := expectedResults[port.Name]
		r.seen = true
		expectedResults[port.Name] = r
		assert.EqualValues(t, r.portNumber, port.Port)
	}
	for k, v := range expectedResults {
		assert.True(t, v.seen, "the port %s wasn't included in the service ports", k)
	}
}

func TestOTLPExposeDefaultPorts(t *testing.T) {
	// prepare
	cfg := &v1beta1.AnyConfig{
		Object: map[string]interface{}{
			"protocols": map[string]interface{}{
				"grpc": map[string]interface{}{},
				"http": map[string]interface{}{},
			},
		},
	}

	expectedResults := map[string]struct {
		portNumber int32
		seen       bool
	}{
		"otlp-grpc": {portNumber: 4317},
		"otlp-http": {portNumber: 4318},
	}

	// test
	ports, err := MultiplePortParser(logr.Discard(), "otlp", cfg, genericMultiPortReceivers["otlp"])

	// verify
	assert.NoError(t, err)
	assert.Len(t, ports, len(expectedResults))

	for _, port := range ports {
		r := expectedResults[port.Name]
		r.seen = true
		expectedResults[port.Name] = r
		assert.EqualValues(t, r.portNumber, port.Port)
	}
	for k, v := range expectedResults {
		assert.True(t, v.seen, "the port %s wasn't included in the service ports", k)
	}
}
