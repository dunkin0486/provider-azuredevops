// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package serviceendpointazurerm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

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
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/serviceendpoint"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointazurerm/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig               = "cannot get Azure DevOps client config"
	errNewClient               = "cannot create new Azure DevOps service endpoint client"
	errGetServiceEndpoint      = "cannot get service endpoint"
	errCreateServiceEndpoint   = "cannot create service endpoint"
	errUpdateServiceEndpoint   = "cannot update service endpoint"
	errDeleteServiceEndpoint   = "cannot delete service endpoint"
	errResolveClientSecret     = "cannot resolve service principal client secret"
	errInvalidCredentials      = "exactly one of servicePrincipal or workloadIdentityFederation credentials must be configured"
	errMissingProjectID        = "projectId is required"
	errMissingName             = "name is required"
	errMissingSubscriptionID   = "azureSubscriptionId is required"
	errMissingSubscriptionName = "azureSubscriptionName is required"
	errMissingTenantID         = "azureTenantId is required"

	serviceEndpointTypeAzureRM               = "azurerm"
	serviceEndpointAuthorizationServicePrinc = "ServicePrincipal"
	serviceEndpointAuthorizationWIF          = "WorkloadIdentityFederation"
	serviceEndpointURL                       = "https://management.azure.com/"
	serviceEndpointOwner                     = "library"
	azureCloudEnvironment                    = "AzureCloud"
	scopeLevelSubscription                   = "Subscription"
	creationModeManual                       = "Manual"
	authParamTenantID                        = "tenantid"
	authParamServicePrincipalID              = "serviceprincipalid"
	authParamServicePrincipalKey             = "serviceprincipalkey"
	authParamAuthenticationType              = "authenticationType"
	authParamAuthenticationTypeSPNKey        = "spnKey"
	dataKeySubscriptionID                    = "subscriptionId"
	dataKeySubscriptionName                  = "subscriptionName"
	dataKeyEnvironment                       = "environment"
	dataKeyScopeLevel                        = "scopeLevel"
	dataKeyCreationMode                      = "creationMode"
	dataKeyResourceGroupName                 = "resourceGroupName"
	dataKeyTenantID                          = "tenantid"
	dataKeyServicePrincipalID                = "serviceprincipalid"
)

// annotationClientSecretHash stores a SHA-256 hash of the service
// principal's client secret value that was last pushed to Azure DevOps.
// Azure DevOps never returns the stored secret back on read, so isUpToDate
// can only compare non-secret fields -- this annotation lets Observe
// detect that the *referenced* Secret's value has since been rotated and
// force a resync (otherwise a rotated secret would silently never
// propagate to Azure DevOps).
const annotationClientSecretHash = "serviceendpointazurerm.azuredevops.crossplane.io/client-secret-hash"

// SetupGated adds a controller that reconciles ServiceEndpointAzureRM managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup ServiceEndpointAzureRM controller"))
		}
	}, v1alpha1.ServiceEndpointAzureRMGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles ServiceEndpointAzureRM managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.ServiceEndpointAzureRMGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.ServiceEndpointAzureRM](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.ServiceEndpointAzureRMList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.ServiceEndpointAzureRMList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.ServiceEndpointAzureRMGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.ServiceEndpointAzureRM{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves the managed resource's ProviderConfig and constructs a Service Endpoint API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.ServiceEndpointAzureRM) (managed.TypedExternalClient[*v1alpha1.ServiceEndpointAzureRM], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	seClient, err := newServiceEndpointClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{kube: c.kube, serviceEndpoint: seClient}, nil
}

type external struct {
	kube            client.Client
	serviceEndpoint ServiceEndpointClient
}

func (c *external) Observe(ctx context.Context, cr *v1alpha1.ServiceEndpointAzureRM) (managed.ExternalObservation, error) {
	if err := validateParameters(cr.Spec.ForProvider); err != nil {
		return managed.ExternalObservation{}, err
	}

	name := meta.GetExternalName(cr)
	if name == "" {
		return managed.ExternalObservation{}, nil
	}

	endpointID, err := parseUUID(name, "service endpoint id")
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	endpoint, err := c.getServiceEndpoint(ctx, cr.Spec.ForProvider.ProjectID, endpointID)
	if azuredevops.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetServiceEndpoint)
	}

	cr.Status.AtProvider = observationFromServiceEndpoint(endpoint)
	if cr.Status.AtProvider.IsReady {
		cr.SetConditions(xpv2.Available())
	} else {
		cr.SetConditions(xpv2.Creating())
	}

	upToDate := isUpToDate(cr.Spec.ForProvider, endpoint)
	if upToDate {
		secretUpToDate, err := c.clientSecretUpToDate(ctx, cr)
		if err != nil {
			return managed.ExternalObservation{}, err
		}
		upToDate = secretUpToDate
	}

	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

// clientSecretUpToDate detects drift that isUpToDate never can: Azure
// DevOps never returns the service principal's client secret back on read,
// so the only way to tell that the referenced Secret's value has since
// been rotated is to re-resolve it now and compare its hash against the
// one captured at the last Create/Update.
func (c *external) clientSecretUpToDate(ctx context.Context, cr *v1alpha1.ServiceEndpointAzureRM) (bool, error) {
	secret, err := c.resolveClientSecretValue(ctx, cr.Spec.ForProvider)
	if err != nil {
		return false, err
	}
	if secret == "" {
		// WorkloadIdentityFederation credentials have no secret to track.
		return true, nil
	}
	return hashClientSecret(secret) == cr.GetAnnotations()[annotationClientSecretHash], nil
}

func (c *external) Create(ctx context.Context, cr *v1alpha1.ServiceEndpointAzureRM) (managed.ExternalCreation, error) {
	endpoint, secret, err := c.buildServiceEndpoint(ctx, cr.Spec.ForProvider)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	var created *serviceendpoint.ServiceEndpoint
	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var createErr error
		created, createErr = c.serviceEndpoint.CreateServiceEndpoint(ctx, serviceendpoint.CreateServiceEndpointArgs{Endpoint: endpoint})
		return createErr
	})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateServiceEndpoint)
	}

	cr.Status.AtProvider = observationFromServiceEndpoint(created)
	if cr.Status.AtProvider.ID != "" {
		meta.SetExternalName(cr, cr.Status.AtProvider.ID)
	}
	setClientSecretHashAnnotation(cr, secret)

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, cr *v1alpha1.ServiceEndpointAzureRM) (managed.ExternalUpdate, error) {
	endpointID, err := parseUUID(meta.GetExternalName(cr), "service endpoint id")
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	endpoint, secret, err := c.buildServiceEndpoint(ctx, cr.Spec.ForProvider)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	endpoint.Id = endpointID

	var updated *serviceendpoint.ServiceEndpoint
	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var updateErr error
		updated, updateErr = c.serviceEndpoint.UpdateServiceEndpoint(ctx, serviceendpoint.UpdateServiceEndpointArgs{
			Endpoint:   endpoint,
			EndpointId: endpointID,
		})
		return updateErr
	})
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateServiceEndpoint)
	}

	cr.Status.AtProvider = observationFromServiceEndpoint(updated)
	setClientSecretHashAnnotation(cr, secret)
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, cr *v1alpha1.ServiceEndpointAzureRM) (managed.ExternalDelete, error) {
	name := meta.GetExternalName(cr)
	if name == "" {
		return managed.ExternalDelete{}, nil
	}
	if cr.Spec.ForProvider.ProjectID == "" {
		return managed.ExternalDelete{}, errors.New(errMissingProjectID)
	}

	endpointID, err := parseUUID(name, "service endpoint id")
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	projectIDs := []string{cr.Spec.ForProvider.ProjectID}

	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		return c.serviceEndpoint.DeleteServiceEndpoint(ctx, serviceendpoint.DeleteServiceEndpointArgs{
			EndpointId: endpointID,
			ProjectIds: &projectIDs,
		})
	})
	return managed.ExternalDelete{}, errors.Wrap(resource.Ignore(azuredevops.IsNotFound, err), errDeleteServiceEndpoint)
}

func (c *external) Disconnect(_ context.Context) error {
	return nil
}

func (c *external) getServiceEndpoint(ctx context.Context, projectID string, endpointID *uuid.UUID) (*serviceendpoint.ServiceEndpoint, error) {
	var endpoint *serviceendpoint.ServiceEndpoint
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var getErr error
		endpoint, getErr = c.serviceEndpoint.GetServiceEndpointDetails(ctx, serviceendpoint.GetServiceEndpointDetailsArgs{
			Project:    &projectID,
			EndpointId: endpointID,
		})
		return getErr
	})
	return endpoint, err
}

func (c *external) buildServiceEndpoint(ctx context.Context, p v1alpha1.ServiceEndpointAzureRMParameters) (*serviceendpoint.ServiceEndpoint, string, error) {
	if err := validateParameters(p); err != nil {
		return nil, "", err
	}

	projectID, err := parseUUID(p.ProjectID, "project id")
	if err != nil {
		return nil, "", err
	}

	authorization, secret, err := c.resolveAuthorization(ctx, p)
	if err != nil {
		return nil, "", err
	}

	data := desiredData(p, authClientID(p.Credentials), p.AzureTenantID)
	ready := true
	shared := false
	typ := serviceEndpointTypeAzureRM
	url := serviceEndpointURL
	owner := serviceEndpointOwner
	name := p.Name

	return &serviceendpoint.ServiceEndpoint{
		Authorization: authorization,
		Data:          &data,
		IsReady:       &ready,
		IsShared:      &shared,
		Name:          &name,
		Owner:         &owner,
		ServiceEndpointProjectReferences: &[]serviceendpoint.ServiceEndpointProjectReference{{
			Name: &name,
			ProjectReference: &serviceendpoint.ProjectReference{
				Id: projectID,
			},
		}},
		Type: &typ,
		Url:  &url,
	}, secret, nil
}

func (c *external) resolveAuthorization(ctx context.Context, p v1alpha1.ServiceEndpointAzureRMParameters) (*serviceendpoint.EndpointAuthorization, string, error) {
	scheme, clientID, err := desiredAuthorizationDetails(p.Credentials)
	if err != nil {
		return nil, "", err
	}

	params := map[string]string{
		authParamTenantID:           p.AzureTenantID,
		authParamServicePrincipalID: clientID,
	}

	secret, err := c.resolveClientSecretValue(ctx, p)
	if err != nil {
		return nil, "", err
	}
	if scheme == serviceEndpointAuthorizationServicePrinc {
		params[authParamAuthenticationType] = authParamAuthenticationTypeSPNKey
		params[authParamServicePrincipalKey] = secret
	}

	return &serviceendpoint.EndpointAuthorization{Parameters: &params, Scheme: &scheme}, secret, nil
}

// resolveClientSecretValue resolves the service principal's client secret
// from its referenced Kubernetes Secret. Returns "" (no error) for
// WorkloadIdentityFederation credentials, which have no secret.
func (c *external) resolveClientSecretValue(ctx context.Context, p v1alpha1.ServiceEndpointAzureRMParameters) (string, error) {
	if p.Credentials.ServicePrincipal == nil {
		return "", nil
	}
	secret, err := resource.CommonCredentialExtractor(ctx, xpv2.CredentialsSourceSecret, c.kube, xpv2.CommonCredentialSelectors{
		SecretRef: &p.Credentials.ServicePrincipal.ClientSecretRef,
	})
	if err != nil {
		return "", errors.Wrap(err, errResolveClientSecret)
	}
	return string(secret), nil
}

// hashClientSecret returns a SHA-256 hex digest of secret, for storing in
// annotationClientSecretHash without persisting the plaintext value itself.
func hashClientSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// setClientSecretHashAnnotation records a hash of secret on cr so a future
// Observe can detect if the referenced Secret's value has since changed
// (see annotationClientSecretHash). No-op if secret is empty (i.e.
// WorkloadIdentityFederation credentials, which have nothing to track).
func setClientSecretHashAnnotation(cr *v1alpha1.ServiceEndpointAzureRM, secret string) {
	if secret == "" {
		return
	}
	meta.AddAnnotations(cr, map[string]string{annotationClientSecretHash: hashClientSecret(secret)})
}

func validateParameters(p v1alpha1.ServiceEndpointAzureRMParameters) error {
	if p.Name == "" {
		return errors.New(errMissingName)
	}
	if p.ProjectID == "" {
		return errors.New(errMissingProjectID)
	}
	if p.AzureSubscriptionID == "" {
		return errors.New(errMissingSubscriptionID)
	}
	if p.AzureSubscriptionName == "" {
		return errors.New(errMissingSubscriptionName)
	}
	if p.AzureTenantID == "" {
		return errors.New(errMissingTenantID)
	}
	_, _, err := desiredAuthorizationDetails(p.Credentials)
	return err
}

func desiredAuthorizationDetails(c v1alpha1.ServiceEndpointAzureRMCredentials) (string, string, error) {
	hasSP := c.ServicePrincipal != nil
	hasWIF := c.WorkloadIdentityFederation != nil
	if hasSP == hasWIF {
		return "", "", errors.New(errInvalidCredentials)
	}

	if hasSP {
		return serviceEndpointAuthorizationServicePrinc, c.ServicePrincipal.ClientID, nil
	}
	return serviceEndpointAuthorizationWIF, c.WorkloadIdentityFederation.ClientID, nil
}

func authClientID(c v1alpha1.ServiceEndpointAzureRMCredentials) string {
	if c.ServicePrincipal != nil {
		return c.ServicePrincipal.ClientID
	}
	if c.WorkloadIdentityFederation != nil {
		return c.WorkloadIdentityFederation.ClientID
	}
	return ""
}

func desiredData(p v1alpha1.ServiceEndpointAzureRMParameters, clientID, tenantID string) map[string]string {
	data := map[string]string{
		dataKeySubscriptionID:     p.AzureSubscriptionID,
		dataKeySubscriptionName:   p.AzureSubscriptionName,
		dataKeyEnvironment:        azureCloudEnvironment,
		dataKeyScopeLevel:         scopeLevelSubscription,
		dataKeyCreationMode:       creationModeManual,
		dataKeyTenantID:           tenantID,
		dataKeyServicePrincipalID: clientID,
	}
	if p.ResourceGroup != "" {
		data[dataKeyResourceGroupName] = p.ResourceGroup
	}
	return data
}

func comparableData(p v1alpha1.ServiceEndpointAzureRMParameters) map[string]string {
	data := map[string]string{
		dataKeySubscriptionID:   p.AzureSubscriptionID,
		dataKeySubscriptionName: p.AzureSubscriptionName,
		dataKeyEnvironment:      azureCloudEnvironment,
		dataKeyScopeLevel:       scopeLevelSubscription,
		dataKeyCreationMode:     creationModeManual,
	}
	if p.ResourceGroup != "" {
		data[dataKeyResourceGroupName] = p.ResourceGroup
	}
	return data
}

func observationFromServiceEndpoint(endpoint *serviceendpoint.ServiceEndpoint) v1alpha1.ServiceEndpointAzureRMObservation {
	o := v1alpha1.ServiceEndpointAzureRMObservation{}
	if endpoint == nil {
		return o
	}
	if endpoint.Id != nil {
		o.ID = endpoint.Id.String()
	}
	if endpoint.IsReady != nil {
		o.IsReady = *endpoint.IsReady
	}
	if endpoint.Authorization != nil && endpoint.Authorization.Scheme != nil {
		o.AuthorizationScheme = *endpoint.Authorization.Scheme
	}
	return o
}

// The service principal secret is write-only in Azure DevOps, so up-to-date
// checks intentionally compare only non-secret fields and the selected auth scheme.
//
//nolint:gocyclo // The comparison intentionally checks several discrete endpoint fields.
func isUpToDate(p v1alpha1.ServiceEndpointAzureRMParameters, endpoint *serviceendpoint.ServiceEndpoint) bool {
	if endpoint == nil {
		return false
	}
	if endpoint.Name == nil || *endpoint.Name != p.Name {
		return false
	}
	if endpoint.Authorization == nil || endpoint.Authorization.Scheme == nil {
		return false
	}
	scheme, clientID, err := desiredAuthorizationDetails(p.Credentials)
	if err != nil || !strings.EqualFold(*endpoint.Authorization.Scheme, scheme) {
		return false
	}

	data := map[string]string{}
	if endpoint.Data != nil {
		data = *endpoint.Data
	}
	for key, want := range comparableData(p) {
		if data[key] != want {
			return false
		}
	}
	if p.ResourceGroup == "" && data[dataKeyResourceGroupName] != "" {
		return false
	}

	params := map[string]string{}
	if endpoint.Authorization.Parameters != nil {
		params = *endpoint.Authorization.Parameters
	}
	if got := firstNonEmpty(params[authParamServicePrincipalID], data[dataKeyServicePrincipalID]); got != clientID {
		return false
	}
	if got := firstNonEmpty(params[authParamTenantID], data[dataKeyTenantID]); got != p.AzureTenantID {
		return false
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseUUID(s, subject string) (*uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse "+subject)
	}
	return &id, nil
}
