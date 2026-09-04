// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package gitrepository

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	adogit "github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

// GitRepositoryClient is the subset of the Azure DevOps Git API client used by
// this controller to manage GitRepository resources.
type GitRepositoryClient interface {
	GetRepository(ctx context.Context, args adogit.GetRepositoryArgs) (*adogit.GitRepository, error)
	CreateRepository(ctx context.Context, args adogit.CreateRepositoryArgs) (*adogit.GitRepository, error)
	UpdateRepository(ctx context.Context, args adogit.UpdateRepositoryArgs) (*adogit.GitRepository, error)
	DeleteRepository(ctx context.Context, args adogit.DeleteRepositoryArgs) error
}

// newGitRepositoryClient builds the real Azure DevOps Git SDK client used by
// this controller from a connection resolved via the shared
// internal/clients/azuredevops package.
func newGitRepositoryClient(ctx context.Context, connection *azuredevops.Connection) (GitRepositoryClient, error) {
	return adogit.NewClient(ctx, connection)
}
