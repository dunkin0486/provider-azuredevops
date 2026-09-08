// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package serviceendpointkubernetes

import (
	"context"
	"strconv"
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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointkubernetes/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
	"github.com/dunkin0486/provider-azuredevops/internal/secrethash"
)

const (
	errGetConfig                        = "cannot get Azure DevOps client config"
	errNewClient                        = "cannot create new Azure DevOps service endpoint client"
	errGetServiceEndpoint               = "cannot get service endpoint"
	errCreateServiceEndpoint            = "cannot create service endpoint"
	errUpdateServiceEndpoint            = "cannot update service endpoint"
	errDeleteServiceEndpoint            = "cannot delete service endpoint"
	errResolveKubeconfig                = "cannot resolve kubeconfig"
	errResolveServiceAccountToken       = "cannot resolve service account token"
	errInvalidAuthorizationType         = "authorizationType must be one of Kubeconfig, ServiceAccount, or AzureSubscription"
	errAzureSubscriptionUnsupported     = "authorizationType AzureSubscription is not yet supported for create/update; use Kubeconfig or ServiceAccount"
	errMissingProjectID                 = "projectId is required"
	errMissingName                      = "name is required"
	errMissingClusterServer             = "clusterServer is required"
	errMissingKubeconfigSecretRef       = "kubeconfigSecretRef is required for authorizationType Kubeconfig"
	errMissingServiceAccountTokenSecret = "serviceAccountTokenSecretRef is required for authorizationType ServiceAccount"
	errMissingClusterCACertificate      = "clusterCACertificate is required for authorizationType ServiceAccount"

	serviceEndpointTypeKubernetes      = "kubernetes"
	serviceEndpointOwner               = "library"
	authorizationTypeKubeconfig        = "Kubeconfig"
	authorizationTypeServiceAccount    = "ServiceAccount"
	authorizationTypeAzureSubscription = "AzureSubscription"
	authSchemeKubernetes               = "Kubernetes"
	authSchemeToken                    = "Token"
	authParamKubeconfig                = "kubeconfig"
	authParamAPIToken                  = "apiToken"
	authParamServiceAccountCert        = "serviceAccountCertificate"
	dataKeyAuthorizationType           = "authorizationType"
	dataKeyAcceptUntrustedCerts        = "acceptUntrustedCerts"
	annotationAuthSecretHash           = "serviceendpointkubernetes.azuredevops.crossplane.io/auth-secret-hash"
)

// SetupGated adds a controller that reconciles ServiceEndpointKubernetes managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup ServiceEndpointKubernetes controller"))
		}
	}, v1alpha1.ServiceEndpointKubernetesGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles ServiceEndpointKubernetes managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.ServiceEndpointKubernetesGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.ServiceEndpointKubernetes](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.ServiceEndpointKubernetesList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.ServiceEndpointKubernetesList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.ServiceEndpointKubernetesGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.ServiceEndpointKubernetes{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves the managed resource's ProviderConfig and constructs a Service Endpoint API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.ServiceEndpointKubernetes) (managed.TypedExternalClient[*v1alpha1.ServiceEndpointKubernetes], error) {
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

func (c *external) Observe(ctx context.Context, cr *v1alpha1.ServiceEndpointKubernetes) (managed.ExternalObservation, error) {
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
	if upToDate && cr.GetDeletionTimestamp().IsZero() {
		secretUpToDate, err := c.authSecretUpToDate(ctx, cr)
		if err != nil {
			return managed.ExternalObservation{}, err
		}
		upToDate = secretUpToDate
	}

	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

// authSecretUpToDate detects drift that isUpToDate never can: Azure DevOps
// never returns stored kubeconfig content or service account tokens back on
// read, so the only way to tell that the referenced Secret's value has since
// been rotated is to re-resolve it now and compare its hash against the one
// captured at the last Create/Update.
func (c *external) authSecretUpToDate(ctx context.Context, cr *v1alpha1.ServiceEndpointKubernetes) (bool, error) {
	secret, err := c.resolveAuthSecretValue(ctx, cr.Spec.ForProvider)
	if err != nil {
		return false, err
	}
	if secret == "" {
		return cr.GetAnnotations()[annotationAuthSecretHash] == "", nil
	}
	return secrethash.Matches(secret, cr.GetAnnotations()[annotationAuthSecretHash]), nil
}

func (c *external) Create(ctx context.Context, cr *v1alpha1.ServiceEndpointKubernetes) (managed.ExternalCreation, error) {
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
	if err := setAuthSecretHashAnnotation(cr, secret); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateServiceEndpoint)
	}

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, cr *v1alpha1.ServiceEndpointKubernetes) (managed.ExternalUpdate, error) {
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
	if err := setAuthSecretHashAnnotation(cr, secret); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateServiceEndpoint)
	}
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, cr *v1alpha1.ServiceEndpointKubernetes) (managed.ExternalDelete, error) {
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

func (c *external) buildServiceEndpoint(ctx context.Context, p v1alpha1.ServiceEndpointKubernetesParameters) (*serviceendpoint.ServiceEndpoint, string, error) {
	if err := validateParameters(p); err != nil {
		return nil, "", err
	}

	projectID, err := parseUUID(p.ProjectID, "project id")
	if err != nil {
		return nil, "", err
	}

	authorization, data, secret, err := c.resolveEndpointConfiguration(ctx, p)
	if err != nil {
		return nil, "", err
	}

	ready := true
	shared := false
	typ := serviceEndpointTypeKubernetes
	url := p.ClusterServer
	owner := serviceEndpointOwner
	name := p.Name

	return &serviceendpoint.ServiceEndpoint{
		Authorization: authorization,
		Data:          data,
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

func (c *external) resolveEndpointConfiguration(ctx context.Context, p v1alpha1.ServiceEndpointKubernetesParameters) (*serviceendpoint.EndpointAuthorization, *map[string]string, string, error) {
	authType, err := desiredAuthorizationType(p.AuthorizationType)
	if err != nil {
		return nil, nil, "", err
	}

	switch authType {
	case authorizationTypeKubeconfig:
		kubeconfig, err := c.resolveKubeconfigSecretValue(ctx, p)
		if err != nil {
			return nil, nil, "", err
		}
		params := map[string]string{authParamKubeconfig: kubeconfig}
		scheme := authSchemeKubernetes
		data := map[string]string{dataKeyAuthorizationType: authorizationTypeKubeconfig}
		// Azure DevOps expects Kubeconfig trust data to live inside the kubeconfig
		// blob itself, so clusterCACertificate is only applied to ServiceAccount
		// auth where the API has a dedicated serviceAccountCertificate field.
		return &serviceendpoint.EndpointAuthorization{Parameters: &params, Scheme: &scheme}, &data, kubeconfig, nil
	case authorizationTypeServiceAccount:
		token, err := c.resolveServiceAccountTokenValue(ctx, p)
		if err != nil {
			return nil, nil, "", err
		}
		params := map[string]string{
			authParamAPIToken:           token,
			authParamServiceAccountCert: p.ClusterCACertificate,
		}
		scheme := authSchemeToken
		data := map[string]string{
			dataKeyAuthorizationType:    authorizationTypeServiceAccount,
			dataKeyAcceptUntrustedCerts: strconv.FormatBool(false),
		}
		return &serviceendpoint.EndpointAuthorization{Parameters: &params, Scheme: &scheme}, &data, token, nil
	case authorizationTypeAzureSubscription:
		// Azure DevOps requires additional AKS metadata here (subscription,
		// tenant, managed cluster resource ID, namespace, and admin mode). This
		// Crossplane API intentionally prioritizes the portable Kubeconfig and
		// ServiceAccount flows; until those Azure-specific inputs are modeled,
		// reject create/update rather than sending a half-valid payload.
		return nil, nil, "", errors.New(errAzureSubscriptionUnsupported)
	default:
		return nil, nil, "", errors.New(errInvalidAuthorizationType)
	}
}

func (c *external) resolveAuthSecretValue(ctx context.Context, p v1alpha1.ServiceEndpointKubernetesParameters) (string, error) {
	authType, err := desiredAuthorizationType(p.AuthorizationType)
	if err != nil {
		return "", err
	}

	switch authType {
	case authorizationTypeKubeconfig:
		return c.resolveKubeconfigSecretValue(ctx, p)
	case authorizationTypeServiceAccount:
		return c.resolveServiceAccountTokenValue(ctx, p)
	default:
		return "", nil
	}
}

func (c *external) resolveKubeconfigSecretValue(ctx context.Context, p v1alpha1.ServiceEndpointKubernetesParameters) (string, error) {
	if p.KubeconfigSecretRef == nil {
		return "", nil
	}
	secret, err := resource.CommonCredentialExtractor(ctx, xpv2.CredentialsSourceSecret, c.kube, xpv2.CommonCredentialSelectors{
		SecretRef: p.KubeconfigSecretRef,
	})
	if err != nil {
		return "", errors.Wrap(err, errResolveKubeconfig)
	}
	return string(secret), nil
}

func (c *external) resolveServiceAccountTokenValue(ctx context.Context, p v1alpha1.ServiceEndpointKubernetesParameters) (string, error) {
	if p.ServiceAccountTokenSecretRef == nil {
		return "", nil
	}
	secret, err := resource.CommonCredentialExtractor(ctx, xpv2.CredentialsSourceSecret, c.kube, xpv2.CommonCredentialSelectors{
		SecretRef: p.ServiceAccountTokenSecretRef,
	})
	if err != nil {
		return "", errors.Wrap(err, errResolveServiceAccountToken)
	}
	return string(secret), nil
}

func setAuthSecretHashAnnotation(cr *v1alpha1.ServiceEndpointKubernetes, secret string) error {
	if secret == "" {
		meta.RemoveAnnotations(cr, annotationAuthSecretHash)
		return nil
	}
	hash, err := secrethash.Hash(secret)
	if err != nil {
		return err
	}
	meta.AddAnnotations(cr, map[string]string{annotationAuthSecretHash: hash})
	return nil
}

func validateParameters(p v1alpha1.ServiceEndpointKubernetesParameters) error {
	if p.Name == "" {
		return errors.New(errMissingName)
	}
	if p.ProjectID == "" {
		return errors.New(errMissingProjectID)
	}
	if p.ClusterServer == "" {
		return errors.New(errMissingClusterServer)
	}
	authType, err := desiredAuthorizationType(p.AuthorizationType)
	if err != nil {
		return err
	}
	if authType == authorizationTypeKubeconfig && p.KubeconfigSecretRef == nil {
		return errors.New(errMissingKubeconfigSecretRef)
	}
	if authType == authorizationTypeServiceAccount {
		if p.ServiceAccountTokenSecretRef == nil {
			return errors.New(errMissingServiceAccountTokenSecret)
		}
		if p.ClusterCACertificate == "" {
			return errors.New(errMissingClusterCACertificate)
		}
	}
	return nil
}

func desiredAuthorizationType(s string) (string, error) {
	switch s {
	case "", authorizationTypeServiceAccount:
		return authorizationTypeServiceAccount, nil
	case authorizationTypeKubeconfig:
		return authorizationTypeKubeconfig, nil
	case authorizationTypeAzureSubscription:
		return authorizationTypeAzureSubscription, nil
	default:
		return "", errors.New(errInvalidAuthorizationType)
	}
}

func observationFromServiceEndpoint(endpoint *serviceendpoint.ServiceEndpoint) v1alpha1.ServiceEndpointKubernetesObservation {
	o := v1alpha1.ServiceEndpointKubernetesObservation{}
	if endpoint == nil {
		return o
	}
	if endpoint.Id != nil {
		o.ID = endpoint.Id.String()
	}
	if endpoint.IsReady != nil {
		o.IsReady = *endpoint.IsReady
	}
	return o
}

// Kubeconfig content and service account tokens are write-only in Azure DevOps,
// so up-to-date checks intentionally compare only non-secret fields here.
//
//nolint:gocyclo // The comparison intentionally checks several discrete endpoint fields.
func isUpToDate(p v1alpha1.ServiceEndpointKubernetesParameters, endpoint *serviceendpoint.ServiceEndpoint) bool {
	if endpoint == nil {
		return false
	}
	if endpoint.Name == nil || *endpoint.Name != p.Name {
		return false
	}
	if endpoint.Type == nil || !strings.EqualFold(*endpoint.Type, serviceEndpointTypeKubernetes) {
		return false
	}
	if endpoint.Url == nil || *endpoint.Url != p.ClusterServer {
		return false
	}
	if endpoint.Authorization == nil || endpoint.Authorization.Scheme == nil {
		return false
	}
	authType, err := desiredAuthorizationType(p.AuthorizationType)
	if err != nil {
		return false
	}
	if !strings.EqualFold(*endpoint.Authorization.Scheme, desiredAuthorizationScheme(authType)) {
		return false
	}
	if endpoint.Data == nil || !strings.EqualFold((*endpoint.Data)[dataKeyAuthorizationType], authType) {
		return false
	}
	if authType != authorizationTypeServiceAccount {
		return true
	}
	if (*endpoint.Data)[dataKeyAcceptUntrustedCerts] != strconv.FormatBool(false) {
		return false
	}
	if endpoint.Authorization.Parameters == nil {
		return p.ClusterCACertificate == ""
	}
	cert := (*endpoint.Authorization.Parameters)[authParamServiceAccountCert]
	return cert == p.ClusterCACertificate
}

func desiredAuthorizationScheme(authType string) string {
	if authType == authorizationTypeServiceAccount {
		return authSchemeToken
	}
	return authSchemeKubernetes
}

func parseUUID(s, subject string) (*uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse "+subject)
	}
	return &id, nil
}
