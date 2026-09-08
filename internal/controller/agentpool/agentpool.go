// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package agentpool

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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/agentpool/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig           = "cannot get Azure DevOps client config"
	errNewClient           = "cannot create new Azure DevOps agent pool client"
	errMissingName         = "spec.forProvider.name is required"
	errMissingAgentPoolID  = "agent pool id is required"
	errParseAgentPoolID    = "cannot parse agent pool id"
	errGetAgentPool        = "cannot get agent pool"
	errCreateAgentPool     = "cannot create agent pool"
	errCreateAgentPoolNoID = "created agent pool did not include an id"
	errUpdateAgentPool     = "cannot update agent pool"
	errDeleteAgentPool     = "cannot delete agent pool"
)

// SetupGated adds a controller that reconciles AgentPool managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup AgentPool controller"))
		}
	}, v1alpha1.AgentPoolGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles AgentPool managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.AgentPoolGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.AgentPool](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.AgentPoolList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.AgentPoolList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.AgentPoolGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.AgentPool{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves cr's ProviderConfig via the shared azuredevops client
// package and uses it to construct the TaskAgent API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.AgentPool) (managed.TypedExternalClient[*v1alpha1.AgentPool], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	agentPoolClient, err := newAgentPoolClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{agentPools: agentPoolClient}, nil
}

type external struct {
	agentPools AgentPoolClient
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.AgentPool) (managed.ExternalObservation, error) {
	agentPoolID, ok, err := agentPoolIDFrom(cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetAgentPool)
	}
	if !ok {
		return managed.ExternalObservation{}, nil
	}

	agentPool, err := e.getAgentPool(ctx, agentPoolID)
	if azuredevops.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetAgentPool)
	}

	cr.Status.AtProvider = observationFromAgentPool(agentPool)
	cr.SetConditions(xpv2.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, agentPool),
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.AgentPool) (managed.ExternalCreation, error) {
	payload, err := desiredAgentPoolCreate(cr.Spec.ForProvider)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	var created *taskagent.TaskAgentPool
	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var createErr error
		created, createErr = e.agentPools.AddAgentPool(ctx, taskagent.AddAgentPoolArgs{Pool: payload})
		return createErr
	})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateAgentPool)
	}
	if created == nil || created.Id == nil {
		return managed.ExternalCreation{}, errors.New(errCreateAgentPoolNoID)
	}

	meta.SetExternalName(cr, strconv.Itoa(*created.Id))
	cr.Status.AtProvider = observationFromAgentPool(created)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *v1alpha1.AgentPool) (managed.ExternalUpdate, error) {
	agentPoolID, ok, err := agentPoolIDFrom(cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateAgentPool)
	}
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errMissingAgentPoolID)
	}

	current, err := e.getAgentPool(ctx, agentPoolID)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errGetAgentPool)
	}

	patch, err := desiredAgentPoolUpdate(cr.Spec.ForProvider, current)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	if patch == nil {
		cr.Status.AtProvider = observationFromAgentPool(current)
		return managed.ExternalUpdate{}, nil
	}

	var updated *taskagent.TaskAgentPool
	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var updateErr error
		updated, updateErr = e.agentPools.UpdateAgentPool(ctx, taskagent.UpdateAgentPoolArgs{
			Pool:   patch,
			PoolId: intPtr(agentPoolID),
		})
		return updateErr
	})
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateAgentPool)
	}

	cr.Status.AtProvider = observationFromAgentPool(updated)
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.AgentPool) (managed.ExternalDelete, error) {
	agentPoolID, ok, err := agentPoolIDFrom(cr)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteAgentPool)
	}
	if !ok {
		return managed.ExternalDelete{}, nil
	}

	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		return e.agentPools.DeleteAgentPool(ctx, taskagent.DeleteAgentPoolArgs{PoolId: intPtr(agentPoolID)})
	})
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(resource.Ignore(azuredevops.IsNotFound, err), errDeleteAgentPool)
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

func (e *external) getAgentPool(ctx context.Context, agentPoolID int) (*taskagent.TaskAgentPool, error) {
	var agentPool *taskagent.TaskAgentPool
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var getErr error
		agentPool, getErr = e.agentPools.GetAgentPool(ctx, taskagent.GetAgentPoolArgs{PoolId: intPtr(agentPoolID)})
		return getErr
	})
	return agentPool, err
}

func agentPoolIDFrom(cr *v1alpha1.AgentPool) (int, bool, error) {
	id := meta.GetExternalName(cr)
	if id == "" {
		id = cr.Status.AtProvider.ID
	}
	if id == "" {
		return 0, false, nil
	}
	parsed, err := strconv.Atoi(id)
	if err != nil {
		return 0, false, errors.Wrap(err, errParseAgentPoolID)
	}
	return parsed, true, nil
}

func desiredAgentPoolCreate(p v1alpha1.AgentPoolParameters) (*taskagent.TaskAgentPool, error) {
	if p.Name == "" {
		return nil, errors.New(errMissingName)
	}
	payload := &taskagent.TaskAgentPool{
		Name:     stringPtr(p.Name),
		IsHosted: boolPtr(desiredIsHosted(p)),
	}
	if p.AutoProvision != nil {
		payload.AutoProvision = boolPtr(*p.AutoProvision)
	}
	if p.AutoUpdate != nil {
		payload.AutoUpdate = boolPtr(*p.AutoUpdate)
	}
	return payload, nil
}

func desiredAgentPoolUpdate(desired v1alpha1.AgentPoolParameters, current *taskagent.TaskAgentPool) (*taskagent.TaskAgentPool, error) {
	if desired.Name == "" {
		return nil, errors.New(errMissingName)
	}

	patch := &taskagent.TaskAgentPool{}
	changed := false

	if valueOrEmpty(current.Name) != desired.Name {
		patch.Name = stringPtr(desired.Name)
		changed = true
	}
	if valueOrFalse(current.IsHosted) != desiredIsHosted(desired) {
		patch.IsHosted = boolPtr(desiredIsHosted(desired))
		changed = true
	}
	if desired.AutoProvision != nil && valueOrFalse(current.AutoProvision) != *desired.AutoProvision {
		patch.AutoProvision = boolPtr(*desired.AutoProvision)
		changed = true
	}
	if desired.AutoUpdate != nil && valueOrFalse(current.AutoUpdate) != *desired.AutoUpdate {
		patch.AutoUpdate = boolPtr(*desired.AutoUpdate)
		changed = true
	}

	if !changed {
		return nil, nil
	}
	return patch, nil
}

func observationFromAgentPool(agentPool *taskagent.TaskAgentPool) v1alpha1.AgentPoolObservation {
	observation := v1alpha1.AgentPoolObservation{}
	if agentPool == nil {
		return observation
	}
	if agentPool.Id != nil {
		observation.ID = strconv.Itoa(*agentPool.Id)
	}
	if agentPool.Size != nil {
		observation.Size = *agentPool.Size
	}
	return observation
}

func isUpToDate(desired v1alpha1.AgentPoolParameters, current *taskagent.TaskAgentPool) bool {
	if current == nil {
		return false
	}
	if valueOrEmpty(current.Name) != desired.Name {
		return false
	}
	if valueOrFalse(current.IsHosted) != desiredIsHosted(desired) {
		return false
	}
	if desired.AutoProvision != nil && valueOrFalse(current.AutoProvision) != *desired.AutoProvision {
		return false
	}
	if desired.AutoUpdate != nil && valueOrFalse(current.AutoUpdate) != *desired.AutoUpdate {
		return false
	}
	return true
}

func desiredIsHosted(p v1alpha1.AgentPoolParameters) bool {
	if p.IsHosted == nil {
		return false
	}
	return *p.IsHosted
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func valueOrFalse(v *bool) bool {
	if v == nil {
		return false
	}
	return *v
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}
