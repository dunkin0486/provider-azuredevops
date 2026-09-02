// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package azuredevops provides shared helpers for constructing Azure DevOps
// API clients from the official Microsoft Go SDK
// (github.com/microsoft/azure-devops-go-api). Resource controllers should use
// GetConfig (which resolves a managed resource's ProviderConfig and returns a
// Config wrapping NewConnection) rather than constructing SDK connections
// directly, so that authentication, error-handling, and retry behavior stay
// consistent across resources. See errors.go, pagination.go, and retry.go
// for the accompanying shared error-translation, pagination, and retry
// helpers.
package azuredevops

import (
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
)

// NewConnection returns an Azure DevOps SDK connection authenticated with a
// personal access token (PAT). organizationURL is the base URL of the Azure
// DevOps organization, e.g. "https://dev.azure.com/myorg". The returned
// Connection can be passed to any of the SDK's per-area NewClient functions
// (e.g. core.NewClient, git.NewClient, build.NewClient) to obtain a typed
// client for that API area.
func NewConnection(organizationURL, personalAccessToken string) *azuredevops.Connection {
	return azuredevops.NewPatConnection(organizationURL, personalAccessToken)
}
