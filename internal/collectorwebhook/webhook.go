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

package collectorwebhook

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/internal/config"
	"github.com/open-telemetry/opentelemetry-operator/internal/manifests"
	"github.com/open-telemetry/opentelemetry-operator/internal/manifests/collector"
)

var (
	_ admission.CustomValidator = &Webhook{}
	_ admission.CustomDefaulter = &Webhook{}
)

type Webhook struct {
	logger logr.Logger
	c      client.Client
	cfg    config.Config
}

func (c Webhook) Default(ctx context.Context, obj runtime.Object) error {
	otelcol, ok := obj.(*v1alpha1.OpenTelemetryCollector)
	if !ok {
		return fmt.Errorf("expected an OpenTelemetryCollector, received %T", obj)
	}
	otelcol.Default()
	return nil
}

func (c Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	otelcol, ok := obj.(*v1alpha1.OpenTelemetryCollector)
	if !ok {
		return nil, fmt.Errorf("expected an OpenTelemetryCollector, received %T", obj)
	}
	warnings, err = otelcol.ValidateCRDSpec()
	if err != nil {
		return
	}
	if otelcol.Spec.RunValidationJob {
		err = c.runValidationJob(ctx, otelcol)
	}
	return
}

func (c Webhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (warnings admission.Warnings, err error) {
	otelcol, ok := newObj.(*v1alpha1.OpenTelemetryCollector)
	if !ok {
		return nil, fmt.Errorf("expected an OpenTelemetryCollector, received %T", newObj)
	}
	warnings, err = otelcol.ValidateCRDSpec()
	if err != nil {
		return
	}
	return
}

func (c Webhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	otelcol, ok := obj.(*v1alpha1.OpenTelemetryCollector)
	if !ok || otelcol == nil {
		return nil, fmt.Errorf("expected an OpenTelemetryCollector, received %T", obj)
	}
	warnings, err = otelcol.ValidateCRDSpec()
	if err != nil {
		return
	}
	return
}

func (c Webhook) runValidationJob(ctx context.Context, otelcol *v1alpha1.OpenTelemetryCollector) error {
	params := manifests.Params{
		Config:  c.cfg,
		OtelCol: *otelcol,
		Log:     c.logger,
	}
	desiredObjects, err := collector.BuildValidation(params)
	if err != nil {
		return err
	}
	var errs []error
	for _, desired := range desiredObjects {
		err := c.c.Create(ctx, desired)
		if err != nil {
			errs = append(errs, err)
			continue
		}
	}
	return errors.Join(errs...)
}

func SetupCollectorValidatingWebhookWithManager(mgr controllerruntime.Manager, cfg config.Config) error {
	cvw := &Webhook{
		c:      mgr.GetClient(),
		logger: mgr.GetLogger().WithValues("handler", "Webhook"),
		cfg:    cfg,
	}
	return controllerruntime.NewWebhookManagedBy(mgr).
		For(&v1alpha1.OpenTelemetryCollector{}).
		WithValidator(cvw).
		WithDefaulter(cvw).
		Complete()
}
