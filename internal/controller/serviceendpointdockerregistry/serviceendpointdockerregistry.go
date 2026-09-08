// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package serviceendpointdockerregistry

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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointdockerregistry/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig                  = "cannot get Azure DevOps client config"
	errNewClient                  = "cannot create new Azure DevOps service endpoint client"
	errGetServiceEndpoint         = "cannot get service endpoint"
	errCreateServiceEndpoint      = "cannot create service endpoint"
	errUpdateServiceEndpoint      = "cannot update service endpoint"
	errDeleteServiceEndpoint      = "cannot delete service endpoint"
	errResolveUsername            = "cannot resolve registry username"
	errResolvePassword            = "cannot resolve registry password"
	errInvalidRegistryType        = "registryType must be one of DockerHub or Others"
	errMissingProjectID           = "projectId is required"
	errMissingName                = "name is required"
	errMissingRegistryURL         = "registryUrl is required"
	errMissingUsernameSecretRef   = "usernameSecretRef is required"
	errMissingPasswordSecretRef   = "passwordSecretRef is required"
	serviceEndpointTypeDockerAuth = "dockerregistry"
	serviceEndpointAuthScheme     = "UsernamePassword"
	serviceEndpointOwner          = "library"
	serviceEndpointURL            = "https://hub.docker.com/"
	registryTypeDockerHub         = "DockerHub"
	registryTypeOthers            = "Others"
	dataKeyRegistryType           = "registrytype"
	authParamRegistry             = "registry"
	authParamUsername             = "username"
	authParamPassword             = "password"
	authParamEmail                = "email"
	annotationConfigHash          = "serviceendpointdockerregistry.azuredevops.crossplane.io/config-hash"
)

// SetupGated adds a controller that reconciles ServiceEndpointDockerRegistry managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup ServiceEndpointDockerRegistry controller"))
		}
	}, v1alpha1.ServiceEndpointDockerRegistryGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles ServiceEndpointDockerRegistry managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.ServiceEndpointDockerRegistryGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.ServiceEndpointDockerRegistry](&connector{kube: mgr.GetClient()}),
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
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.ServiceEndpointDockerRegistryList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.ServiceEndpointDockerRegistryList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.ServiceEndpointDockerRegistryGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.ServiceEndpointDockerRegistry{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves the managed resource's ProviderConfig and constructs a Service Endpoint API client.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.ServiceEndpointDockerRegistry) (managed.TypedExternalClient[*v1alpha1.ServiceEndpointDockerRegistry], error) {
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

func (c *external) Observe(ctx context.Context, cr *v1alpha1.ServiceEndpointDockerRegistry) (managed.ExternalObservation, error) {
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

	upToDate := cr.Status.AtProvider.IsReady && isUpToDate(cr.Spec.ForProvider, endpoint)
	if upToDate {
		configUpToDate, err := c.configUpToDate(ctx, cr, endpoint)
		if err != nil {
			return managed.ExternalObservation{}, err
		}
		upToDate = configUpToDate
	}

	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

func (c *external) configUpToDate(ctx context.Context, cr *v1alpha1.ServiceEndpointDockerRegistry, endpoint *serviceendpoint.ServiceEndpoint) (bool, error) {
	username, password, err := c.resolveCredentials(ctx, cr.Spec.ForProvider)
	if err != nil {
		return false, err
	}
	if !usernameAndEmailUpToDate(endpoint, username, cr.Spec.ForProvider.DockerEmail) {
		return false, nil
	}
	return hashConfig(password) == cr.GetAnnotations()[annotationConfigHash], nil
}

func (c *external) Create(ctx context.Context, cr *v1alpha1.ServiceEndpointDockerRegistry) (managed.ExternalCreation, error) {
	endpoint, hash, err := c.buildServiceEndpoint(ctx, cr.Spec.ForProvider)
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
	setConfigHashAnnotation(cr, hash)

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, cr *v1alpha1.ServiceEndpointDockerRegistry) (managed.ExternalUpdate, error) {
	endpointID, err := parseUUID(meta.GetExternalName(cr), "service endpoint id")
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	endpoint, hash, err := c.buildServiceEndpoint(ctx, cr.Spec.ForProvider)
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
	setConfigHashAnnotation(cr, hash)
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, cr *v1alpha1.ServiceEndpointDockerRegistry) (managed.ExternalDelete, error) {
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

func (c *external) buildServiceEndpoint(ctx context.Context, p v1alpha1.ServiceEndpointDockerRegistryParameters) (*serviceendpoint.ServiceEndpoint, string, error) {
	if err := validateParameters(p); err != nil {
		return nil, "", err
	}

	projectID, err := parseUUID(p.ProjectID, "project id")
	if err != nil {
		return nil, "", err
	}

	registryType, err := desiredRegistryType(p.RegistryType)
	if err != nil {
		return nil, "", err
	}

	authorization, configHash, err := c.resolveAuthorization(ctx, p)
	if err != nil {
		return nil, "", err
	}

	data := map[string]string{
		dataKeyRegistryType: registryType,
	}
	ready := true
	shared := false
	typ := serviceEndpointTypeDockerAuth
	url := desiredServiceEndpointURL()
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
	}, configHash, nil
}

func (c *external) resolveAuthorization(ctx context.Context, p v1alpha1.ServiceEndpointDockerRegistryParameters) (*serviceendpoint.EndpointAuthorization, string, error) {
	username, password, err := c.resolveCredentials(ctx, p)
	if err != nil {
		return nil, "", err
	}

	params := map[string]string{
		authParamRegistry: p.RegistryURL,
		authParamUsername: username,
		authParamPassword: password,
	}
	if p.DockerEmail != "" {
		params[authParamEmail] = p.DockerEmail
	}

	scheme := serviceEndpointAuthScheme
	return &serviceendpoint.EndpointAuthorization{Parameters: &params, Scheme: &scheme}, hashConfig(password), nil
}

func (c *external) resolveCredentials(ctx context.Context, p v1alpha1.ServiceEndpointDockerRegistryParameters) (string, string, error) {
	username, err := c.resolveSecretValue(ctx, p.UsernameSecretRef, errResolveUsername)
	if err != nil {
		return "", "", err
	}
	password, err := c.resolveSecretValue(ctx, p.PasswordSecretRef, errResolvePassword)
	if err != nil {
		return "", "", err
	}
	return username, password, nil
}

func (c *external) resolveSecretValue(ctx context.Context, ref *xpv2.SecretKeySelector, wrap string) (string, error) {
	secret, err := resource.CommonCredentialExtractor(ctx, xpv2.CredentialsSourceSecret, c.kube, xpv2.CommonCredentialSelectors{
		SecretRef: ref,
	})
	if err != nil {
		return "", errors.Wrap(err, wrap)
	}
	return string(secret), nil
}

func hashConfig(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func setConfigHashAnnotation(cr *v1alpha1.ServiceEndpointDockerRegistry, hash string) {
	meta.AddAnnotations(cr, map[string]string{annotationConfigHash: hash})
}

func validateParameters(p v1alpha1.ServiceEndpointDockerRegistryParameters) error {
	if p.Name == "" {
		return errors.New(errMissingName)
	}
	if p.ProjectID == "" {
		return errors.New(errMissingProjectID)
	}
	if p.RegistryURL == "" {
		return errors.New(errMissingRegistryURL)
	}
	if _, err := desiredRegistryType(p.RegistryType); err != nil {
		return err
	}
	return validateRegistryCredentials(p)
}

func desiredRegistryType(s string) (string, error) {
	switch s {
	case "", registryTypeDockerHub:
		return registryTypeDockerHub, nil
	case registryTypeOthers:
		return registryTypeOthers, nil
	default:
		return "", errors.New(errInvalidRegistryType)
	}
}

func validateRegistryCredentials(p v1alpha1.ServiceEndpointDockerRegistryParameters) error {
	if p.UsernameSecretRef == nil {
		return errors.New(errMissingUsernameSecretRef)
	}
	if p.PasswordSecretRef == nil {
		return errors.New(errMissingPasswordSecretRef)
	}
	return nil
}

func observationFromServiceEndpoint(endpoint *serviceendpoint.ServiceEndpoint) v1alpha1.ServiceEndpointDockerRegistryObservation {
	o := v1alpha1.ServiceEndpointDockerRegistryObservation{}
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

func isUpToDate(p v1alpha1.ServiceEndpointDockerRegistryParameters, endpoint *serviceendpoint.ServiceEndpoint) bool {
	if endpoint == nil {
		return false
	}
	registryType, err := desiredRegistryType(p.RegistryType)
	if err != nil {
		return false
	}

	return endpointCoreUpToDate(p, endpoint) &&
		authorizationSchemeUpToDate(endpoint) &&
		registryDataUpToDate(endpoint, registryType) &&
		registryAuthorizationUpToDate(endpoint, p.RegistryURL)
}

func endpointCoreUpToDate(p v1alpha1.ServiceEndpointDockerRegistryParameters, endpoint *serviceendpoint.ServiceEndpoint) bool {
	if _, err := desiredRegistryType(p.RegistryType); err != nil {
		return false
	}
	return endpoint.Name != nil &&
		*endpoint.Name == p.Name &&
		endpoint.Type != nil &&
		strings.EqualFold(*endpoint.Type, serviceEndpointTypeDockerAuth) &&
		endpoint.Url != nil &&
		*endpoint.Url == desiredServiceEndpointURL()
}

func authorizationSchemeUpToDate(endpoint *serviceendpoint.ServiceEndpoint) bool {
	return endpoint.Authorization != nil &&
		endpoint.Authorization.Scheme != nil &&
		strings.EqualFold(*endpoint.Authorization.Scheme, serviceEndpointAuthScheme)
}

func registryDataUpToDate(endpoint *serviceendpoint.ServiceEndpoint, registryType string) bool {
	data := map[string]string{}
	if endpoint.Data != nil {
		data = *endpoint.Data
	}
	return observedRegistryType(data[dataKeyRegistryType]) == registryType
}

func observedRegistryType(s string) string {
	switch {
	case s == "", strings.EqualFold(s, registryTypeOthers):
		return registryTypeOthers
	case strings.EqualFold(s, registryTypeDockerHub):
		return registryTypeDockerHub
	case strings.EqualFold(s, "Docker Registry"):
		return registryTypeOthers
	default:
		return s
	}
}

func desiredServiceEndpointURL() string {
	return serviceEndpointURL
}

func registryAuthorizationUpToDate(endpoint *serviceendpoint.ServiceEndpoint, registryURL string) bool {
	if endpoint.Authorization == nil || endpoint.Authorization.Parameters == nil {
		return false
	}
	return (*endpoint.Authorization.Parameters)[authParamRegistry] == registryURL
}

func usernameAndEmailUpToDate(endpoint *serviceendpoint.ServiceEndpoint, username, email string) bool {
	if endpoint.Authorization == nil || endpoint.Authorization.Parameters == nil {
		return false
	}
	params := *endpoint.Authorization.Parameters
	return params[authParamUsername] == username && params[authParamEmail] == email
}

func parseUUID(s, subject string) (*uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse "+subject)
	}
	return &id, nil
}
