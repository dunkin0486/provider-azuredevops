// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package agentqueue

import (
	"context"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/agentqueue/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig        = "cannot get Azure DevOps client config"
	errNewClient        = "cannot create new Azure DevOps agent queue client"
	errMissingProjectID = "spec.forProvider.projectId is required"
	errMissingPoolID    = "spec.forProvider.agentPoolId is required"
	errParseQueueID     = "cannot parse agent queue id"
	errParsePoolID      = "cannot parse agent pool id"
	errGetQueue         = "cannot get agent queue"
	errCreateQueue      = "cannot create agent queue"
	errCreateQueueNoID  = "created agent queue did not include an id"
	errDeleteQueue      = "cannot delete agent queue"
)

// SetupGated adds a controller that reconciles AgentQueue managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup AgentQueue controller"))
		}
	}, v1alpha1.AgentQueueGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles AgentQueue managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.AgentQueueGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.AgentQueue](&connector{kube: mgr.GetClient()}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))), //nolint:staticcheck // TODO(jbw976) Crossplane needs to update to the new events API, see https://github.com/crossplane/crossplane/issues/7152
	}

	if o.Features.Enabled(feature.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}

	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}

	if o.MetricOptions != nil && o.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.AgentQueueList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.AgentQueueList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.AgentQueueGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.AgentQueue{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves cr's ProviderConfig via the shared azuredevops client
// package and uses it to construct the TaskAgent API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.AgentQueue) (managed.TypedExternalClient[*v1alpha1.AgentQueue], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	agentQueueClient, err := newAgentQueueClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{queues: agentQueueClient}, nil
}

type external struct {
	queues AgentQueueClient
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.AgentQueue) (managed.ExternalObservation, error) {
	queueID, ok, err := agentQueueIDFrom(cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetQueue)
	}
	if !ok {
		return managed.ExternalObservation{}, nil
	}
	if cr.Spec.ForProvider.ProjectID == "" {
		return managed.ExternalObservation{}, errors.New(errMissingProjectID)
	}

	queue, err := e.getQueue(ctx, cr.Spec.ForProvider.ProjectID, queueID)
	if azuredevops.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetQueue)
	}

	cr.Status.AtProvider = observationFromQueue(queue)
	cr.SetConditions(xpv2.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, queue),
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.AgentQueue) (managed.ExternalCreation, error) {
	p := cr.Spec.ForProvider
	if p.ProjectID == "" {
		return managed.ExternalCreation{}, errors.New(errMissingProjectID)
	}
	if p.AgentPoolID == "" {
		return managed.ExternalCreation{}, errors.New(errMissingPoolID)
	}

	poolID, err := strconv.Atoi(p.AgentPoolID)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errParsePoolID)
	}

	payload := &taskagent.TaskAgentQueue{
		Pool: &taskagent.TaskAgentPoolReference{Id: intPtr(poolID)},
	}
	if p.Name != "" {
		payload.Name = stringPtr(p.Name)
	}

	var created *taskagent.TaskAgentQueue
	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var createErr error
		created, createErr = e.queues.AddAgentQueue(ctx, taskagent.AddAgentQueueArgs{
			Queue:              payload,
			Project:            stringPtr(p.ProjectID),
			AuthorizePipelines: p.AuthorizePipelines,
		})
		return createErr
	})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateQueue)
	}
	if created == nil || created.Id == nil {
		return managed.ExternalCreation{}, errors.New(errCreateQueueNoID)
	}

	meta.SetExternalName(cr, strconv.Itoa(*created.Id))
	cr.Status.AtProvider = observationFromQueue(created)
	return managed.ExternalCreation{}, nil
}

// Update is a no-op: agent queues have no mutable fields in the Azure DevOps
// API once created (renaming or re-pointing a queue requires delete/recreate).
func (e *external) Update(_ context.Context, cr *v1alpha1.AgentQueue) (managed.ExternalUpdate, error) {
	if _, ok, err := agentQueueIDFrom(cr); err != nil {
		return managed.ExternalUpdate{}, err
	} else if !ok {
		return managed.ExternalUpdate{}, errors.New(errMissingPoolID)
	}
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.AgentQueue) (managed.ExternalDelete, error) {
	queueID, ok, err := agentQueueIDFrom(cr)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteQueue)
	}
	if !ok {
		return managed.ExternalDelete{}, nil
	}

	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		return e.queues.DeleteAgentQueue(ctx, taskagent.DeleteAgentQueueArgs{
			QueueId: intPtr(queueID),
			Project: stringPtr(cr.Spec.ForProvider.ProjectID),
		})
	})
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(resource.Ignore(azuredevops.IsNotFound, err), errDeleteQueue)
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

func (e *external) getQueue(ctx context.Context, projectID string, queueID int) (*taskagent.TaskAgentQueue, error) {
	var queue *taskagent.TaskAgentQueue
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var getErr error
		queue, getErr = e.queues.GetAgentQueue(ctx, taskagent.GetAgentQueueArgs{
			QueueId: intPtr(queueID),
			Project: stringPtr(projectID),
		})
		return getErr
	})
	return queue, err
}

func agentQueueIDFrom(cr *v1alpha1.AgentQueue) (int, bool, error) {
	id := meta.GetExternalName(cr)
	if id == "" {
		id = cr.Status.AtProvider.ID
	}
	if id == "" {
		return 0, false, nil
	}
	parsed, err := strconv.Atoi(id)
	if err != nil {
		return 0, false, errors.Wrap(err, errParseQueueID)
	}
	return parsed, true, nil
}

func observationFromQueue(queue *taskagent.TaskAgentQueue) v1alpha1.AgentQueueObservation {
	observation := v1alpha1.AgentQueueObservation{}
	if queue == nil {
		return observation
	}
	if queue.Id != nil {
		observation.ID = strconv.Itoa(*queue.Id)
	}
	if queue.Name != nil {
		observation.Name = *queue.Name
	}
	if queue.Pool != nil && queue.Pool.Id != nil {
		observation.PoolID = strconv.Itoa(*queue.Pool.Id)
	}
	return observation
}

// isUpToDate compares only the pool linkage, since Name is server-assigned
// (mirroring the pool's name when unset) and there is no update API for
// agent queues.
func isUpToDate(desired v1alpha1.AgentQueueParameters, current *taskagent.TaskAgentQueue) bool {
	if current == nil || current.Pool == nil || current.Pool.Id == nil {
		return false
	}
	return strconv.Itoa(*current.Pool.Id) == desired.AgentPoolID
}

func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
