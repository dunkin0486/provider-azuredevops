// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package team

import (
	"context"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"

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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/team/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig        = "cannot get Azure DevOps client config"
	errNewClient        = "cannot create new Azure DevOps core client"
	errMissingProjectID = "spec.forProvider.projectId is required"
	errMissingTeamID    = "team id is required"
	errGetTeam          = "cannot get team"
	errCreateTeam       = "cannot create team"
	errCreateTeamNoID   = "created team did not include an id"
	errUpdateTeam       = "cannot update team"
	errDeleteTeam       = "cannot delete team"
	errImmutableProject = "projectId is immutable once created"
)

// SetupGated adds a controller that reconciles Team managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup Team controller"))
		}
	}, v1alpha1.TeamGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles Team managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.TeamGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.Team](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.TeamList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.TeamList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.TeamGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Team{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector produces an ExternalClient by resolving the managed resource's
// ProviderConfig and building Azure DevOps SDK clients from it.
type connector struct {
	kube client.Client
}

// Connect resolves cr's ProviderConfig via the shared azuredevops client
// package and uses it to construct the Core API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.Team) (managed.TypedExternalClient[*v1alpha1.Team], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	coreClient, err := newTeamClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{teams: coreClient}, nil
}

// An external observes, then either creates, updates, or deletes an external
// Azure DevOps Team to ensure it reflects the managed resource's desired state.
type external struct {
	teams TeamClient
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.Team) (managed.ExternalObservation, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	teamID, ok := getTeamID(cr)
	if !ok {
		return managed.ExternalObservation{}, nil
	}

	team, err := e.getTeam(ctx, projectID, teamID)
	if azuredevops.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetTeam)
	}

	cr.Status.AtProvider = observationFromTeam(team)
	cr.SetConditions(xpv2.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, team),
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.Team) (managed.ExternalCreation, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	created, err := e.createTeam(ctx, projectID, desiredTeam(cr.Spec.ForProvider))
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateTeam)
	}
	if created == nil || created.Id == nil {
		return managed.ExternalCreation{}, errors.New(errCreateTeamNoID)
	}

	meta.SetExternalName(cr, created.Id.String())
	cr.Status.AtProvider = observationFromTeam(created)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *v1alpha1.Team) (managed.ExternalUpdate, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	teamID, ok := getTeamID(cr)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errMissingTeamID)
	}

	current, err := e.getTeam(ctx, projectID, teamID)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errGetTeam)
	}
	if err := validateImmutableFields(cr.Spec.ForProvider, current); err != nil {
		return managed.ExternalUpdate{}, err
	}

	patch := desiredTeamPatch(cr.Spec.ForProvider, current)
	if patch == nil {
		cr.Status.AtProvider = observationFromTeam(current)
		return managed.ExternalUpdate{}, nil
	}

	updated, err := e.updateTeam(ctx, projectID, teamID, patch)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateTeam)
	}

	cr.Status.AtProvider = observationFromTeam(updated)
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.Team) (managed.ExternalDelete, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalDelete{}, err
	}

	teamID, ok := getTeamID(cr)
	if !ok {
		return managed.ExternalDelete{}, nil
	}

	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		return e.teams.DeleteTeam(ctx, core.DeleteTeamArgs{
			ProjectId: stringPtr(projectID),
			TeamId:    stringPtr(teamID),
		})
	})
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(resource.Ignore(azuredevops.IsNotFound, err), errDeleteTeam)
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

func (e *external) getTeam(ctx context.Context, projectID, teamID string) (*core.WebApiTeam, error) {
	var team *core.WebApiTeam
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var getErr error
		team, getErr = e.teams.GetTeam(ctx, core.GetTeamArgs{
			ProjectId: stringPtr(projectID),
			TeamId:    stringPtr(teamID),
		})
		return getErr
	})
	return team, err
}

func (e *external) createTeam(ctx context.Context, projectID string, team *core.WebApiTeam) (*core.WebApiTeam, error) {
	var created *core.WebApiTeam
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var createErr error
		created, createErr = e.teams.CreateTeam(ctx, core.CreateTeamArgs{
			ProjectId: stringPtr(projectID),
			Team:      team,
		})
		return createErr
	})
	return created, err
}

func (e *external) updateTeam(ctx context.Context, projectID, teamID string, patch *core.WebApiTeam) (*core.WebApiTeam, error) {
	var updated *core.WebApiTeam
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var updateErr error
		updated, updateErr = e.teams.UpdateTeam(ctx, core.UpdateTeamArgs{
			ProjectId: stringPtr(projectID),
			TeamId:    stringPtr(teamID),
			TeamData:  patch,
		})
		return updateErr
	})
	return updated, err
}

func getProjectID(cr *v1alpha1.Team) (string, error) {
	if cr.Spec.ForProvider.ProjectID == "" {
		return "", errors.New(errMissingProjectID)
	}
	return cr.Spec.ForProvider.ProjectID, nil
}

func getTeamID(cr *v1alpha1.Team) (string, bool) {
	if externalName := meta.GetExternalName(cr); externalName != "" {
		return externalName, true
	}
	if cr.Status.AtProvider.ID != "" {
		return cr.Status.AtProvider.ID, true
	}
	return "", false
}

func desiredTeam(p v1alpha1.TeamParameters) *core.WebApiTeam {
	team := &core.WebApiTeam{
		Name: stringPtr(p.Name),
	}
	if p.Description != "" {
		team.Description = stringPtr(p.Description)
	}
	return team
}

func desiredTeamPatch(desired v1alpha1.TeamParameters, current *core.WebApiTeam) *core.WebApiTeam {
	patch := &core.WebApiTeam{}
	changed := false

	if current == nil || current.Name == nil || *current.Name != desired.Name {
		patch.Name = stringPtr(desired.Name)
		changed = true
	}
	if desired.Description != "" && (current == nil || current.Description == nil || *current.Description != desired.Description) {
		patch.Description = stringPtr(desired.Description)
		changed = true
	}

	if !changed {
		return nil
	}
	return patch
}

func observationFromTeam(team *core.WebApiTeam) v1alpha1.TeamObservation {
	obs := v1alpha1.TeamObservation{}
	if team == nil {
		return obs
	}
	if team.Id != nil {
		obs.ID = team.Id.String()
	}
	if team.Url != nil {
		obs.URL = *team.Url
	}
	return obs
}

func isUpToDate(desired v1alpha1.TeamParameters, current *core.WebApiTeam) bool {
	if current == nil {
		return false
	}
	if current.Name == nil || *current.Name != desired.Name {
		return false
	}
	if desired.ProjectID != "" {
		currentProjectID := ""
		if current.ProjectId != nil {
			currentProjectID = current.ProjectId.String()
		}
		if !strings.EqualFold(currentProjectID, desired.ProjectID) {
			return false
		}
	}
	if desired.Description != "" {
		if current.Description == nil || *current.Description != desired.Description {
			return false
		}
	}
	return true
}

func validateImmutableFields(desired v1alpha1.TeamParameters, current *core.WebApiTeam) error {
	if current == nil {
		return nil
	}

	currentProjectID := ""
	if current.ProjectId != nil {
		currentProjectID = current.ProjectId.String()
	}
	if desired.ProjectID != "" && currentProjectID != "" && !strings.EqualFold(desired.ProjectID, currentProjectID) {
		return errors.Errorf("%s: desired %q, observed %q", errImmutableProject, desired.ProjectID, currentProjectID)
	}

	return nil
}

func stringPtr(s string) *string {
	return &s
}
