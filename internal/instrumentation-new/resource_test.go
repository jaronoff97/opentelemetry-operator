// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceAttributesLayeringAndExistingLocks(t *testing.T) {
	var attrs ResourceAttributes
	attrs.Add(ResourceCR, "k8s.pod.name", "from-cr")
	attrs.Add(ResourceCR, "service.namespace", "from-cr")
	attrs.Add(ResourceKubernetes, "k8s.pod.name", "from-k8s")
	attrs.Add(ResourcePodAnnotation, "deployment.environment", "prod")

	assert.Equal(t,
		"deployment.environment=prod,k8s.pod.name=from-k8s",
		attrs.Build("service.namespace=user"),
	)
}
