// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package builddefinition

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/google/uuid"
	adobuild "github.com/microsoft/azure-devops-go-api/azuredevops/v7/build"
	adocore "github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"

	"k8s.io/apimachinery/pkg/types"
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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/builddefinition/v1alpha1"
	gitrepositoryv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/gitrepository/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig                      = "cannot get Azure DevOps client config"
	errNewClient                      = "cannot create new Azure DevOps build client"
	errMissingProjectID               = "spec.forProvider.projectId is required"
	errMissingRepositoryID            = "spec.forProvider.repositoryId is required"
	errGetBuildDefinition             = "cannot get build definition"
	errCreateBuildDefinition          = "cannot create build definition"
	errCreateBuildDefinitionNoID      = "created build definition did not include an id"
	errUpdateBuildDefinition          = "cannot update build definition"
	errDeleteBuildDefinition          = "cannot delete build definition"
	errParseBuildDefinitionID         = "cannot parse build definition id"
	errParseProjectID                 = "cannot parse project id"
	errParseVariableGroupID           = "cannot parse variable group id"
	errGetReferencedGitRepository     = "cannot get referenced GitRepository"
	errImmutableProjectID             = "projectId is immutable once created"
	repositoryTypeTfsGit              = "TfsGit"
	yamlProcessType               int = 2
)

// SetupGated adds a controller that reconciles BuildDefinition managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup BuildDefinition controller"))
		}
	}, v1alpha1.BuildDefinitionGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles BuildDefinition managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.BuildDefinitionGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.BuildDefinition](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.BuildDefinitionList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.BuildDefinitionList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.BuildDefinitionGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.BuildDefinition{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves cr's ProviderConfig via the shared azuredevops client package
// and uses it to construct the Build API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.BuildDefinition) (managed.TypedExternalClient[*v1alpha1.BuildDefinition], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	buildClient, err := newBuildDefinitionClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{kube: c.kube, builddefinitions: buildClient}, nil
}

type external struct {
	kube             client.Client
	builddefinitions BuildDefinitionClient
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.BuildDefinition) (managed.ExternalObservation, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	definitionID, ok, err := buildDefinitionIDFrom(cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetBuildDefinition)
	}
	if !ok {
		return managed.ExternalObservation{}, nil
	}

	definition, err := e.builddefinitions.GetDefinition(ctx, adobuild.GetDefinitionArgs{
		Project:      stringPtr(projectID),
		DefinitionId: intPtr(definitionID),
	})
	if azuredevops.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetBuildDefinition)
	}

	cr.Status.AtProvider = observationFromBuildDefinition(definition)
	cr.SetConditions(xpv2.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, definition),
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.BuildDefinition) (managed.ExternalCreation, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	definition, err := e.desiredBuildDefinition(ctx, cr, nil)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	created, err := e.builddefinitions.CreateDefinition(ctx, adobuild.CreateDefinitionArgs{
		Project:    stringPtr(projectID),
		Definition: definition,
	})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBuildDefinition)
	}
	if created == nil || created.Id == nil {
		return managed.ExternalCreation{}, errors.New(errCreateBuildDefinitionNoID)
	}

	id := strconv.Itoa(*created.Id)
	meta.SetExternalName(cr, id)
	cr.Status.AtProvider = observationFromBuildDefinition(created)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *v1alpha1.BuildDefinition) (managed.ExternalUpdate, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	definitionID, ok, err := buildDefinitionIDFrom(cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateBuildDefinition)
	}
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errParseBuildDefinitionID)
	}

	current, err := e.builddefinitions.GetDefinition(ctx, adobuild.GetDefinitionArgs{
		Project:      stringPtr(projectID),
		DefinitionId: intPtr(definitionID),
	})
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errGetBuildDefinition)
	}

	definition, err := e.desiredBuildDefinition(ctx, cr, current)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	definition.Revision = current.Revision

	updated, err := e.builddefinitions.UpdateDefinition(ctx, adobuild.UpdateDefinitionArgs{
		Project:      stringPtr(projectID),
		DefinitionId: intPtr(definitionID),
		Definition:   definition,
	})
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateBuildDefinition)
	}

	cr.Status.AtProvider = observationFromBuildDefinition(updated)
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.BuildDefinition) (managed.ExternalDelete, error) {
	projectID, err := getProjectID(cr)
	if err != nil {
		return managed.ExternalDelete{}, err
	}

	definitionID, ok, err := buildDefinitionIDFrom(cr)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteBuildDefinition)
	}
	if !ok {
		return managed.ExternalDelete{}, nil
	}

	err = resource.Ignore(azuredevops.IsNotFound, e.builddefinitions.DeleteDefinition(ctx, adobuild.DeleteDefinitionArgs{
		Project:      stringPtr(projectID),
		DefinitionId: intPtr(definitionID),
	}))
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteBuildDefinition)
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

func (e *external) desiredBuildDefinition(ctx context.Context, cr *v1alpha1.BuildDefinition, current *adobuild.BuildDefinition) (*adobuild.BuildDefinition, error) {
	if err := validateImmutableFields(cr.Spec.ForProvider, current); err != nil {
		return nil, errors.Wrap(err, errUpdateBuildDefinition)
	}

	projectUUID, err := uuid.Parse(cr.Spec.ForProvider.ProjectID)
	if err != nil {
		return nil, errors.Wrap(err, errParseProjectID)
	}

	definition := &adobuild.BuildDefinition{}
	if current != nil {
		cloned := *current
		definition = &cloned
	}

	repository, err := e.desiredRepository(ctx, cr, current)
	if err != nil {
		return nil, err
	}
	variableGroups, err := desiredVariableGroups(cr.Spec.ForProvider.VariableGroupIDs)
	if err != nil {
		return nil, err
	}

	definition.Name = stringPtr(cr.Spec.ForProvider.Name)
	definition.Path = stringPtr(normalizePath(cr.Spec.ForProvider.Path))
	definition.Project = &adocore.TeamProjectReference{Id: &projectUUID}
	definition.Type = definitionTypePtr(adobuild.DefinitionTypeValues.Build)
	definition.Repository = repository
	// Azure DevOps identifies YAML-backed build definitions with process type 2.
	definition.Process = &adobuild.YamlProcess{Type: intPtr(yamlProcessType), YamlFilename: stringPtr(cr.Spec.ForProvider.YamlPath)}
	definition.Triggers = desiredTriggers(cr.Spec.ForProvider)
	definition.VariableGroups = variableGroups

	return definition, nil
}

func (e *external) desiredRepository(ctx context.Context, cr *v1alpha1.BuildDefinition, current *adobuild.BuildDefinition) (*adobuild.BuildRepository, error) { //nolint:gocyclo // Straightforward repository payload construction and optional reference hydration.
	if cr.Spec.ForProvider.RepositoryID == "" {
		return nil, errors.New(errMissingRepositoryID)
	}

	repository := &adobuild.BuildRepository{
		Id:   stringPtr(cr.Spec.ForProvider.RepositoryID),
		Type: stringPtr(repositoryTypeTfsGit),
	}

	if current != nil && current.Repository != nil && current.Repository.Id != nil && *current.Repository.Id == cr.Spec.ForProvider.RepositoryID {
		repository.Name = current.Repository.Name
		repository.DefaultBranch = current.Repository.DefaultBranch
		repository.Url = current.Repository.Url
	}

	if cr.Spec.ForProvider.RepositoryIDRef == nil {
		return repository, nil
	}

	ref := *cr.Spec.ForProvider.RepositoryIDRef
	name := types.NamespacedName{Name: ref.Name, Namespace: cr.GetNamespace()}
	if ref.Namespace != "" {
		name.Namespace = ref.Namespace
	}

	gr := &gitrepositoryv1alpha1.GitRepository{}
	if err := e.kube.Get(ctx, name, gr); err != nil {
		return nil, errors.Wrap(err, errGetReferencedGitRepository)
	}

	repository.Name = stringPtr(gr.Spec.ForProvider.Name)
	if gr.Spec.ForProvider.DefaultBranch != "" {
		repository.DefaultBranch = stringPtr(gr.Spec.ForProvider.DefaultBranch)
	}
	if gr.Status.AtProvider.RemoteURL != "" {
		repository.Url = stringPtr(gr.Status.AtProvider.RemoteURL)
	}

	return repository, nil
}

func getProjectID(cr *v1alpha1.BuildDefinition) (string, error) {
	if cr.Spec.ForProvider.ProjectID == "" {
		return "", errors.New(errMissingProjectID)
	}
	return cr.Spec.ForProvider.ProjectID, nil
}

func buildDefinitionIDFrom(cr *v1alpha1.BuildDefinition) (int, bool, error) {
	id := meta.GetExternalName(cr)
	if id == "" {
		id = cr.Status.AtProvider.ID
	}
	if id == "" {
		return 0, false, nil
	}
	parsed, err := strconv.Atoi(id)
	if err != nil {
		return 0, false, errors.Wrap(err, errParseBuildDefinitionID)
	}
	return parsed, true, nil
}

func desiredVariableGroups(groups []string) (*[]adobuild.VariableGroup, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	desired := make([]adobuild.VariableGroup, 0, len(groups))
	for _, group := range groups {
		if group == "" {
			return nil, errors.New(errParseVariableGroupID)
		}
		id, err := strconv.Atoi(group)
		if err != nil {
			return nil, errors.Wrap(err, errParseVariableGroupID)
		}
		desired = append(desired, adobuild.VariableGroup{Id: intPtr(id)})
	}
	return &desired, nil
}

func desiredTriggers(p v1alpha1.BuildDefinitionParameters) *[]interface{} {
	triggers := make([]interface{}, 0, 2)
	if boolValue(p.CIEnabled) {
		triggers = append(triggers, &adobuild.ContinuousIntegrationTrigger{
			TriggerType: definitionTriggerTypePtr(adobuild.DefinitionTriggerTypeValues.ContinuousIntegration),
		})
	}
	if boolValue(p.PREnabled) {
		triggers = append(triggers, &adobuild.PullRequestTrigger{
			TriggerType: definitionTriggerTypePtr(adobuild.DefinitionTriggerTypeValues.PullRequest),
		})
	}
	if len(triggers) == 0 {
		return nil
	}
	return &triggers
}

func observationFromBuildDefinition(definition *adobuild.BuildDefinition) v1alpha1.BuildDefinitionObservation {
	observation := v1alpha1.BuildDefinitionObservation{}
	if definition == nil {
		return observation
	}
	if definition.Id != nil {
		observation.ID = strconv.Itoa(*definition.Id)
	}
	if definition.Revision != nil {
		observation.Revision = *definition.Revision
	}
	if definition.Url != nil {
		observation.URL = *definition.Url
	}
	return observation
}

func isUpToDate(desired v1alpha1.BuildDefinitionParameters, current *adobuild.BuildDefinition) bool { //nolint:gocyclo // Straightforward desired-versus-observed field comparison.
	if current == nil {
		return false
	}
	if valueOrEmpty(current.Name) != desired.Name {
		return false
	}
	if valueOrEmpty(current.Path) != normalizePath(desired.Path) {
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
	if current.Type == nil || *current.Type != adobuild.DefinitionTypeValues.Build {
		return false
	}
	if valueOrEmpty(buildRepositoryID(current.Repository)) != desired.RepositoryID {
		return false
	}
	if !yamlProcessMatches(desired.YamlPath, current.Process) {
		return false
	}
	if !triggerConfigurationMatches(desired, current.Triggers) {
		return false
	}
	return variableGroupsMatch(desired.VariableGroupIDs, current.VariableGroups)
}

func validateImmutableFields(desired v1alpha1.BuildDefinitionParameters, current *adobuild.BuildDefinition) error {
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
	return nil
}

func variableGroupsMatch(desired []string, current *[]adobuild.VariableGroup) bool {
	if len(desired) == 0 {
		return current == nil || len(*current) == 0
	}
	if current == nil || len(desired) != len(*current) {
		return false
	}

	observed := map[string]struct{}{}
	for _, group := range *current {
		if group.Id == nil {
			return false
		}
		observed[strconv.Itoa(*group.Id)] = struct{}{}
	}
	for _, group := range desired {
		if _, ok := observed[group]; !ok {
			return false
		}
	}
	return true
}

func triggerConfigurationMatches(desired v1alpha1.BuildDefinitionParameters, triggers *[]interface{}) bool {
	expectedCI := boolValue(desired.CIEnabled)
	expectedPR := boolValue(desired.PREnabled)
	if !expectedCI && !expectedPR {
		return triggers == nil || len(*triggers) == 0
	}
	if triggers == nil {
		return false
	}

	foundCI := false
	foundPR := false
	for _, raw := range *triggers {
		switch triggerTypeFrom(raw) {
		case string(adobuild.DefinitionTriggerTypeValues.ContinuousIntegration):
			foundCI = true
		case string(adobuild.DefinitionTriggerTypeValues.PullRequest):
			foundPR = true
		default:
			return false
		}
	}

	return foundCI == expectedCI && foundPR == expectedPR && len(*triggers) == boolCount(expectedCI)+boolCount(expectedPR)
}

func yamlProcessMatches(expectedPath string, process interface{}) bool {
	filename, ok := yamlFilenameFromProcess(process)
	return ok && filename == expectedPath
}

func yamlFilenameFromProcess(process interface{}) (string, bool) {
	switch p := process.(type) {
	case *adobuild.YamlProcess:
		return valueOrEmpty(p.YamlFilename), true
	case adobuild.YamlProcess:
		return valueOrEmpty(p.YamlFilename), true
	case map[string]interface{}:
		filename, ok := p["yamlFilename"].(string)
		return filename, ok
	default:
		raw, err := json.Marshal(process)
		if err != nil {
			return "", false
		}
		decoded := &adobuild.YamlProcess{}
		if err := json.Unmarshal(raw, decoded); err != nil {
			return "", false
		}
		if decoded.YamlFilename == nil {
			return "", false
		}
		return *decoded.YamlFilename, true
	}
}

func triggerTypeFrom(trigger interface{}) string {
	switch t := trigger.(type) {
	case *adobuild.ContinuousIntegrationTrigger:
		return stringValueDefinitionTriggerType(t.TriggerType)
	case adobuild.ContinuousIntegrationTrigger:
		return stringValueDefinitionTriggerType(t.TriggerType)
	case *adobuild.PullRequestTrigger:
		return stringValueDefinitionTriggerType(t.TriggerType)
	case adobuild.PullRequestTrigger:
		return stringValueDefinitionTriggerType(t.TriggerType)
	case map[string]interface{}:
		if value, ok := t["triggerType"].(string); ok {
			return value
		}
	}
	return ""
}

func buildRepositoryID(repository *adobuild.BuildRepository) *string {
	if repository == nil {
		return nil
	}
	return repository.Id
}

func normalizePath(path string) string {
	if path == "" {
		return "\\"
	}
	return path
}

func boolValue(b *bool) bool {
	return b != nil && *b
}

func boolCount(v bool) int {
	if v {
		return 1
	}
	return 0
}

func valueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func stringValueDefinitionTriggerType(v *adobuild.DefinitionTriggerType) string {
	if v == nil {
		return ""
	}
	return string(*v)
}

func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func definitionTypePtr(v adobuild.DefinitionType) *adobuild.DefinitionType {
	return &v
}

func definitionTriggerTypePtr(v adobuild.DefinitionTriggerType) *adobuild.DefinitionTriggerType {
	return &v
}
