// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"strconv"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/webapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/environment/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig                   = "cannot get Azure DevOps client config"
	errNewClient                   = "cannot create new Azure DevOps environment client"
	errMissingProjectID            = "spec.forProvider.projectId is required"
	errMissingName                 = "spec.forProvider.name is required"
	errMissingEnvironmentID        = "environment id is required"
	errParseEnvironmentID          = "cannot parse environment id"
	errGetEnvironment              = "cannot get environment"
	errCreateEnvironment           = "cannot create environment"
	errCreateEnvironmentNoID       = "created environment did not include an id"
	errUpdateEnvironment           = "cannot update environment"
	errDeleteEnvironment           = "cannot delete environment"
	errImmutableProject            = "projectId is immutable once created"
	errImportProjectIDVerification = "cannot verify imported environment projectId"
)

const annotationProjectID = "environment.azuredevops.crossplane.io/project-id"

// SetupGated adds a controller that reconciles Environment managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup Environment controller"))
		}
	}, v1alpha1.EnvironmentGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles Environment managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.EnvironmentGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.Environment](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.EnvironmentList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.EnvironmentList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.EnvironmentGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Environment{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves cr's ProviderConfig via the shared azuredevops client
// package and uses it to construct the TaskAgent API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.Environment) (managed.TypedExternalClient[*v1alpha1.Environment], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	environmentClient, err := newEnvironmentClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{environments: environmentClient}, nil
}

type external struct {
	environments EnvironmentClient
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.Environment) (managed.ExternalObservation, error) {
	if _, err := getProjectID(cr); err != nil {
		return managed.ExternalObservation{}, err
	}

	projectID := projectIDForAPI(cr)
	environmentID, ok, err := environmentIDFrom(cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetEnvironment)
	}
	if !ok {
		return managed.ExternalObservation{}, nil
	}

	environment, err := e.getEnvironment(ctx, projectID, environmentID)
	if azuredevops.IsNotFound(err) {
		return missingObservation(cr)
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetEnvironment)
	}

	cr.Status.AtProvider = observationFromEnvironment(environment)
	setProjectIDAnnotation(cr, projectIDFromEnvironment(environment, projectID))
	cr.SetConditions(xpv2.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, environment),
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.Environment) (managed.ExternalCreation, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	payload, err := desiredEnvironmentCreateParameter(cr.Spec.ForProvider)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	var created *taskagent.EnvironmentInstance
	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var createErr error
		created, createErr = e.environments.AddEnvironment(ctx, taskagent.AddEnvironmentArgs{
			Project:                    stringPtr(projectID),
			EnvironmentCreateParameter: payload,
		})
		return createErr
	})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateEnvironment)
	}
	if created == nil || created.Id == nil {
		return managed.ExternalCreation{}, errors.New(errCreateEnvironmentNoID)
	}

	meta.SetExternalName(cr, strconv.Itoa(*created.Id))
	cr.Status.AtProvider = observationFromEnvironment(created)
	setProjectIDAnnotation(cr, projectIDFromEnvironment(created, projectID))
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *v1alpha1.Environment) (managed.ExternalUpdate, error) {
	projectID, err := verifiedProjectIDForMutation(cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	environmentID, ok, err := environmentIDFrom(cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateEnvironment)
	}
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errMissingEnvironmentID)
	}

	current, err := e.getEnvironment(ctx, projectID, environmentID)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errGetEnvironment)
	}
	if err := validateImmutableFields(cr.Spec.ForProvider, current); err != nil {
		return managed.ExternalUpdate{}, err
	}

	patch, err := desiredEnvironmentUpdateParameter(cr.Spec.ForProvider, current)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	if patch == nil {
		cr.Status.AtProvider = observationFromEnvironment(current)
		return managed.ExternalUpdate{}, nil
	}

	var updated *taskagent.EnvironmentInstance
	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var updateErr error
		updated, updateErr = e.environments.UpdateEnvironment(ctx, taskagent.UpdateEnvironmentArgs{
			Project:                    stringPtr(projectID),
			EnvironmentId:              intPtr(environmentID),
			EnvironmentUpdateParameter: patch,
		})
		return updateErr
	})
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateEnvironment)
	}

	cr.Status.AtProvider = observationFromEnvironment(updated)
	setProjectIDAnnotation(cr, projectIDFromEnvironment(updated, projectID))
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.Environment) (managed.ExternalDelete, error) {
	projectID, err := verifiedProjectIDForMutation(cr)
	if err != nil {
		return managed.ExternalDelete{}, err
	}

	environmentID, ok, err := environmentIDFrom(cr)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteEnvironment)
	}
	if !ok {
		return managed.ExternalDelete{}, nil
	}

	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		return e.environments.DeleteEnvironment(ctx, taskagent.DeleteEnvironmentArgs{
			Project:       stringPtr(projectID),
			EnvironmentId: intPtr(environmentID),
		})
	})
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(resource.Ignore(azuredevops.IsNotFound, err), errDeleteEnvironment)
	}

	return managed.ExternalDelete{}, nil
}

func missingObservation(cr *v1alpha1.Environment) (managed.ExternalObservation, error) {
	if meta.GetExternalName(cr) != "" {
		observedProjectID := cr.GetAnnotations()[annotationProjectID]
		if observedProjectID == "" {
			return managed.ExternalObservation{}, errors.New(errImportProjectIDVerification)
		}
		if desiredProjectID := cr.Spec.ForProvider.ProjectID; desiredProjectID != "" && !strings.EqualFold(desiredProjectID, observedProjectID) {
			return managed.ExternalObservation{}, errors.Errorf("%s: desired %q, observed %q", errImmutableProject, desiredProjectID, observedProjectID)
		}
	}
	return managed.ExternalObservation{ResourceExists: false}, nil
}

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

func projectIDForAPI(cr *v1alpha1.Environment) string {
	if observed := cr.GetAnnotations()[annotationProjectID]; observed != "" {
		return observed
	}
	return cr.Spec.ForProvider.ProjectID
}

func verifiedProjectIDForMutation(cr *v1alpha1.Environment) (string, error) {
	desiredProjectID, err := getProjectID(cr)
	if err != nil {
		return "", err
	}
	if meta.GetExternalName(cr) == "" {
		return desiredProjectID, nil
	}

	observedProjectID := cr.GetAnnotations()[annotationProjectID]
	if observedProjectID == "" {
		return "", errors.New(errImportProjectIDVerification)
	}
	if !strings.EqualFold(desiredProjectID, observedProjectID) {
		return "", errors.Errorf("%s: desired %q, observed %q", errImmutableProject, desiredProjectID, observedProjectID)
	}
	return observedProjectID, nil
}

func setProjectIDAnnotation(cr *v1alpha1.Environment, projectID string) {
	if projectID == "" {
		return
	}
	meta.AddAnnotations(cr, map[string]string{annotationProjectID: projectID})
}

func (e *external) getEnvironment(ctx context.Context, projectID string, environmentID int) (*taskagent.EnvironmentInstance, error) {
	var environment *taskagent.EnvironmentInstance
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var getErr error
		environment, getErr = e.environments.GetEnvironmentById(ctx, taskagent.GetEnvironmentByIdArgs{
			Project:       stringPtr(projectID),
			EnvironmentId: intPtr(environmentID),
		})
		return getErr
	})
	return environment, err
}

func getProjectID(cr *v1alpha1.Environment) (string, error) {
	if cr.Spec.ForProvider.ProjectID == "" {
		return "", errors.New(errMissingProjectID)
	}
	return cr.Spec.ForProvider.ProjectID, nil
}

func environmentIDFrom(cr *v1alpha1.Environment) (int, bool, error) {
	id := meta.GetExternalName(cr)
	if id == "" {
		id = cr.Status.AtProvider.ID
	}
	if id == "" {
		return 0, false, nil
	}
	parsed, err := strconv.Atoi(id)
	if err != nil {
		return 0, false, errors.Wrap(err, errParseEnvironmentID)
	}
	return parsed, true, nil
}

func desiredEnvironmentCreateParameter(p v1alpha1.EnvironmentParameters) (*taskagent.EnvironmentCreateParameter, error) {
	if p.Name == "" {
		return nil, errors.New(errMissingName)
	}
	payload := &taskagent.EnvironmentCreateParameter{
		Name: stringPtr(p.Name),
	}
	if p.Description != "" {
		payload.Description = stringPtr(p.Description)
	}
	return payload, nil
}

func desiredEnvironmentUpdateParameter(desired v1alpha1.EnvironmentParameters, current *taskagent.EnvironmentInstance) (*taskagent.EnvironmentUpdateParameter, error) {
	if desired.Name == "" {
		return nil, errors.New(errMissingName)
	}

	patch := &taskagent.EnvironmentUpdateParameter{}
	changed := false

	if valueOrEmpty(current.Name) != desired.Name {
		patch.Name = stringPtr(desired.Name)
		changed = true
	}
	if valueOrEmpty(current.Description) != desired.Description {
		patch.Description = stringPtr(desired.Description)
		changed = true
	}

	if !changed {
		return nil, nil
	}
	return patch, nil
}

func observationFromEnvironment(environment *taskagent.EnvironmentInstance) v1alpha1.EnvironmentObservation {
	observation := v1alpha1.EnvironmentObservation{}
	if environment == nil {
		return observation
	}
	if environment.Id != nil {
		observation.ID = strconv.Itoa(*environment.Id)
	}
	observation.CreatedBy = identityString(environment.CreatedBy)
	if environment.CreatedOn != nil {
		createdOn := metav1.NewTime(environment.CreatedOn.Time)
		observation.CreatedOn = &createdOn
	}
	return observation
}

func isUpToDate(desired v1alpha1.EnvironmentParameters, current *taskagent.EnvironmentInstance) bool {
	if current == nil {
		return false
	}
	if valueOrEmpty(current.Name) != desired.Name {
		return false
	}
	if valueOrEmpty(current.Description) != desired.Description {
		return false
	}
	if desired.ProjectID != "" {
		currentProjectID := ""
		if current.Project != nil && current.Project.Id != nil {
			currentProjectID = current.Project.Id.String()
		}
		if !strings.EqualFold(currentProjectID, desired.ProjectID) {
			return false
		}
	}
	return true
}

func validateImmutableFields(desired v1alpha1.EnvironmentParameters, current *taskagent.EnvironmentInstance) error {
	if current == nil || desired.ProjectID == "" || current.Project == nil || current.Project.Id == nil {
		return nil
	}

	currentProjectID := current.Project.Id.String()
	if !strings.EqualFold(desired.ProjectID, currentProjectID) {
		return errors.Errorf("%s: desired %q, observed %q", errImmutableProject, desired.ProjectID, currentProjectID)
	}
	return nil
}

func projectIDFromEnvironment(environment *taskagent.EnvironmentInstance, fallback string) string {
	if environment != nil && environment.Project != nil && environment.Project.Id != nil {
		return environment.Project.Id.String()
	}
	return fallback
}

func identityString(identity *webapi.IdentityRef) string {
	if identity == nil {
		return ""
	}
	if identity.DisplayName != nil && *identity.DisplayName != "" {
		return *identity.DisplayName
	}
	if identity.UniqueName != nil && *identity.UniqueName != "" {
		return *identity.UniqueName
	}
	if identity.Id != nil && *identity.Id != "" {
		return *identity.Id
	}
	return ""
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
