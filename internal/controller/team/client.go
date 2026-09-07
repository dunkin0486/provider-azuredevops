// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package team

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
)

// TeamClient is the subset of the Azure DevOps Core API client used by this
// controller to manage Team resources.
type TeamClient interface {
	GetTeam(ctx context.Context, args core.GetTeamArgs) (*core.WebApiTeam, error)
	CreateTeam(ctx context.Context, args core.CreateTeamArgs) (*core.WebApiTeam, error)
	UpdateTeam(ctx context.Context, args core.UpdateTeamArgs) (*core.WebApiTeam, error)
	DeleteTeam(ctx context.Context, args core.DeleteTeamArgs) error
}

// newTeamClient builds the real Azure DevOps Core SDK client used by this
// controller from a connection resolved via the shared
// internal/clients/azuredevops package.
func newTeamClient(ctx context.Context, connection *azuredevops.Connection) (TeamClient, error) {
	return core.NewClient(ctx, connection)
}
