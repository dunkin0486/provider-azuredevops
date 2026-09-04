// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package variablegroup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
	corev1 "k8s.io/api/core/v1"
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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/variablegroup/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig            = "cannot get Azure DevOps client config"
	errNewClient            = "cannot create new Azure DevOps variable group client"
	errGetVariableGroup     = "cannot get variable group"
	errCreateVariableGroup  = "cannot create variable group"
	errUpdateVariableGroup  = "cannot update variable group"
	errDeleteVariableGroup  = "cannot delete variable group"
	errParseVariableGroupID = "cannot parse variable group id"
	errParseProjectID       = "cannot parse project id"
	errParseServiceEndpoint = "cannot parse service endpoint id"
	errResolveSecret        = "cannot resolve variable value from secret"
	errInvalidParameters    = "invalid variable group parameters"
)

const (
	variableGroupTypeVsts          = "Vsts"
	variableGroupTypeAzureKeyVault = "AzureKeyVault"
)

// annotationSecretsHash stores a SHA-256 hash of the currently-applied
// secret-valued variables' plaintext values. Azure DevOps never returns
// secret variable values back on read, so there is no way to detect drift
// (e.g. a rotated Kubernetes Secret) by comparing against the observed
// variable group alone -- this annotation lets Observe detect that the
// *referenced* Secret's value has changed since the last Create/Update and
// force a resync.
const annotationSecretsHash = "variablegroup.azuredevops.crossplane.io/secrets-hash"

// SetupGated adds a controller that reconciles VariableGroup managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup VariableGroup controller"))
		}
	}, v1alpha1.VariableGroupGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles VariableGroup managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.VariableGroupGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.VariableGroup](&connector{kube: mgr.GetClient()}),
		managed.WithInitializers(),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.VariableGroupList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.VariableGroupList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.VariableGroupGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.VariableGroup{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves cr's ProviderConfig via the shared azuredevops client
// package and uses it to construct the TaskAgent API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.VariableGroup) (managed.TypedExternalClient[*v1alpha1.VariableGroup], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	vgClient, err := newVariableGroupClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{kube: c.kube, variablegroups: vgClient}, nil
}

type external struct {
	kube           client.Client
	variablegroups VariableGroupClient
}

func (c *external) Observe(ctx context.Context, cr *v1alpha1.VariableGroup) (managed.ExternalObservation, error) {
	id, ok, err := variableGroupIDFrom(cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetVariableGroup)
	}
	if !ok {
		return managed.ExternalObservation{}, nil
	}

	projectID, err := parseProjectID(cr.Spec.ForProvider.ProjectID)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetVariableGroup)
	}
	project := projectID.String()

	vg, err := c.variablegroups.GetVariableGroup(ctx, taskagent.GetVariableGroupArgs{
		Project: &project,
		GroupId: &id,
	})
	if azuredevops.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetVariableGroup)
	}

	cr.Status.AtProvider = observationFromVariableGroup(vg)
	cr.SetConditions(xpv2.Available())

	upToDate, err := isUpToDate(cr.Spec.ForProvider, vg)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetVariableGroup)
	}
	if upToDate {
		upToDate, err = c.secretsUpToDate(ctx, cr)
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errGetVariableGroup)
		}
	}

	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

// secretsUpToDate detects drift that isUpToDate never can: Azure DevOps
// never returns secret-variable values back on read, so the only way to
// tell that a referenced Kubernetes Secret's value has since changed (e.g.
// a rotated credential) is to re-resolve it now and compare its hash
// against the one captured at the last Create/Update.
func (c *external) secretsUpToDate(ctx context.Context, cr *v1alpha1.VariableGroup) (bool, error) {
	if cr.Spec.ForProvider.KeyVault != nil {
		return true, nil
	}
	hash, err := c.currentSecretsHash(ctx, cr.Spec.ForProvider)
	if err != nil {
		return false, err
	}
	if hash == "" {
		return true, nil
	}
	return hash == cr.GetAnnotations()[annotationSecretsHash], nil
}

func (c *external) Create(ctx context.Context, cr *v1alpha1.VariableGroup) (managed.ExternalCreation, error) {
	payload, err := c.buildVariableGroupParameters(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateVariableGroup)
	}

	vg, err := c.variablegroups.AddVariableGroup(ctx, taskagent.AddVariableGroupArgs{VariableGroupParameters: payload})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateVariableGroup)
	}

	if vg != nil && vg.Id != nil {
		id := strconv.Itoa(*vg.Id)
		meta.SetExternalName(cr, id)
	}
	cr.Status.AtProvider = observationFromVariableGroup(vg)
	setSecretsHashAnnotation(cr, payload.Variables)

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, cr *v1alpha1.VariableGroup) (managed.ExternalUpdate, error) {
	id, ok, err := variableGroupIDFrom(cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateVariableGroup)
	}
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errUpdateVariableGroup)
	}

	payload, err := c.buildVariableGroupParameters(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateVariableGroup)
	}

	vg, err := c.variablegroups.UpdateVariableGroup(ctx, taskagent.UpdateVariableGroupArgs{
		GroupId:                 &id,
		VariableGroupParameters: payload,
	})
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateVariableGroup)
	}

	cr.Status.AtProvider = observationFromVariableGroup(vg)
	setSecretsHashAnnotation(cr, payload.Variables)
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, cr *v1alpha1.VariableGroup) (managed.ExternalDelete, error) {
	id, ok, err := variableGroupIDFrom(cr)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteVariableGroup)
	}
	if !ok {
		return managed.ExternalDelete{}, nil
	}

	// Deliberately avoid buildVariableGroupParameters here: it resolves
	// secret variable values via k8s Secret lookups, which are irrelevant
	// for deletion (only the project ID(s) are needed) and can fail if a
	// referenced Secret has already been removed during a cascading
	// teardown -- that would wrongly block Delete forever since the
	// finalizer could never be released.
	projectIDs, err := projectIDsForDelete(cr.Spec.ForProvider)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteVariableGroup)
	}

	err = c.variablegroups.DeleteVariableGroup(ctx, taskagent.DeleteVariableGroupArgs{
		GroupId:    &id,
		ProjectIds: &projectIDs,
	})
	if azuredevops.IsNotFound(err) {
		return managed.ExternalDelete{}, nil
	}
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteVariableGroup)
	}

	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(_ context.Context) error {
	return nil
}

func (c *external) buildVariableGroupParameters(ctx context.Context, cr *v1alpha1.VariableGroup) (*taskagent.VariableGroupParameters, error) {
	p := cr.Spec.ForProvider
	if err := validateParameters(p); err != nil {
		return nil, errors.Wrap(err, errInvalidParameters)
	}

	projectRef, _, err := buildProjectReference(p)
	if err != nil {
		return nil, err
	}

	payload := &taskagent.VariableGroupParameters{
		Name:        &p.Name,
		Description: &p.Description,
		VariableGroupProjectReferences: &[]taskagent.VariableGroupProjectReference{{
			Name:             &p.Name,
			Description:      &p.Description,
			ProjectReference: projectRef,
		}},
	}

	if p.KeyVault != nil {
		serviceEndpointID, err := parseServiceEndpointID(p.KeyVault.ServiceEndpointID)
		if err != nil {
			return nil, err
		}
		groupType := variableGroupTypeAzureKeyVault
		payload.Type = &groupType
		payload.ProviderData = &taskagent.AzureKeyVaultVariableGroupProviderData{
			ServiceEndpointId: serviceEndpointID,
			Vault:             &p.KeyVault.Name,
		}
		return payload, nil
	}

	variables, err := c.resolveVariables(ctx, p.Variables)
	if err != nil {
		return nil, err
	}
	groupType := variableGroupTypeVsts
	payload.Type = &groupType
	payload.Variables = &variables
	return payload, nil
}

func (c *external) resolveVariables(ctx context.Context, variables []v1alpha1.VariableGroupVariable) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(variables))
	for _, variable := range variables {
		value := variable.Value
		if variable.ValueFrom != nil {
			resolved, err := c.resolveSecretValue(ctx, variable.ValueFrom.SecretKeyRef)
			if err != nil {
				return nil, err
			}
			value = resolved
		}

		v := taskagent.VariableValue{}
		if value != "" || variable.ValueFrom != nil {
			v.Value = &value
		}
		if variable.IsSecret {
			v.IsSecret = boolPtr(true)
		}
		out[variable.Name] = v
	}
	return out, nil
}

func (c *external) resolveSecretValue(ctx context.Context, selector xpv2.SecretKeySelector) (string, error) {
	secret := &corev1.Secret{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: selector.Namespace}, secret); err != nil {
		return "", errors.Wrap(err, errResolveSecret)
	}
	value, ok := secret.Data[selector.Key]
	if !ok {
		return "", errors.Wrap(errors.Errorf("secret %s/%s does not contain key %q", selector.Namespace, selector.Name, selector.Key), errResolveSecret)
	}
	return string(value), nil
}

// currentSecretsHash re-resolves p's secret-valued variables from their
// referenced Kubernetes Secrets right now and returns a hash of their
// current values, so Observe can detect drift that Azure DevOps' API can
// never surface (it never returns secret values back). Returns "" if there
// are no secret variables at all.
func (c *external) currentSecretsHash(ctx context.Context, p v1alpha1.VariableGroupParameters) (string, error) {
	resolved, err := c.resolveVariables(ctx, p.Variables)
	if err != nil {
		return "", err
	}
	return secretsHashFromVariables(&resolved)
}

// setSecretsHashAnnotation records a hash of payload's secret-valued
// variables on cr so a future Observe can detect if the referenced Secret's
// value has since changed (see annotationSecretsHash).
func setSecretsHashAnnotation(cr *v1alpha1.VariableGroup, variables *map[string]interface{}) {
	hash, err := secretsHashFromVariables(variables)
	if err != nil || hash == "" {
		return
	}
	meta.AddAnnotations(cr, map[string]string{annotationSecretsHash: hash})
}

// secretsHashFromVariables computes a deterministic SHA-256 hash over the
// name/value pairs of every variable in variables that is marked secret.
// Returns "" if there are no secret variables, so callers can distinguish
// "no secrets to track" from "hash of zero-length secrets".
func secretsHashFromVariables(variables *map[string]interface{}) (string, error) {
	if variables == nil {
		return "", nil
	}
	names := make([]string, 0, len(*variables))
	values := make(map[string]string, len(*variables))
	for name, raw := range *variables {
		vv, err := decodeVariableValue(raw)
		if err != nil {
			return "", err
		}
		if !boolValue(vv.IsSecret) {
			continue
		}
		names = append(names, name)
		values[name] = valueOrEmpty(vv.Value)
	}
	if len(names) == 0 {
		return "", nil
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write([]byte(values[name]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func observationFromVariableGroup(vg *taskagent.VariableGroup) v1alpha1.VariableGroupObservation {
	o := v1alpha1.VariableGroupObservation{}
	if vg != nil && vg.Id != nil {
		o.ID = strconv.Itoa(*vg.Id)
	}
	return o
}

func variableGroupIDFrom(cr *v1alpha1.VariableGroup) (int, bool, error) {
	id := meta.GetExternalName(cr)
	if id == "" {
		id = cr.Status.AtProvider.ID
	}
	if id == "" {
		return 0, false, nil
	}
	parsed, err := strconv.Atoi(id)
	if err != nil {
		return 0, false, errors.Wrap(err, errParseVariableGroupID)
	}
	return parsed, true, nil
}

func validateParameters(p v1alpha1.VariableGroupParameters) error {
	if p.ProjectID == "" {
		return errors.New("projectId is required")
	}

	for _, variable := range p.Variables {
		if err := validateVariable(variable); err != nil {
			return err
		}
	}
	if err := validateUniqueVariableNames(p.Variables); err != nil {
		return err
	}

	if p.KeyVault != nil {
		return validateKeyVaultParameters(p)
	}

	return nil
}

func validateUniqueVariableNames(variables []v1alpha1.VariableGroupVariable) error {
	seen := map[string]struct{}{}
	for _, variable := range variables {
		if _, ok := seen[variable.Name]; ok {
			return errors.Errorf("variables[%s] is duplicated", variable.Name)
		}
		seen[variable.Name] = struct{}{}
	}
	return nil
}

func validateVariable(variable v1alpha1.VariableGroupVariable) error {
	if variable.Name == "" {
		return errors.New("variables.name is required")
	}
	if variable.Value != "" && variable.ValueFrom != nil {
		return errors.Errorf("variable %q may set only one of value or valueFrom", variable.Name)
	}
	if !variable.IsSecret {
		return nil
	}
	if variable.Value != "" {
		return errors.Errorf("variable %q isSecret=true requires valueFrom and forbids plaintext value", variable.Name)
	}
	if variable.ValueFrom == nil {
		return errors.Errorf("variable %q isSecret=true requires valueFrom", variable.Name)
	}
	return nil
}

func validateKeyVaultParameters(p v1alpha1.VariableGroupParameters) error {
	if p.KeyVault.ServiceEndpointID == "" {
		return errors.New("keyVault.serviceEndpointId is required")
	}
	if p.KeyVault.Name == "" {
		return errors.New("keyVault.name is required")
	}
	if len(p.Variables) > 0 {
		return errors.New("keyVault-linked variable groups do not support inline variables")
	}
	return nil
}

func buildProjectReference(p v1alpha1.VariableGroupParameters) (*taskagent.ProjectReference, []string, error) {
	projectID, err := parseProjectID(p.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	id := projectID.String()
	return &taskagent.ProjectReference{Id: projectID}, []string{id}, nil
}

// projectIDsForDelete returns the project ID(s) needed to call
// DeleteVariableGroup without resolving variable values/secrets, which are
// irrelevant for deletion and must not be able to block it.
func projectIDsForDelete(p v1alpha1.VariableGroupParameters) ([]string, error) {
	if p.ProjectID == "" {
		return nil, errors.New("projectId is required")
	}
	_, projectIDs, err := buildProjectReference(p)
	return projectIDs, err
}

func parseProjectID(s string) (*uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, errors.Wrap(err, errParseProjectID)
	}
	return &id, nil
}

func parseServiceEndpointID(s string) (*uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, errors.Wrap(err, errParseServiceEndpoint)
	}
	return &id, nil
}

func isUpToDate(p v1alpha1.VariableGroupParameters, vg *taskagent.VariableGroup) (bool, error) {
	if vg == nil {
		return false, nil
	}
	if !metadataMatches(p, vg) {
		return false, nil
	}
	if !projectReferencesMatch(p.ProjectID, vg.VariableGroupProjectReferences) {
		return false, nil
	}

	observedType := normalizeType(vg.Type)
	if p.KeyVault != nil {
		return keyVaultUpToDate(p.KeyVault, observedType, vg.ProviderData)
	}

	return variablesUpToDate(p.Variables, observedType, vg.Variables)
}

func metadataMatches(p v1alpha1.VariableGroupParameters, vg *taskagent.VariableGroup) bool {
	return valueOrEmpty(vg.Name) == p.Name && valueOrEmpty(vg.Description) == p.Description
}

func keyVaultUpToDate(desired *v1alpha1.KeyVaultReference, observedType string, providerData interface{}) (bool, error) {
	if observedType != variableGroupTypeAzureKeyVault {
		return false, nil
	}
	observed, err := decodeProviderData(providerData)
	if err != nil {
		return false, err
	}
	if observed == nil {
		return false, nil
	}
	return valueOrEmpty(observed.Vault) == desired.Name &&
		uuidStringOrEmpty(observed.ServiceEndpointId) == desired.ServiceEndpointID, nil
}

func variablesUpToDate(desired []v1alpha1.VariableGroupVariable, observedType string, observedRaw *map[string]interface{}) (bool, error) {
	if observedType != variableGroupTypeVsts {
		return false, nil
	}
	observedVariables, err := observedVariableValues(observedRaw)
	if err != nil {
		return false, err
	}
	if len(observedVariables) != len(desired) {
		return false, nil
	}

	for _, variable := range desired {
		observed, ok := observedVariables[variable.Name]
		if !ok {
			return false, nil
		}
		if boolValue(observed.IsSecret) != variable.IsSecret {
			return false, nil
		}
		if variable.IsSecret {
			// Azure DevOps does not return secret variable values on read, so we
			// only compare that the variable exists and remains marked secret.
			continue
		}
		if valueOrEmpty(observed.Value) != variable.Value {
			return false, nil
		}
	}
	return true, nil
}

func projectReferencesMatch(projectID string, refs *[]taskagent.VariableGroupProjectReference) bool {
	if refs == nil || len(*refs) != 1 {
		return false
	}
	ref := (*refs)[0]
	if ref.ProjectReference == nil || ref.ProjectReference.Id == nil {
		return false
	}
	return strings.EqualFold(ref.ProjectReference.Id.String(), projectID)
}

func observedVariableValues(in *map[string]interface{}) (map[string]taskagent.VariableValue, error) {
	out := map[string]taskagent.VariableValue{}
	if in == nil {
		return out, nil
	}
	for name, value := range *in {
		vv, err := decodeVariableValue(value)
		if err != nil {
			return nil, err
		}
		out[name] = vv
	}
	return out, nil
}

func decodeVariableValue(v interface{}) (taskagent.VariableValue, error) {
	switch current := v.(type) {
	case taskagent.VariableValue:
		return current, nil
	case *taskagent.VariableValue:
		if current == nil {
			return taskagent.VariableValue{}, nil
		}
		return *current, nil
	default:
		out := taskagent.VariableValue{}
		if err := remarshal(current, &out); err != nil {
			return taskagent.VariableValue{}, err
		}
		return out, nil
	}
}

func decodeProviderData(v interface{}) (*taskagent.AzureKeyVaultVariableGroupProviderData, error) {
	switch current := v.(type) {
	case nil:
		return nil, nil
	case taskagent.AzureKeyVaultVariableGroupProviderData:
		return &current, nil
	case *taskagent.AzureKeyVaultVariableGroupProviderData:
		return current, nil
	default:
		out := &taskagent.AzureKeyVaultVariableGroupProviderData{}
		if err := remarshal(current, out); err != nil {
			return nil, err
		}
		return out, nil
	}
}

func remarshal(in, out interface{}) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func normalizeType(t *string) string {
	if t == nil || *t == "" {
		return variableGroupTypeVsts
	}
	return *t
}

func valueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func uuidStringOrEmpty(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func boolValue(b *bool) bool {
	return b != nil && *b
}

func boolPtr(v bool) *bool {
	return &v
}
