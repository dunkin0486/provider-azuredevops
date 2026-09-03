// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package gitrepository

import (
	"context"

	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	adogit "github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"

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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/gitrepository/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig                   = "cannot get Azure DevOps client config"
	errNewClient                   = "cannot create new Azure DevOps git client"
	errMissingProjectID            = "spec.forProvider.projectId is required"
	errGetRepository               = "cannot get repository"
	errCreateRepository            = "cannot create repository"
	errCreateRepositoryMissingID   = "created repository did not include an id"
	errUpdateRepository            = "cannot update repository"
	errDeleteRepository            = "cannot delete repository"
	errParseProjectID              = "cannot parse project id"
	errParseParentRepositoryID     = "cannot parse parent repository id"
	errParseRepositoryID           = "cannot parse repository id"
	errImmutableProjectID          = "projectId is immutable once created"
	errImmutableParentRepositoryID = "parentRepositoryId is immutable once created"
)

// SetupGated adds a controller that reconciles GitRepository managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup GitRepository controller"))
		}
	}, v1alpha1.GitRepositoryGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles GitRepository managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.GitRepositoryGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.GitRepository](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.GitRepositoryList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.GitRepositoryList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.GitRepositoryGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.GitRepository{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector produces an ExternalClient by resolving the managed resource's
// ProviderConfig and building Azure DevOps SDK clients from it.
type connector struct {
	kube client.Client
}

// Connect resolves cr's ProviderConfig via the shared azuredevops client package
// and uses it to construct the Git API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.GitRepository) (managed.TypedExternalClient[*v1alpha1.GitRepository], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	gitClient, err := newGitRepositoryClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{git: gitClient}, nil
}

// An external observes, then either creates, updates, or deletes an external
// Azure DevOps GitRepository to ensure it reflects the managed resource's desired state.
// Project and parent repository identity are immutable after creation; Update
// validates them and returns a configuration error rather than attempting a move or re-fork.
type external struct {
	git GitRepositoryClient
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.GitRepository) (managed.ExternalObservation, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	repositoryID := meta.GetExternalName(cr)
	if repositoryID == "" {
		return managed.ExternalObservation{}, nil
	}

	repo, err := e.git.GetRepository(ctx, adogit.GetRepositoryArgs{
		Project:      stringPtr(projectID),
		RepositoryId: stringPtr(repositoryID),
	})
	if azuredevops.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetRepository)
	}

	cr.Status.AtProvider = observationFromRepository(repo)
	cr.SetConditions(xpv2.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, repo),
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.GitRepository) (managed.ExternalCreation, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	req, err := createRepositoryArgs(cr.Spec.ForProvider, projectID)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	repo, err := e.git.CreateRepository(ctx, req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateRepository)
	}
	if repo == nil || repo.Id == nil {
		return managed.ExternalCreation{}, errors.New(errCreateRepositoryMissingID)
	}

	meta.SetExternalName(cr, repo.Id.String())
	cr.Status.AtProvider = observationFromRepository(repo)

	if !requiresPostCreateUpdate(cr.Spec.ForProvider) {
		return managed.ExternalCreation{}, nil
	}

	updated, err := e.updateRepository(ctx, projectID, repo.Id, cr.Spec.ForProvider, repo)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	cr.Status.AtProvider = observationFromRepository(updated)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *v1alpha1.GitRepository) (managed.ExternalUpdate, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	repositoryID, err := getRepositoryUUID(cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	if repositoryID == nil {
		return managed.ExternalUpdate{}, errors.New(errParseRepositoryID)
	}

	current, err := e.git.GetRepository(ctx, adogit.GetRepositoryArgs{
		Project:      stringPtr(projectID),
		RepositoryId: stringPtr(repositoryID.String()),
	})
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errGetRepository)
	}

	updated, err := e.updateRepository(ctx, projectID, repositoryID, cr.Spec.ForProvider, current)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	cr.Status.AtProvider = observationFromRepository(updated)

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.GitRepository) (managed.ExternalDelete, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalDelete{}, err
	}

	repositoryID, err := getRepositoryUUID(cr)
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	if repositoryID == nil {
		return managed.ExternalDelete{}, nil
	}

	err = resource.Ignore(azuredevops.IsNotFound, e.git.DeleteRepository(ctx, adogit.DeleteRepositoryArgs{
		Project:      stringPtr(projectID),
		RepositoryId: repositoryID,
	}))
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteRepository)
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

func (e *external) updateRepository(ctx context.Context, projectID string, repositoryID *uuid.UUID, desired v1alpha1.GitRepositoryParameters, current *adogit.GitRepository) (*adogit.GitRepository, error) { //nolint:gocyclo // Straightforward field-by-field PATCH construction.
	if err := validateImmutableFields(desired, current); err != nil {
		return nil, err
	}

	patch := &adogit.GitRepository{}
	changed := false

	if current == nil || current.Name == nil || *current.Name != desired.Name {
		patch.Name = stringPtr(desired.Name)
		changed = true
	}
	if desired.DefaultBranch != "" && (current == nil || current.DefaultBranch == nil || *current.DefaultBranch != desired.DefaultBranch) {
		patch.DefaultBranch = stringPtr(desired.DefaultBranch)
		changed = true
	}
	if desired.Disabled != nil {
		if current == nil || current.IsDisabled == nil || *current.IsDisabled != *desired.Disabled {
			patch.IsDisabled = desired.Disabled
			changed = true
		}
	}

	if !changed {
		if current != nil {
			return current, nil
		}
		return &adogit.GitRepository{}, nil
	}

	updated, err := e.git.UpdateRepository(ctx, adogit.UpdateRepositoryArgs{
		NewRepositoryInfo: patch,
		Project:           stringPtr(projectID),
		RepositoryId:      repositoryID,
	})
	if err != nil {
		return nil, errors.Wrap(err, errUpdateRepository)
	}
	return updated, nil
}

func createRepositoryArgs(p v1alpha1.GitRepositoryParameters, projectID string) (adogit.CreateRepositoryArgs, error) {
	req := adogit.CreateRepositoryArgs{
		Project: stringPtr(projectID),
		GitRepositoryToCreate: &adogit.GitRepositoryCreateOptions{
			Name: stringPtr(p.Name),
		},
	}

	projectUUID, err := parseUUID(projectID, errParseProjectID)
	if err != nil {
		return adogit.CreateRepositoryArgs{}, err
	}
	req.GitRepositoryToCreate.Project = &core.TeamProjectReference{Id: projectUUID}

	if p.ParentRepositoryID != "" {
		parentUUID, err := parseUUID(p.ParentRepositoryID, errParseParentRepositoryID)
		if err != nil {
			return adogit.CreateRepositoryArgs{}, err
		}
		req.GitRepositoryToCreate.ParentRepository = &adogit.GitRepositoryRef{Id: parentUUID}
	}

	return req, nil
}

func getProjectID(cr *v1alpha1.GitRepository) (string, error) {
	if cr.Spec.ForProvider.ProjectID == "" {
		return "", errors.New(errMissingProjectID)
	}
	return cr.Spec.ForProvider.ProjectID, nil
}

func getRepositoryUUID(cr *v1alpha1.GitRepository) (*uuid.UUID, error) {
	if externalName := meta.GetExternalName(cr); externalName != "" {
		return parseUUID(externalName, errParseRepositoryID)
	}
	if cr.Status.AtProvider.ID != "" {
		return parseUUID(cr.Status.AtProvider.ID, errParseRepositoryID)
	}
	return nil, nil
}

func parseUUID(value, message string) (*uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, errors.Wrap(err, message)
	}
	return &id, nil
}

func observationFromRepository(repo *adogit.GitRepository) v1alpha1.GitRepositoryObservation {
	obs := v1alpha1.GitRepositoryObservation{}
	if repo == nil {
		return obs
	}
	if repo.Id != nil {
		obs.ID = repo.Id.String()
	}
	if repo.RemoteUrl != nil {
		obs.RemoteURL = *repo.RemoteUrl
	}
	if repo.SshUrl != nil {
		obs.SSHURL = *repo.SshUrl
	}
	if repo.WebUrl != nil {
		obs.WebURL = *repo.WebUrl
	}
	if repo.Size != nil {
		obs.Size = int64(*repo.Size) //nolint:gosec // Azure DevOps repository sizes fit safely in int64 for provider status use.
	}
	return obs
}

func isUpToDate(desired v1alpha1.GitRepositoryParameters, current *adogit.GitRepository) bool { //nolint:gocyclo // Straightforward desired-versus-observed field comparison.
	if current == nil {
		return false
	}
	if current.Name == nil || *current.Name != desired.Name {
		return false
	}
	if desired.ProjectID != "" {
		currentProjectID := ""
		if current.Project != nil && current.Project.Id != nil {
			currentProjectID = current.Project.Id.String()
		}
		if currentProjectID != desired.ProjectID {
			return false
		}
	}

	desiredParent := desired.ParentRepositoryID
	currentParent := ""
	if current.ParentRepository != nil && current.ParentRepository.Id != nil {
		currentParent = current.ParentRepository.Id.String()
	}
	if desiredParent != currentParent {
		if desiredParent != "" || currentParent != "" {
			return false
		}
	}

	if desired.DefaultBranch != "" {
		if current.DefaultBranch == nil || *current.DefaultBranch != desired.DefaultBranch {
			return false
		}
	}
	if desired.Disabled != nil {
		if current.IsDisabled == nil || *current.IsDisabled != *desired.Disabled {
			return false
		}
	}
	return true
}

func validateImmutableFields(desired v1alpha1.GitRepositoryParameters, current *adogit.GitRepository) error { //nolint:gocyclo // Straightforward immutable field validation.
	if current == nil {
		return nil
	}

	currentProjectID := ""
	if current.Project != nil && current.Project.Id != nil {
		currentProjectID = current.Project.Id.String()
	}
	if desired.ProjectID != "" && currentProjectID != "" && desired.ProjectID != currentProjectID {
		return errors.Errorf("%s: desired %q, observed %q", errImmutableProjectID, desired.ProjectID, currentProjectID)
	}

	desiredParent := desired.ParentRepositoryID
	currentParent := ""
	if current.ParentRepository != nil && current.ParentRepository.Id != nil {
		currentParent = current.ParentRepository.Id.String()
	}
	if desiredParent != currentParent {
		if desiredParent != "" || currentParent != "" {
			return errors.Errorf("%s: desired %q, observed %q", errImmutableParentRepositoryID, desiredParent, currentParent)
		}
	}

	return nil
}

func requiresPostCreateUpdate(p v1alpha1.GitRepositoryParameters) bool {
	return p.DefaultBranch != "" || p.Disabled != nil
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
