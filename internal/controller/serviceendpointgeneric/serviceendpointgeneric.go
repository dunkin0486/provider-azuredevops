// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package serviceendpointgeneric

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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointgeneric/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig                       = "cannot get Azure DevOps client config"
	errNewClient                       = "cannot create new Azure DevOps service endpoint client"
	errGetServiceEndpoint              = "cannot get service endpoint"
	errCreateServiceEndpoint           = "cannot create service endpoint"
	errUpdateServiceEndpoint           = "cannot update service endpoint"
	errDeleteServiceEndpoint           = "cannot delete service endpoint"
	errResolvePasswordSecret           = "cannot resolve password secret"
	errInvalidAuthorizationScheme      = "authorizationScheme must be one of UsernamePassword, Token, or None"
	errMissingProjectID                = "projectId is required"
	errMissingName                     = "name is required"
	errMissingServerURL                = "serverUrl is required"
	errMissingAuthorizationScheme      = "authorizationScheme is required"
	errMissingPasswordSecretRef        = "passwordSecretRef is required when authorizationScheme is UsernamePassword or Token"
	errUsernameOnlyForUsernamePassword = "username may only be set when authorizationScheme is UsernamePassword"
	errPasswordSecretRefNotAllowed     = "passwordSecretRef may not be set when authorizationScheme is None"

	serviceEndpointTypeGeneric                   = "generic"
	serviceEndpointAuthorizationUsernamePassword = "UsernamePassword"
	serviceEndpointAuthorizationToken            = "Token"
	serviceEndpointAuthorizationNone             = "None"
	serviceEndpointOwner                         = "library"
	authParamUsername                            = "username"
	authParamPassword                            = "password"
	// Azure DevOps' generic service endpoint type metadata documents
	// "apitoken" for Token auth, so use that canonical key rather than
	// undocumented alternatives like "accesstoken".
	authParamAPIToken = "apitoken"
)

// annotationPasswordHash stores a SHA-256 hash of the password or token value
// that was last pushed to Azure DevOps. Azure DevOps never returns stored
// secrets back on read, so isUpToDate can only compare non-secret fields --
// this annotation lets Observe detect that the *referenced* Secret's value has
// since been rotated and force a resync (otherwise a rotated secret would
// silently never propagate to Azure DevOps).
const annotationPasswordHash = "serviceendpointgeneric.azuredevops.crossplane.io/password-hash"

// SetupGated adds a controller that reconciles ServiceEndpointGeneric managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup ServiceEndpointGeneric controller"))
		}
	}, v1alpha1.ServiceEndpointGenericGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles ServiceEndpointGeneric managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.ServiceEndpointGenericGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.ServiceEndpointGeneric](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.ServiceEndpointGenericList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.ServiceEndpointGenericList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.ServiceEndpointGenericGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.ServiceEndpointGeneric{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves the managed resource's ProviderConfig and constructs a Service Endpoint API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.ServiceEndpointGeneric) (managed.TypedExternalClient[*v1alpha1.ServiceEndpointGeneric], error) {
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

func (c *external) Observe(ctx context.Context, cr *v1alpha1.ServiceEndpointGeneric) (managed.ExternalObservation, error) {
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
		passwordUpToDate, err := c.passwordUpToDate(ctx, cr)
		if err != nil {
			return managed.ExternalObservation{}, err
		}
		upToDate = passwordUpToDate
	}

	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

// passwordUpToDate detects drift that isUpToDate never can: Azure DevOps never
// returns the password or token back on read, so the only way to tell that the
// referenced Secret's value has since been rotated is to re-resolve it now and
// compare its hash against the one captured at the last Create/Update.
func (c *external) passwordUpToDate(ctx context.Context, cr *v1alpha1.ServiceEndpointGeneric) (bool, error) {
	secret, err := c.resolvePasswordValue(ctx, cr.Spec.ForProvider)
	if err != nil {
		return false, err
	}
	if secret == "" {
		return true, nil
	}
	return hashPassword(secret) == cr.GetAnnotations()[annotationPasswordHash], nil
}

func (c *external) Create(ctx context.Context, cr *v1alpha1.ServiceEndpointGeneric) (managed.ExternalCreation, error) {
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
	setPasswordHashAnnotation(cr, secret)

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, cr *v1alpha1.ServiceEndpointGeneric) (managed.ExternalUpdate, error) {
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
	setPasswordHashAnnotation(cr, secret)
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, cr *v1alpha1.ServiceEndpointGeneric) (managed.ExternalDelete, error) {
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

func (c *external) buildServiceEndpoint(ctx context.Context, p v1alpha1.ServiceEndpointGenericParameters) (*serviceendpoint.ServiceEndpoint, string, error) {
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

	data := map[string]string{}
	ready := true
	shared := false
	typ := serviceEndpointTypeGeneric
	url := p.ServerURL
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

func (c *external) resolveAuthorization(ctx context.Context, p v1alpha1.ServiceEndpointGenericParameters) (*serviceendpoint.EndpointAuthorization, string, error) {
	scheme, err := desiredAuthorizationScheme(p)
	if err != nil {
		return nil, "", err
	}

	params := map[string]string{}
	secret, err := c.resolvePasswordValue(ctx, p)
	if err != nil {
		return nil, "", err
	}

	switch scheme {
	case serviceEndpointAuthorizationUsernamePassword:
		if p.Username != "" {
			params[authParamUsername] = p.Username
		}
		params[authParamPassword] = secret
	case serviceEndpointAuthorizationToken:
		params[authParamAPIToken] = secret
	case serviceEndpointAuthorizationNone:
	}

	return &serviceendpoint.EndpointAuthorization{Parameters: &params, Scheme: &scheme}, secret, nil
}

// resolvePasswordValue resolves the password or token from its referenced
// Kubernetes Secret. Returns "" (no error) for authorizationScheme=None,
// which has no secret to track.
func (c *external) resolvePasswordValue(ctx context.Context, p v1alpha1.ServiceEndpointGenericParameters) (string, error) {
	if p.PasswordSecretRef == nil {
		return "", nil
	}
	secret, err := resource.CommonCredentialExtractor(ctx, xpv2.CredentialsSourceSecret, c.kube, xpv2.CommonCredentialSelectors{
		SecretRef: p.PasswordSecretRef,
	})
	if err != nil {
		return "", errors.Wrap(err, errResolvePasswordSecret)
	}
	return string(secret), nil
}

// hashPassword returns a SHA-256 hex digest of secret, for storing in
// annotationPasswordHash without persisting the plaintext value itself.
func hashPassword(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// setPasswordHashAnnotation records a hash of secret on cr so a future Observe
// can detect if the referenced Secret's value has since changed (see
// annotationPasswordHash). No-op if secret is empty (i.e. None auth, which has
// nothing to track).
func setPasswordHashAnnotation(cr *v1alpha1.ServiceEndpointGeneric, secret string) {
	if secret == "" {
		return
	}
	meta.AddAnnotations(cr, map[string]string{annotationPasswordHash: hashPassword(secret)})
}

func validateParameters(p v1alpha1.ServiceEndpointGenericParameters) error {
	if p.Name == "" {
		return errors.New(errMissingName)
	}
	if p.ProjectID == "" {
		return errors.New(errMissingProjectID)
	}
	if p.ServerURL == "" {
		return errors.New(errMissingServerURL)
	}
	if p.AuthorizationScheme == "" {
		return errors.New(errMissingAuthorizationScheme)
	}
	_, err := desiredAuthorizationScheme(p)
	return err
}

func desiredAuthorizationScheme(p v1alpha1.ServiceEndpointGenericParameters) (string, error) {
	switch p.AuthorizationScheme {
	case serviceEndpointAuthorizationUsernamePassword:
		if p.PasswordSecretRef == nil {
			return "", errors.New(errMissingPasswordSecretRef)
		}
		return serviceEndpointAuthorizationUsernamePassword, nil
	case serviceEndpointAuthorizationToken:
		if p.Username != "" {
			return "", errors.New(errUsernameOnlyForUsernamePassword)
		}
		if p.PasswordSecretRef == nil {
			return "", errors.New(errMissingPasswordSecretRef)
		}
		return serviceEndpointAuthorizationToken, nil
	case serviceEndpointAuthorizationNone:
		if p.Username != "" {
			return "", errors.New(errUsernameOnlyForUsernamePassword)
		}
		if p.PasswordSecretRef != nil {
			return "", errors.New(errPasswordSecretRefNotAllowed)
		}
		return serviceEndpointAuthorizationNone, nil
	default:
		return "", errors.New(errInvalidAuthorizationScheme)
	}
}

func observationFromServiceEndpoint(endpoint *serviceendpoint.ServiceEndpoint) v1alpha1.ServiceEndpointGenericObservation {
	o := v1alpha1.ServiceEndpointGenericObservation{}
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

func isUpToDate(p v1alpha1.ServiceEndpointGenericParameters, endpoint *serviceendpoint.ServiceEndpoint) bool {
	if endpoint == nil {
		return false
	}
	scheme, err := desiredAuthorizationScheme(p)
	if err != nil {
		return false
	}
	return endpointCoreUpToDate(p, endpoint) &&
		authorizationSchemeUpToDate(endpoint, scheme) &&
		authorizationParametersUpToDate(p, endpoint, scheme)
}

func endpointCoreUpToDate(p v1alpha1.ServiceEndpointGenericParameters, endpoint *serviceendpoint.ServiceEndpoint) bool {
	return endpoint.Name != nil &&
		*endpoint.Name == p.Name &&
		endpoint.Type != nil &&
		strings.EqualFold(*endpoint.Type, serviceEndpointTypeGeneric) &&
		endpoint.Url != nil &&
		*endpoint.Url == p.ServerURL
}

func authorizationSchemeUpToDate(endpoint *serviceendpoint.ServiceEndpoint, scheme string) bool {
	return endpoint.Authorization != nil &&
		endpoint.Authorization.Scheme != nil &&
		strings.EqualFold(*endpoint.Authorization.Scheme, scheme)
}

func authorizationParametersUpToDate(p v1alpha1.ServiceEndpointGenericParameters, endpoint *serviceendpoint.ServiceEndpoint, scheme string) bool {
	params := map[string]string{}
	if endpoint.Authorization != nil && endpoint.Authorization.Parameters != nil {
		params = *endpoint.Authorization.Parameters
	}

	switch scheme {
	case serviceEndpointAuthorizationUsernamePassword:
		return params[authParamUsername] == p.Username
	case serviceEndpointAuthorizationToken, serviceEndpointAuthorizationNone:
		return params[authParamUsername] == ""
	default:
		return false
	}
}

func parseUUID(s, subject string) (*uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse "+subject)
	}
	return &id, nil
}
