// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package branchpolicycommentresolution

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/policy"

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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/branchpolicycommentresolution/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig                     = "cannot get Azure DevOps client config"
	errNewClient                     = "cannot create new Azure DevOps policy client"
	errMissingProjectID              = "spec.forProvider.projectId is required"
	errMissingRepositoryID           = "spec.forProvider.repositoryId is required"
	errMissingBranch                 = "spec.forProvider.branch is required"
	errImmutableProjectID            = "projectId is immutable once created"
	errGetPolicyConfiguration        = "cannot get policy configuration"
	errImportProjectIDVerification   = "cannot verify imported policy configuration projectId"
	errPolicyConfigurationDeleted    = "external policy configuration has been deleted"
	errCreatePolicyConfiguration     = "cannot create policy configuration"
	errCreatePolicyConfigurationNoID = "created policy configuration did not include an id"
	errUpdatePolicyConfiguration     = "cannot update policy configuration"
	errDeletePolicyConfiguration     = "cannot delete policy configuration"
	errParsePolicyConfigurationID    = "cannot parse policy configuration id"
	errDecodePolicyConfiguration     = "cannot decode policy configuration settings"
	errInvalidBranch                 = "branch must be an exact ref like refs/heads/main or a trailing-wildcard prefix like refs/heads/*"
	errUnexpectedPolicyType          = "external policy configuration is not a comment requirements policy"
	errUnsupportedPolicyScope        = "external policy configuration scope is unsupported: expected exactly one repository-bound scope"
	errUnsupportedPolicySettings     = "external policy configuration contains unsupported comment resolution settings"
	settingScope                     = "scope"
	matchKindExact                   = "Exact"
	matchKindPrefix                  = "Prefix"
	commentResolutionPolicyTypeName  = "Comment requirements"
	commentResolutionPolicyTypeIDStr = "c6a1889d-b943-4856-b76f-9e46bb6b0df2"
	annotationProjectID              = "branchpolicycommentresolution.azuredevops.crossplane.io/project-id"
)

var commentResolutionPolicyTypeID = uuid.MustParse(commentResolutionPolicyTypeIDStr)

type commentResolutionPolicySettings struct {
	Scope []policyScope `json:"scope"`
}

type policyScope struct {
	RepositoryID string `json:"repositoryId"`
	RefName      string `json:"refName"`
	MatchKind    string `json:"matchKind"`
}

// SetupGated adds a controller that reconciles BranchPolicyCommentResolution
// managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup BranchPolicyCommentResolution controller"))
		}
	}, v1alpha1.BranchPolicyCommentResolutionGroupVersionKind)
	return nil
}

func isPolicyDeleted(cfg *policy.PolicyConfiguration) bool {
	return cfg != nil && cfg.IsDeleted != nil && *cfg.IsDeleted
}

// Setup adds a controller that reconciles BranchPolicyCommentResolution managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.BranchPolicyCommentResolutionGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.BranchPolicyCommentResolution](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.BranchPolicyCommentResolutionList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.BranchPolicyCommentResolutionList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.BranchPolicyCommentResolutionGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.BranchPolicyCommentResolution{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves the managed resource's ProviderConfig and constructs a
// Policy API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.BranchPolicyCommentResolution) (managed.TypedExternalClient[*v1alpha1.BranchPolicyCommentResolution], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	policyClient, err := newBranchPolicyCommentResolutionClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{policies: policyClient}, nil
}

type external struct {
	policies BranchPolicyCommentResolutionClient
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.BranchPolicyCommentResolution) (managed.ExternalObservation, error) {
	projectID, err := projectIDForExternalOperations(cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	if cr.GetDeletionTimestamp().IsZero() {
		if err := validateImmutableProjectID(cr, projectID); err != nil {
			return managed.ExternalObservation{}, err
		}
	}

	configurationID, ok, err := policyConfigurationIDFrom(cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	if !ok {
		return managed.ExternalObservation{}, nil
	}

	cfg, exists, err := e.observePolicyConfiguration(ctx, projectID, configurationID)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	if !exists {
		return missingObservation(cr)
	}

	upToDate, err := isUpToDate(cr.Spec.ForProvider, cfg)
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	cr.Status.AtProvider = observationFromPolicyConfiguration(cfg)
	setProjectIDAnnotation(cr, projectID)
	cr.SetConditions(xpv2.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: upToDate,
	}, nil
}

func missingObservation(cr *v1alpha1.BranchPolicyCommentResolution) (managed.ExternalObservation, error) {
	if meta.GetExternalName(cr) != "" && cr.GetAnnotations()[annotationProjectID] == "" {
		return managed.ExternalObservation{}, errors.New(errImportProjectIDVerification)
	}
	return managed.ExternalObservation{ResourceExists: false}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.BranchPolicyCommentResolution) (managed.ExternalCreation, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	cfg, err := desiredPolicyConfiguration(cr.Spec.ForProvider, nil)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	var created *policy.PolicyConfiguration
	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var createErr error
		created, createErr = e.policies.CreatePolicyConfiguration(ctx, policy.CreatePolicyConfigurationArgs{
			Project:       stringPtr(projectID),
			Configuration: cfg,
		})
		return createErr
	})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreatePolicyConfiguration)
	}
	if created == nil || created.Id == nil {
		return managed.ExternalCreation{}, errors.New(errCreatePolicyConfigurationNoID)
	}

	meta.SetExternalName(cr, strconv.Itoa(*created.Id))
	cr.Status.AtProvider = observationFromPolicyConfiguration(created)
	setProjectIDAnnotation(cr, projectID)

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *v1alpha1.BranchPolicyCommentResolution) (managed.ExternalUpdate, error) {
	projectID, err := projectIDForExternalOperations(cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	if err := validateImmutableProjectID(cr, projectID); err != nil {
		return managed.ExternalUpdate{}, err
	}

	configurationID, ok, err := policyConfigurationIDFrom(cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errParsePolicyConfigurationID)
	}

	current, err := e.getPolicyConfiguration(ctx, projectID, configurationID)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errGetPolicyConfiguration)
	}
	if isPolicyDeleted(current) {
		return managed.ExternalUpdate{}, errors.New(errPolicyConfigurationDeleted)
	}
	if err := validateManagedPolicyConfiguration(current); err != nil {
		return managed.ExternalUpdate{}, err
	}

	cfg, err := desiredPolicyConfiguration(cr.Spec.ForProvider, current)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	var updated *policy.PolicyConfiguration
	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var updateErr error
		updated, updateErr = e.policies.UpdatePolicyConfiguration(ctx, policy.UpdatePolicyConfigurationArgs{
			Project:         stringPtr(projectID),
			ConfigurationId: intPtr(configurationID),
			Configuration:   cfg,
		})
		return updateErr
	})
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdatePolicyConfiguration)
	}

	cr.Status.AtProvider = observationFromPolicyConfiguration(updated)
	setProjectIDAnnotation(cr, projectID)
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.BranchPolicyCommentResolution) (managed.ExternalDelete, error) {
	projectID, err := projectIDForExternalOperations(cr)
	if err != nil {
		return managed.ExternalDelete{}, err
	}

	configurationID, ok, err := policyConfigurationIDFrom(cr)
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	if !ok {
		return managed.ExternalDelete{}, nil
	}

	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		return e.policies.DeletePolicyConfiguration(ctx, policy.DeletePolicyConfigurationArgs{
			Project:         stringPtr(projectID),
			ConfigurationId: intPtr(configurationID),
		})
	})
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(resource.Ignore(azuredevops.IsNotFound, err), errDeletePolicyConfiguration)
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

func (e *external) getPolicyConfiguration(ctx context.Context, projectID string, configurationID int) (*policy.PolicyConfiguration, error) {
	var cfg *policy.PolicyConfiguration
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var getErr error
		cfg, getErr = e.policies.GetPolicyConfiguration(ctx, policy.GetPolicyConfigurationArgs{
			Project:         stringPtr(projectID),
			ConfigurationId: intPtr(configurationID),
		})
		return getErr
	})
	return cfg, err
}

func (e *external) observePolicyConfiguration(ctx context.Context, projectID string, configurationID int) (*policy.PolicyConfiguration, bool, error) {
	cfg, err := e.getPolicyConfiguration(ctx, projectID, configurationID)
	if azuredevops.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.Wrap(err, errGetPolicyConfiguration)
	}
	if isPolicyDeleted(cfg) {
		return nil, false, nil
	}
	if err := validateManagedPolicyConfiguration(cfg); err != nil {
		return nil, false, err
	}

	return cfg, true, nil
}

func desiredPolicyConfiguration(p v1alpha1.BranchPolicyCommentResolutionParameters, current *policy.PolicyConfiguration) (*policy.PolicyConfiguration, error) {
	settings, err := desiredPolicySettings(p)
	if err != nil {
		return nil, err
	}

	cfg := &policy.PolicyConfiguration{
		IsEnabled:  boolPtr(p.Enabled),
		IsBlocking: boolPtr(p.Blocking),
		Settings:   settings,
		Type: &policy.PolicyTypeRef{
			Id:          &commentResolutionPolicyTypeID,
			DisplayName: stringPtr(commentResolutionPolicyTypeName),
		},
	}

	if current != nil {
		cfg.Id = current.Id
		cfg.Revision = current.Revision
		cfg.IsEnterpriseManaged = current.IsEnterpriseManaged
		if current.Type != nil {
			cfg.Type = current.Type
		}
	}

	return cfg, nil
}

func desiredPolicySettings(p v1alpha1.BranchPolicyCommentResolutionParameters) (map[string]interface{}, error) {
	scope, err := desiredPolicyScope(p)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		settingScope: []policyScope{scope},
	}, nil
}

func desiredPolicyScope(p v1alpha1.BranchPolicyCommentResolutionParameters) (policyScope, error) {
	repositoryID, err := getRepositoryID(p)
	if err != nil {
		return policyScope{}, err
	}

	refName, matchKind, err := branchScope(p.Branch)
	if err != nil {
		return policyScope{}, err
	}

	return policyScope{
		RepositoryID: repositoryID,
		RefName:      refName,
		MatchKind:    matchKind,
	}, nil
}

func isUpToDate(desired v1alpha1.BranchPolicyCommentResolutionParameters, current *policy.PolicyConfiguration) (bool, error) {
	if current == nil {
		return false, nil
	}
	if err := validateManagedPolicyConfiguration(current); err != nil {
		return false, err
	}
	if !matchesTopLevelConfiguration(desired, current) {
		return false, nil
	}

	return settingsUpToDate(desired, current.Settings)
}

func matchesTopLevelConfiguration(desired v1alpha1.BranchPolicyCommentResolutionParameters, current *policy.PolicyConfiguration) bool {
	if current.IsEnabled == nil || *current.IsEnabled != desired.Enabled {
		return false
	}
	if current.IsBlocking == nil || *current.IsBlocking != desired.Blocking {
		return false
	}

	return true
}

func settingsUpToDate(desired v1alpha1.BranchPolicyCommentResolutionParameters, raw interface{}) (bool, error) {
	currentSettings, err := settingsFromConfiguration(raw)
	if err != nil {
		return false, errors.Wrap(err, errDecodePolicyConfiguration)
	}

	desiredScope, err := desiredPolicyScope(desired)
	if err != nil {
		return false, err
	}
	want := commentResolutionPolicySettings{Scope: []policyScope{desiredScope}}

	return reflect.DeepEqual(want, currentSettings), nil
}

func settingsFromConfiguration(raw interface{}) (commentResolutionPolicySettings, error) {
	settings := commentResolutionPolicySettings{}

	data, err := json.Marshal(raw)
	if err != nil {
		return settings, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, err
	}
	for i := range settings.Scope {
		switch {
		case strings.EqualFold(settings.Scope[i].MatchKind, matchKindExact):
			settings.Scope[i].MatchKind = matchKindExact
		case strings.EqualFold(settings.Scope[i].MatchKind, matchKindPrefix):
			settings.Scope[i].MatchKind = matchKindPrefix
		}
	}

	return settings, nil
}

func settingsMapFrom(raw interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	settings := map[string]interface{}{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	return settings, nil
}

func hasOnlySupportedSettings(raw interface{}) (bool, error) {
	settings, err := settingsMapFrom(raw)
	if err != nil {
		return false, err
	}

	for key := range settings {
		if key != settingScope {
			return false, nil
		}
	}

	return true, nil
}

func validatePolicyType(cfg *policy.PolicyConfiguration) error {
	if cfg == nil || cfg.Type == nil || cfg.Type.Id == nil || *cfg.Type.Id != commentResolutionPolicyTypeID {
		return errors.New(errUnexpectedPolicyType)
	}
	return nil
}

func validateManagedPolicyConfiguration(cfg *policy.PolicyConfiguration) error {
	if err := validatePolicyType(cfg); err != nil {
		return err
	}

	settings, err := settingsFromConfiguration(cfg.Settings)
	if err != nil {
		return errors.Wrap(err, errDecodePolicyConfiguration)
	}
	if len(settings.Scope) != 1 || settings.Scope[0].RepositoryID == "" {
		return errors.New(errUnsupportedPolicyScope)
	}
	supported, err := hasOnlySupportedSettings(cfg.Settings)
	if err != nil {
		return errors.Wrap(err, errDecodePolicyConfiguration)
	}
	if !supported {
		return errors.New(errUnsupportedPolicySettings)
	}

	return nil
}

func observationFromPolicyConfiguration(cfg *policy.PolicyConfiguration) v1alpha1.BranchPolicyCommentResolutionObservation {
	observation := v1alpha1.BranchPolicyCommentResolutionObservation{}
	if cfg == nil {
		return observation
	}
	if cfg.Id != nil {
		observation.ID = strconv.Itoa(*cfg.Id)
	}
	return observation
}

func getProjectID(cr *v1alpha1.BranchPolicyCommentResolution) (string, error) {
	if cr.Spec.ForProvider.ProjectID == "" {
		return "", errors.New(errMissingProjectID)
	}
	return cr.Spec.ForProvider.ProjectID, nil
}

func projectIDForExternalOperations(cr *v1alpha1.BranchPolicyCommentResolution) (string, error) {
	if !cr.GetDeletionTimestamp().IsZero() {
		if existing := cr.GetAnnotations()[annotationProjectID]; existing != "" {
			return existing, nil
		}
	}
	return getProjectID(cr)
}

func getRepositoryID(p v1alpha1.BranchPolicyCommentResolutionParameters) (string, error) {
	if p.RepositoryID == "" {
		return "", errors.New(errMissingRepositoryID)
	}
	return p.RepositoryID, nil
}

func branchScope(branch string) (string, string, error) {
	if branch == "" {
		return "", "", errors.New(errMissingBranch)
	}
	if strings.Count(branch, "*") > 1 || (strings.Contains(branch, "*") && !strings.HasSuffix(branch, "*")) {
		return "", "", errors.New(errInvalidBranch)
	}
	if strings.HasSuffix(branch, "*") {
		refName := strings.TrimSuffix(branch, "*")
		if refName == "" {
			return "", "", errors.New(errInvalidBranch)
		}
		return refName, matchKindPrefix, nil
	}
	return branch, matchKindExact, nil
}

func validateImmutableProjectID(cr *v1alpha1.BranchPolicyCommentResolution, projectID string) error {
	if existing := cr.GetAnnotations()[annotationProjectID]; existing != "" && existing != projectID {
		return errors.New(errImmutableProjectID)
	}
	return nil
}

func setProjectIDAnnotation(cr *v1alpha1.BranchPolicyCommentResolution, projectID string) {
	annotations := cr.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[annotationProjectID] = projectID
	cr.SetAnnotations(annotations)
}

func policyConfigurationIDFrom(cr *v1alpha1.BranchPolicyCommentResolution) (int, bool, error) {
	id := meta.GetExternalName(cr)
	if id == "" {
		id = cr.Status.AtProvider.ID
	}
	if id == "" {
		return 0, false, nil
	}

	parsed, err := strconv.Atoi(id)
	if err != nil {
		return 0, false, errors.Wrap(err, errParsePolicyConfigurationID)
	}
	return parsed, true, nil
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
