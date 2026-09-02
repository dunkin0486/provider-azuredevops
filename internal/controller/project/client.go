// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/operations"
)

// ProjectClient is the subset of the Azure DevOps Core API's client used by
// this controller to manage Project resources. It's scoped down from the
// full core.Client interface so it can be faked in controller unit tests
// without depending on a live Azure DevOps organization.
type ProjectClient interface {
	GetProject(ctx context.Context, args core.GetProjectArgs) (*core.TeamProject, error)
	GetProcesses(ctx context.Context, args core.GetProcessesArgs) (*[]core.Process, error)
	QueueCreateProject(ctx context.Context, args core.QueueCreateProjectArgs) (*operations.OperationReference, error)
	UpdateProject(ctx context.Context, args core.UpdateProjectArgs) (*operations.OperationReference, error)
	QueueDeleteProject(ctx context.Context, args core.QueueDeleteProjectArgs) (*operations.OperationReference, error)
}

// OperationsClient is the subset of the Azure DevOps Operations API's client
// used to poll the status of the async operations returned by
// QueueCreateProject, UpdateProject, and QueueDeleteProject.
type OperationsClient interface {
	GetOperation(ctx context.Context, args operations.GetOperationArgs) (*operations.Operation, error)
}

// newProjectClient builds the real Azure DevOps SDK clients used by this
// controller from a connection resolved via the shared
// internal/clients/azuredevops package.
func newProjectClient(ctx context.Context, connection *azuredevops.Connection) (ProjectClient, OperationsClient, error) {
	c, err := core.NewClient(ctx, connection)
	if err != nil {
		return nil, nil, err
	}
	return c, operations.NewClient(ctx, connection), nil
}
