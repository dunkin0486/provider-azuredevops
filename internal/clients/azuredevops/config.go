// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package azuredevops

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dunkin0486/provider-azuredevops/apis/v1alpha1"
)

// Error strings for GetConfig and its helpers.
const (
	errNoProviderConfigRef = "managed resource does not reference a ProviderConfig"
	errGetProviderConfig   = "cannot get referenced ProviderConfig"
	errGetClusterConfig    = "cannot get referenced ClusterProviderConfig"
	errTrackUsage          = "cannot track ProviderConfig usage"
	errExtractCredentials  = "cannot extract credentials"
	errUnknownProviderKind = "unsupported ProviderConfig reference kind %q"
)

// A Config holds everything a resource controller needs to talk to an Azure
// DevOps organization on behalf of a managed resource: the organization URL
// and a personal access token (PAT) resolved from the referenced
// ProviderConfig (or ClusterProviderConfig).
type Config struct {
	// OrganizationURL is the base URL of the Azure DevOps organization,
	// e.g. "https://dev.azure.com/myorg".
	OrganizationURL string

	// Token is the personal access token (PAT) used to authenticate to the
	// organization above.
	Token string
}

// Connection returns an Azure DevOps SDK connection for this Config. Pass it
// to any of the SDK's per-area NewClient functions (e.g. core.NewClient,
// git.NewClient) to obtain a typed client for a given API area.
func (c *Config) Connection() *azuredevops.Connection {
	return NewConnection(c.OrganizationURL, c.Token)
}

// GetConfig resolves the ProviderConfig (or ClusterProviderConfig) referenced
// by mg, tracks its usage, and extracts its credentials, returning a Config
// ready to build an Azure DevOps SDK connection from. Resource controllers
// should call this from their ExternalConnecter.Connect implementation
// instead of duplicating ProviderConfig resolution and credential extraction
// logic.
func GetConfig(ctx context.Context, kube client.Client, mg resource.ModernManaged) (*Config, error) {
	ref := mg.GetProviderConfigReference()
	if ref == nil {
		return nil, errors.New(errNoProviderConfigRef)
	}

	usage := resource.NewProviderConfigUsageTracker(kube, &v1alpha1.ProviderConfigUsage{})
	if err := usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}

	switch ref.Kind {
	case v1alpha1.ClusterProviderConfigKind:
		pc := &v1alpha1.ClusterProviderConfig{}
		if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name}, pc); err != nil {
			return nil, errors.Wrap(err, errGetClusterConfig)
		}
		return configFromSpec(ctx, kube, pc.Spec)
	case v1alpha1.ProviderConfigKind:
		pc := &v1alpha1.ProviderConfig{}
		if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: mg.GetNamespace()}, pc); err != nil {
			return nil, errors.Wrap(err, errGetProviderConfig)
		}
		return configFromSpec(ctx, kube, pc.Spec)
	default:
		return nil, errors.Errorf(errUnknownProviderKind, ref.Kind)
	}
}

func configFromSpec(ctx context.Context, kube client.Client, spec v1alpha1.ProviderConfigSpec) (*Config, error) {
	data, err := resource.CommonCredentialExtractor(ctx, spec.Credentials.Source, kube, spec.Credentials.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errExtractCredentials)
	}

	return &Config{
		OrganizationURL: spec.OrganizationURL,
		Token:           string(data),
	}, nil
}
