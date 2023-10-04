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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/controllers"
	"github.com/open-telemetry/opentelemetry-operator/internal/config"
	"github.com/open-telemetry/opentelemetry-operator/internal/manifests"
	"github.com/open-telemetry/opentelemetry-operator/internal/manifests/collector"
	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
)

const (
	maxTimeout = 20 * time.Second
	sleepTime  = 1 * time.Second
)

var (
	_ admission.CustomValidator = &Webhook{}
	_ admission.CustomDefaulter = &Webhook{}
)

type Webhook struct {
	logger logr.Logger
	c      client.Client
	cfg    config.Config
	scheme *runtime.Scheme
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
	if otelcol.Spec.RunValidationJob {
		err = c.runValidationJob(ctx, otelcol)
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
	err = controllers.ReconcileDesiredObjects(ctx, c.c, c.logger, otelcol, c.scheme, false, desiredObjects...)
	if err != nil {
		return err
	}
	return c.waitForStatus(ctx, otelcol)
}

func (c Webhook) waitForStatus(ctx context.Context, otelcol *v1alpha1.OpenTelemetryCollector) (err error) {
	logger := c.logger.WithValues("logger", "waiter")
	completed := false
	var timeWaited time.Duration
	configHash := collector.GetConfigMapSHA(otelcol.Spec.Config)
	jobName := naming.Job(otelcol.Name, configHash)
	nsn := types.NamespacedName{Namespace: otelcol.Namespace, Name: jobName}
	logger.Info("attempting to get", "nsn", nsn)
	for !completed {
		if timeWaited > maxTimeout {
			err = fmt.Errorf("%s timed out, check pod logs or disable validation", jobName)
			logger.Error(err, "timed out")
			completed = true
		}
		job := batchv1.Job{}
		err = c.c.Get(ctx, nsn, &job)
		if apierrors.IsNotFound(err) {
			timeWaited += sleepTime
			time.Sleep(sleepTime)
			logger.Info("job not found yet...")
			continue
		}
		if err != nil {
			logger.Error(err, "failed to get")
			return
		}
		logger.Info("showing status...", "status", job.Status)
		if job.Status.Succeeded > 0 {
			completed = true
		}
		if job.Status.Failed > 0 {
			err = fmt.Errorf("%s has failed, check pod logs", job.Name)
			completed = true
		}
		if job.Status.Active == 0 && job.Status.Succeeded == 0 && job.Status.Failed == 0 {
			logger.Info("job hasn't started yet", "job", job.Name)
		}
		if job.Status.Active > 0 {
			logger.Info("job is still running", "job", job.Name)
		}
		// The job hasn't finished yet, sleep to backoff.
		timeWaited += sleepTime
		time.Sleep(sleepTime)
	}
	logger.Info("completed")
	return
}

func SetupCollectorValidatingWebhookWithManager(mgr ctrl.Manager, cfg config.Config) error {
	cvw := &Webhook{
		c:      mgr.GetClient(),
		logger: mgr.GetLogger().WithValues("handler", "Webhook"),
		scheme: mgr.GetScheme(),
		cfg:    cfg,
	}
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.OpenTelemetryCollector{}).
		WithValidator(cvw).
		WithDefaulter(cvw).
		Complete()
}
