// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package agentpool

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
)

// AgentPoolClient is the subset of the Azure DevOps TaskAgent API used by
// this controller to manage AgentPool resources.
type AgentPoolClient interface {
	AddAgentPool(ctx context.Context, args taskagent.AddAgentPoolArgs) (*taskagent.TaskAgentPool, error)
	GetAgentPool(ctx context.Context, args taskagent.GetAgentPoolArgs) (*taskagent.TaskAgentPool, error)
	UpdateAgentPool(ctx context.Context, args taskagent.UpdateAgentPoolArgs) (*taskagent.TaskAgentPool, error)
	DeleteAgentPool(ctx context.Context, args taskagent.DeleteAgentPoolArgs) error
}

// newAgentPoolClient builds the real Azure DevOps SDK client used by this
// controller from a resolved Azure DevOps SDK connection.
func newAgentPoolClient(ctx context.Context, connection *azuredevops.Connection) (AgentPoolClient, error) {
	return taskagent.NewClient(ctx, connection)
}
