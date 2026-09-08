// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
)

// EnvironmentClient is the subset of the Azure DevOps TaskAgent API used by
// this controller to manage pipeline environments.
type EnvironmentClient interface {
	AddEnvironment(ctx context.Context, args taskagent.AddEnvironmentArgs) (*taskagent.EnvironmentInstance, error)
	GetEnvironmentById(ctx context.Context, args taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error)
	UpdateEnvironment(ctx context.Context, args taskagent.UpdateEnvironmentArgs) (*taskagent.EnvironmentInstance, error)
	DeleteEnvironment(ctx context.Context, args taskagent.DeleteEnvironmentArgs) error
}

// newEnvironmentClient builds the real Azure DevOps SDK client used by this
// controller from a resolved Azure DevOps SDK connection.
func newEnvironmentClient(ctx context.Context, connection *azuredevops.Connection) (EnvironmentClient, error) {
	return taskagent.NewClient(ctx, connection)
}
