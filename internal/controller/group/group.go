// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package group

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/graph"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/webapi"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/group/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig     = "cannot get Azure DevOps client config"
	errNewClient     = "cannot create new Azure DevOps graph client"
	errGetGroup      = "cannot get group"
	errCreateGroup   = "cannot create group"
	errUpdateGroup   = "cannot update group"
	errDeleteGroup   = "cannot delete group"
	errMissingExtern = "external-name annotation is required to identify the group descriptor"
)

// SetupGated adds a controller that reconciles Group managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup Group controller"))
		}
	}, v1alpha1.GroupGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles Group managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.GroupGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.Group](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.GroupList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.GroupList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.GroupGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Group{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves cr's ProviderConfig via the shared azuredevops client
// package and uses it to construct the Graph API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.Group) (managed.TypedExternalClient[*v1alpha1.Group], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	groupClient, err := newGroupClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{groups: groupClient}, nil
}

type external struct {
	groups GroupClient
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.Group) (managed.ExternalObservation, error) {
	descriptor := meta.GetExternalName(cr)
	if descriptor == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	grp, err := e.getGroup(ctx, descriptor)
	if azuredevops.IsNotFound(err) {
		cr.Status.AtProvider = v1alpha1.GroupObservation{}
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetGroup)
	}

	cr.Status.AtProvider = observationFromGroup(grp)
	cr.SetConditions(xpv2.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, grp),
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.Group) (managed.ExternalCreation, error) {
	p := cr.Spec.ForProvider

	creationContext := &graph.GraphGroupVstsCreationContext{
		DisplayName: stringPtr(p.DisplayName),
	}
	if p.Description != "" {
		creationContext.Description = stringPtr(p.Description)
	}

	args := graph.CreateGroupVstsArgs{
		CreationContext: creationContext,
	}
	if p.ScopeDescriptor != "" {
		args.ScopeDescriptor = stringPtr(p.ScopeDescriptor)
	}

	var grp *graph.GraphGroup
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var createErr error
		grp, createErr = e.groups.CreateGroupVsts(ctx, args)
		return createErr
	})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateGroup)
	}
	if grp == nil || grp.Descriptor == nil || *grp.Descriptor == "" {
		return managed.ExternalCreation{}, errors.New(errCreateGroup)
	}

	meta.SetExternalName(cr, *grp.Descriptor)
	cr.Status.AtProvider = observationFromGroup(grp)

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *v1alpha1.Group) (managed.ExternalUpdate, error) {
	descriptor := meta.GetExternalName(cr)
	if descriptor == "" {
		return managed.ExternalUpdate{}, errors.New(errMissingExtern)
	}

	// patchForUpdate always sends a full replace of displayName and
	// description; Observe's isUpToDate check already ensures Update is
	// only invoked when at least one of them has actually changed.
	patch := patchForUpdate(cr.Spec.ForProvider)

	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		_, updateErr := e.groups.UpdateGroup(ctx, graph.UpdateGroupArgs{
			GroupDescriptor: stringPtr(descriptor),
			PatchDocument:   &patch,
		})
		return updateErr
	})
	return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateGroup)
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.Group) (managed.ExternalDelete, error) {
	descriptor := meta.GetExternalName(cr)
	if descriptor == "" {
		return managed.ExternalDelete{}, nil
	}

	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		return e.groups.DeleteGroup(ctx, graph.DeleteGroupArgs{
			GroupDescriptor: stringPtr(descriptor),
		})
	})
	return managed.ExternalDelete{}, errors.Wrap(resource.Ignore(azuredevops.IsNotFound, err), errDeleteGroup)
}

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

func (e *external) getGroup(ctx context.Context, descriptor string) (*graph.GraphGroup, error) {
	var grp *graph.GraphGroup
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var getErr error
		grp, getErr = e.groups.GetGroup(ctx, graph.GetGroupArgs{
			GroupDescriptor: stringPtr(descriptor),
		})
		return getErr
	})
	return grp, err
}

func observationFromGroup(grp *graph.GraphGroup) v1alpha1.GroupObservation {
	obs := v1alpha1.GroupObservation{}
	if grp == nil {
		return obs
	}
	if grp.Descriptor != nil {
		obs.Descriptor = *grp.Descriptor
	}
	if grp.OriginId != nil {
		obs.OriginID = *grp.OriginId
	}
	if grp.Origin != nil {
		obs.Origin = *grp.Origin
	}
	if grp.Url != nil {
		obs.URL = *grp.Url
	}
	return obs
}

// isUpToDate compares the mutable fields of the desired Group against the
// observed GraphGroup. ScopeDescriptor is write-once (only usable at
// creation) and has no corresponding observable field, so it's excluded.
func isUpToDate(p v1alpha1.GroupParameters, grp *graph.GraphGroup) bool {
	if grp == nil {
		return false
	}
	currentName := ""
	if grp.DisplayName != nil {
		currentName = *grp.DisplayName
	}
	currentDescription := ""
	if grp.Description != nil {
		currentDescription = *grp.Description
	}
	return p.DisplayName == currentName && p.Description == currentDescription
}

func patchForUpdate(p v1alpha1.GroupParameters) []webapi.JsonPatchOperation {
	return []webapi.JsonPatchOperation{
		{
			Op:    operationPtr(webapi.OperationValues.Replace),
			Path:  stringPtr("/displayName"),
			Value: p.DisplayName,
		},
		{
			Op:    operationPtr(webapi.OperationValues.Replace),
			Path:  stringPtr("/description"),
			Value: p.Description,
		},
	}
}

func stringPtr(s string) *string {
	return &s
}

func operationPtr(o webapi.Operation) *webapi.Operation {
	return &o
}
