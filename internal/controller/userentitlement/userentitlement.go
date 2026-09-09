// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package userentitlement

import (
	"context"
	"sort"

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
	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/graph"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/licensing"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/memberentitlementmanagement"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/webapi"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/userentitlement/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig                   = "cannot get Azure DevOps client config"
	errNewClient                   = "cannot create new Azure DevOps member entitlement management client"
	errGetUserEntitlement          = "cannot get user entitlement"
	errCreateUserEntitlement       = "cannot create user entitlement"
	errCreateUserEntitlementFailed = "create user entitlement operation reported failure"
	errUpdateUserEntitlement       = "cannot update user entitlement"
	errDeleteUserEntitlement       = "cannot delete user entitlement"
	errParseExternalName           = "cannot parse external-name as a user entitlement GUID"
	errMissingExternalName         = "external-name annotation is required to identify the user entitlement"
	errInvalidProjectID            = "cannot parse projectId as a GUID"
)

// timeFormat is the RFC3339 layout used to render LastAccessedDate.
const timeFormat = "2006-01-02T15:04:05Z07:00"

// SetupGated adds a controller that reconciles UserEntitlement managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup UserEntitlement controller"))
		}
	}, v1alpha1.UserEntitlementGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles UserEntitlement managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.UserEntitlementGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.UserEntitlement](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.UserEntitlementList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.UserEntitlementList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.UserEntitlementGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.UserEntitlement{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves cr's ProviderConfig via the shared azuredevops client
// package and uses it to construct the member entitlement management
// client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.UserEntitlement) (managed.TypedExternalClient[*v1alpha1.UserEntitlement], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	ueClient, err := newUserEntitlementClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{userEntitlements: ueClient}, nil
}

type external struct {
	userEntitlements UserEntitlementClient
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.UserEntitlement) (managed.ExternalObservation, error) {
	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	userID, err := parseUserID(externalName)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errParseExternalName)
	}

	ue, err := e.getUserEntitlement(ctx, userID)
	if azuredevops.IsNotFound(err) {
		cr.Status.AtProvider = v1alpha1.UserEntitlementObservation{}
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetUserEntitlement)
	}

	cr.Status.AtProvider = observationFromUserEntitlement(ue)
	cr.SetConditions(xpv2.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, ue),
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.UserEntitlement) (managed.ExternalCreation, error) {
	p := cr.Spec.ForProvider

	req := &memberentitlementmanagement.UserEntitlement{
		User: &graph.GraphUser{
			PrincipalName: stringPtr(p.PrincipalName),
		},
		AccessLevel: &licensing.AccessLevel{
			AccountLicenseType: accountLicenseTypePtr(licensing.AccountLicenseType(p.AccessLevel)),
		},
	}
	if len(p.ProjectEntitlements) > 0 {
		pe, err := desiredProjectEntitlements(p.ProjectEntitlements)
		if err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errInvalidProjectID)
		}
		req.ProjectEntitlements = pe
	}

	var resp *memberentitlementmanagement.UserEntitlementsPostResponse
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var createErr error
		resp, createErr = e.userEntitlements.AddUserEntitlement(ctx, memberentitlementmanagement.AddUserEntitlementArgs{
			UserEntitlement: req,
		})
		return createErr
	})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateUserEntitlement)
	}
	if resp == nil || resp.IsSuccess == nil || !*resp.IsSuccess || resp.UserEntitlement == nil || resp.UserEntitlement.Id == nil {
		return managed.ExternalCreation{}, errors.New(errCreateUserEntitlementFailed)
	}

	meta.SetExternalName(cr, resp.UserEntitlement.Id.String())
	cr.Status.AtProvider = observationFromUserEntitlement(resp.UserEntitlement)

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *v1alpha1.UserEntitlement) (managed.ExternalUpdate, error) {
	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return managed.ExternalUpdate{}, errors.New(errMissingExternalName)
	}

	userID, err := parseUserID(externalName)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errParseExternalName)
	}

	ue, err := e.getUserEntitlement(ctx, userID)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errGetUserEntitlement)
	}

	patch, err := patchForUpdate(cr.Spec.ForProvider, ue)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errInvalidProjectID)
	}
	if len(patch) == 0 {
		return managed.ExternalUpdate{}, nil
	}

	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		_, updateErr := e.userEntitlements.UpdateUserEntitlement(ctx, memberentitlementmanagement.UpdateUserEntitlementArgs{
			UserId:   userID,
			Document: &patch,
		})
		return updateErr
	})
	return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateUserEntitlement)
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.UserEntitlement) (managed.ExternalDelete, error) {
	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return managed.ExternalDelete{}, nil
	}

	userID, err := parseUserID(externalName)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errParseExternalName)
	}

	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		return e.userEntitlements.DeleteUserEntitlement(ctx, memberentitlementmanagement.DeleteUserEntitlementArgs{
			UserId: userID,
		})
	})
	return managed.ExternalDelete{}, errors.Wrap(resource.Ignore(azuredevops.IsNotFound, err), errDeleteUserEntitlement)
}

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

func (e *external) getUserEntitlement(ctx context.Context, userID *uuid.UUID) (*memberentitlementmanagement.UserEntitlement, error) {
	var ue *memberentitlementmanagement.UserEntitlement
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var getErr error
		ue, getErr = e.userEntitlements.GetUserEntitlement(ctx, memberentitlementmanagement.GetUserEntitlementArgs{
			UserId: userID,
		})
		return getErr
	})
	return ue, err
}

func observationFromUserEntitlement(ue *memberentitlementmanagement.UserEntitlement) v1alpha1.UserEntitlementObservation {
	obs := v1alpha1.UserEntitlementObservation{}
	if ue == nil {
		return obs
	}
	if ue.Id != nil {
		obs.ID = ue.Id.String()
	}
	if ue.User != nil {
		if ue.User.Origin != nil {
			obs.Origin = *ue.User.Origin
		}
		if ue.User.OriginId != nil {
			obs.OriginID = *ue.User.OriginId
		}
	}
	if ue.LastAccessedDate != nil {
		obs.LastAccessedDate = ue.LastAccessedDate.Time.Format(timeFormat)
	}
	return obs
}

// isUpToDate compares the mutable fields of the desired UserEntitlement
// against the observed SDK UserEntitlement.
func isUpToDate(p v1alpha1.UserEntitlementParameters, ue *memberentitlementmanagement.UserEntitlement) bool {
	if ue == nil {
		return false
	}
	if currentAccessLevel(ue) != p.AccessLevel {
		return false
	}
	return projectEntitlementsEqual(p.ProjectEntitlements, ue.ProjectEntitlements)
}

func currentAccessLevel(ue *memberentitlementmanagement.UserEntitlement) string {
	if ue.AccessLevel == nil || ue.AccessLevel.AccountLicenseType == nil {
		return ""
	}
	return string(*ue.AccessLevel.AccountLicenseType)
}

func desiredAccessLevel(p v1alpha1.UserEntitlementParameters) *licensing.AccessLevel {
	return &licensing.AccessLevel{
		AccountLicenseType: accountLicenseTypePtr(licensing.AccountLicenseType(p.AccessLevel)),
	}
}

func desiredProjectEntitlements(entitlements []v1alpha1.ProjectEntitlementParameters) (*[]memberentitlementmanagement.ProjectEntitlement, error) {
	out := make([]memberentitlementmanagement.ProjectEntitlement, 0, len(entitlements))
	for _, pe := range entitlements {
		id, err := uuid.Parse(pe.ProjectID)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid projectId %q", pe.ProjectID)
		}
		out = append(out, memberentitlementmanagement.ProjectEntitlement{
			ProjectRef: &memberentitlementmanagement.ProjectRef{Id: uuidPtr(id)},
			Group: &memberentitlementmanagement.Group{
				GroupType: groupTypePtr(memberentitlementmanagement.GroupType(pe.GroupType)),
			},
		})
	}
	return &out, nil
}

// patchForUpdate builds a JSON patch document that replaces the access
// level and diffs the desired project entitlements against the observed
// ones, emitting one add/remove operation per project entitlement that
// changed. The Azure DevOps API does not support bulk-replacing
// /projectEntitlements; each project must be added or removed individually.
func patchForUpdate(p v1alpha1.UserEntitlementParameters, ue *memberentitlementmanagement.UserEntitlement) ([]webapi.JsonPatchOperation, error) {
	var current *[]memberentitlementmanagement.ProjectEntitlement
	if ue != nil {
		current = ue.ProjectEntitlements
	}

	projectPatch, err := projectEntitlementPatch(p.ProjectEntitlements, current)
	if err != nil {
		return nil, err
	}

	patch := make([]webapi.JsonPatchOperation, 0, 1+len(projectPatch))
	patch = append(patch, webapi.JsonPatchOperation{
		Op:    operationPtr(webapi.OperationValues.Replace),
		Path:  stringPtr("/accessLevel"),
		Value: desiredAccessLevel(p),
	})
	patch = append(patch, projectPatch...)

	return patch, nil
}

// projectEntitlementPatch diffs the desired project entitlements against
// the observed ones (keyed by "projectID|groupType") and returns one
// add/remove JsonPatchOperation per entry that differs.
func projectEntitlementPatch(desired []v1alpha1.ProjectEntitlementParameters, current *[]memberentitlementmanagement.ProjectEntitlement) ([]webapi.JsonPatchOperation, error) {
	desiredEntitlements, err := desiredProjectEntitlements(desired)
	if err != nil {
		return nil, err
	}

	currentKeys := make(map[string]bool)
	for _, key := range currentProjectEntitlementKeys(current) {
		currentKeys[key] = true
	}

	desiredKeys := make(map[string]bool)
	for _, key := range projectEntitlementKeys(desired) {
		desiredKeys[key] = true
	}

	ops := addProjectEntitlementOps(*desiredEntitlements, currentKeys)
	ops = append(ops, removeProjectEntitlementOps(current, desiredKeys)...)
	return ops, nil
}

// addProjectEntitlementOps returns an "add" JsonPatchOperation for every
// desired project entitlement whose key is not already present.
func addProjectEntitlementOps(desired []memberentitlementmanagement.ProjectEntitlement, currentKeys map[string]bool) []webapi.JsonPatchOperation {
	var ops []webapi.JsonPatchOperation
	for _, pe := range desired {
		if currentKeys[projectEntitlementKey(pe)] {
			continue
		}
		ops = append(ops, webapi.JsonPatchOperation{
			Op:    operationPtr(webapi.OperationValues.Add),
			Path:  stringPtr("/projectEntitlements"),
			Value: pe,
		})
	}
	return ops
}

// removeProjectEntitlementOps returns a "remove" JsonPatchOperation for
// every observed project entitlement whose key is no longer desired.
func removeProjectEntitlementOps(current *[]memberentitlementmanagement.ProjectEntitlement, desiredKeys map[string]bool) []webapi.JsonPatchOperation {
	if current == nil {
		return nil
	}
	var ops []webapi.JsonPatchOperation
	for _, pe := range *current {
		if desiredKeys[projectEntitlementKey(pe)] || pe.ProjectRef == nil || pe.ProjectRef.Id == nil {
			continue
		}
		ops = append(ops, webapi.JsonPatchOperation{
			Op:   operationPtr(webapi.OperationValues.Remove),
			Path: stringPtr("/projectEntitlements/" + pe.ProjectRef.Id.String()),
		})
	}
	return ops
}

// projectEntitlementsEqual performs an order-independent comparison between
// the desired project entitlements and the observed ones, keyed by
// "projectID|groupType".
func projectEntitlementsEqual(desired []v1alpha1.ProjectEntitlementParameters, current *[]memberentitlementmanagement.ProjectEntitlement) bool {
	desiredKeys := projectEntitlementKeys(desired)
	currentKeys := currentProjectEntitlementKeys(current)

	sort.Strings(desiredKeys)
	sort.Strings(currentKeys)

	if len(desiredKeys) != len(currentKeys) {
		return false
	}
	for i := range desiredKeys {
		if desiredKeys[i] != currentKeys[i] {
			return false
		}
	}
	return true
}

func currentProjectEntitlementKeys(current *[]memberentitlementmanagement.ProjectEntitlement) []string {
	if current == nil {
		return nil
	}
	var keys []string
	for _, pe := range *current {
		key := projectEntitlementKey(pe)
		if key != "|" {
			keys = append(keys, key)
		}
	}
	return keys
}

func projectEntitlementKey(pe memberentitlementmanagement.ProjectEntitlement) string {
	var projectID, groupType string
	if pe.ProjectRef != nil && pe.ProjectRef.Id != nil {
		projectID = pe.ProjectRef.Id.String()
	}
	if pe.Group != nil && pe.Group.GroupType != nil {
		groupType = string(*pe.Group.GroupType)
	}
	return projectID + "|" + groupType
}

func projectEntitlementKeys(entitlements []v1alpha1.ProjectEntitlementParameters) []string {
	keys := make([]string, 0, len(entitlements))
	for _, pe := range entitlements {
		keys = append(keys, pe.ProjectID+"|"+pe.GroupType)
	}
	return keys
}

func stringPtr(s string) *string {
	return &s
}

func uuidPtr(u uuid.UUID) *uuid.UUID {
	return &u
}

func accountLicenseTypePtr(t licensing.AccountLicenseType) *licensing.AccountLicenseType {
	return &t
}

func groupTypePtr(t memberentitlementmanagement.GroupType) *memberentitlementmanagement.GroupType {
	return &t
}

func operationPtr(o webapi.Operation) *webapi.Operation {
	return &o
}
