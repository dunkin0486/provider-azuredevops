// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package serviceendpointgithub

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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointgithub/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig             = "cannot get Azure DevOps client config"
	errNewClient             = "cannot create new Azure DevOps service endpoint client"
	errGetServiceEndpoint    = "cannot get service endpoint"
	errCreateServiceEndpoint = "cannot create service endpoint"
	errUpdateServiceEndpoint = "cannot update service endpoint"
	errDeleteServiceEndpoint = "cannot delete service endpoint"
	errResolveToken          = "cannot resolve GitHub token"
	errInvalidAuthScheme     = "authScheme must be one of PersonalAccessToken, OAuth, or InstallationToken"
	errMissingProjectID      = "projectId is required"
	errMissingName           = "name is required"
	errMissingTokenSecretRef = "tokenSecretRef is required for the selected authScheme"

	serviceEndpointTypeGitHub              = "github"
	serviceEndpointAuthorizationPAT        = "PersonalAccessToken"
	serviceEndpointAuthorizationOAuth      = "OAuth"
	serviceEndpointAuthorizationInstallTok = "InstallationToken"
	serviceEndpointURL                     = "https://github.com/"
	serviceEndpointOwner                   = "library"
	authParamAccessToken                   = "accessToken"
	annotationTokenHash                    = "serviceendpointgithub.azuredevops.crossplane.io/token-hash"
)

// SetupGated adds a controller that reconciles ServiceEndpointGitHub managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup ServiceEndpointGitHub controller"))
		}
	}, v1alpha1.ServiceEndpointGitHubGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles ServiceEndpointGitHub managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.ServiceEndpointGitHubGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.ServiceEndpointGitHub](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.ServiceEndpointGitHubList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.ServiceEndpointGitHubList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.ServiceEndpointGitHubGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.ServiceEndpointGitHub{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves the managed resource's ProviderConfig and constructs a Service Endpoint API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.ServiceEndpointGitHub) (managed.TypedExternalClient[*v1alpha1.ServiceEndpointGitHub], error) {
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

func (c *external) Observe(ctx context.Context, cr *v1alpha1.ServiceEndpointGitHub) (managed.ExternalObservation, error) {
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
		tokenUpToDate, err := c.tokenUpToDate(ctx, cr)
		if err != nil {
			return managed.ExternalObservation{}, err
		}
		upToDate = tokenUpToDate
	}

	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

// tokenUpToDate detects drift that isUpToDate never can: Azure DevOps never
// returns stored endpoint tokens back on read, so the only way to tell that the
// referenced Secret's value has since been rotated is to re-resolve it now and
// compare its hash against the one captured at the last Create/Update.
func (c *external) tokenUpToDate(ctx context.Context, cr *v1alpha1.ServiceEndpointGitHub) (bool, error) {
	token, err := c.resolveTokenSecretValue(ctx, cr.Spec.ForProvider)
	if err != nil {
		return false, err
	}
	if token == "" {
		return cr.GetAnnotations()[annotationTokenHash] == "", nil
	}
	return hashToken(token) == cr.GetAnnotations()[annotationTokenHash], nil
}

func (c *external) Create(ctx context.Context, cr *v1alpha1.ServiceEndpointGitHub) (managed.ExternalCreation, error) {
	endpoint, token, err := c.buildServiceEndpoint(ctx, cr.Spec.ForProvider)
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
	setTokenHashAnnotation(cr, token)

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, cr *v1alpha1.ServiceEndpointGitHub) (managed.ExternalUpdate, error) {
	endpointID, err := parseUUID(meta.GetExternalName(cr), "service endpoint id")
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	endpoint, token, err := c.buildServiceEndpoint(ctx, cr.Spec.ForProvider)
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
	setTokenHashAnnotation(cr, token)
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, cr *v1alpha1.ServiceEndpointGitHub) (managed.ExternalDelete, error) {
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

func (c *external) buildServiceEndpoint(ctx context.Context, p v1alpha1.ServiceEndpointGitHubParameters) (*serviceendpoint.ServiceEndpoint, string, error) {
	if err := validateParameters(p); err != nil {
		return nil, "", err
	}

	projectID, err := parseUUID(p.ProjectID, "project id")
	if err != nil {
		return nil, "", err
	}

	authorization, token, err := c.resolveAuthorization(ctx, p)
	if err != nil {
		return nil, "", err
	}

	data := map[string]string{}
	ready := true
	shared := false
	typ := serviceEndpointTypeGitHub
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
	}, token, nil
}

func (c *external) resolveAuthorization(ctx context.Context, p v1alpha1.ServiceEndpointGitHubParameters) (*serviceendpoint.EndpointAuthorization, string, error) {
	scheme, err := desiredAuthorizationScheme(p.AuthScheme)
	if err != nil {
		return nil, "", err
	}

	token, err := c.resolveTokenSecretValue(ctx, p)
	if err != nil {
		return nil, "", err
	}

	params := map[string]string{}
	if token != "" {
		params[authParamAccessToken] = token
	}

	// Azure DevOps' GitHub OAuth scheme can rely on server-side OAuth
	// configuration, so tokenSecretRef is optional there. When a token is
	// supplied anyway, pass it through as the generic accessToken parameter used
	// by other GitHub token-backed flows.
	return &serviceendpoint.EndpointAuthorization{Parameters: &params, Scheme: &scheme}, token, nil
}

// resolveTokenSecretValue resolves the endpoint token from its referenced
// Kubernetes Secret. OAuth service connections may omit tokenSecretRef when
// Azure DevOps manages the OAuth handshake separately.
func (c *external) resolveTokenSecretValue(ctx context.Context, p v1alpha1.ServiceEndpointGitHubParameters) (string, error) {
	if p.TokenSecretRef == nil {
		return "", nil
	}
	secret, err := resource.CommonCredentialExtractor(ctx, xpv2.CredentialsSourceSecret, c.kube, xpv2.CommonCredentialSelectors{
		SecretRef: p.TokenSecretRef,
	})
	if err != nil {
		return "", errors.Wrap(err, errResolveToken)
	}
	return string(secret), nil
}

// hashToken returns a SHA-256 hex digest of token, for storing in
// annotationTokenHash without persisting the plaintext value itself.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// setTokenHashAnnotation records a hash of token on cr so a future Observe can
// detect if the referenced Secret's value has since changed (see
// annotationTokenHash). No-op if token is empty.
func setTokenHashAnnotation(cr *v1alpha1.ServiceEndpointGitHub, token string) {
	if token == "" {
		meta.RemoveAnnotations(cr, annotationTokenHash)
		return
	}
	meta.AddAnnotations(cr, map[string]string{annotationTokenHash: hashToken(token)})
}

func validateParameters(p v1alpha1.ServiceEndpointGitHubParameters) error {
	if p.Name == "" {
		return errors.New(errMissingName)
	}
	if p.ProjectID == "" {
		return errors.New(errMissingProjectID)
	}
	scheme, err := desiredAuthorizationScheme(p.AuthScheme)
	if err != nil {
		return err
	}
	if requiresTokenSecret(scheme) && p.TokenSecretRef == nil {
		return errors.New(errMissingTokenSecretRef)
	}
	return nil
}

func desiredAuthorizationScheme(s string) (string, error) {
	switch s {
	case "", serviceEndpointAuthorizationPAT:
		return serviceEndpointAuthorizationPAT, nil
	case serviceEndpointAuthorizationOAuth:
		return serviceEndpointAuthorizationOAuth, nil
	case serviceEndpointAuthorizationInstallTok:
		return serviceEndpointAuthorizationInstallTok, nil
	default:
		return "", errors.New(errInvalidAuthScheme)
	}
}

func requiresTokenSecret(scheme string) bool {
	return scheme == serviceEndpointAuthorizationPAT || scheme == serviceEndpointAuthorizationInstallTok
}

func observationFromServiceEndpoint(endpoint *serviceendpoint.ServiceEndpoint) v1alpha1.ServiceEndpointGitHubObservation {
	o := v1alpha1.ServiceEndpointGitHubObservation{}
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

// GitHub access tokens are write-only in Azure DevOps, so up-to-date checks
// intentionally compare only non-secret fields and the selected auth scheme.
//
//nolint:gocyclo // The comparison intentionally checks several discrete endpoint fields.
func isUpToDate(p v1alpha1.ServiceEndpointGitHubParameters, endpoint *serviceendpoint.ServiceEndpoint) bool {
	if endpoint == nil {
		return false
	}
	if endpoint.Name == nil || *endpoint.Name != p.Name {
		return false
	}
	if endpoint.Type == nil || !strings.EqualFold(*endpoint.Type, serviceEndpointTypeGitHub) {
		return false
	}
	if endpoint.Url == nil || *endpoint.Url != serviceEndpointURL {
		return false
	}
	if endpoint.Authorization == nil || endpoint.Authorization.Scheme == nil {
		return false
	}
	scheme, err := desiredAuthorizationScheme(p.AuthScheme)
	return err == nil && strings.EqualFold(*endpoint.Authorization.Scheme, scheme)
}

func parseUUID(s, subject string) (*uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse "+subject)
	}
	return &id, nil
}
