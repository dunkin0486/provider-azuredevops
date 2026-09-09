// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package agentqueue

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
)

// AgentQueueClient is the subset of the Azure DevOps TaskAgent API used by
// this controller to manage AgentQueue resources.
type AgentQueueClient interface {
	AddAgentQueue(ctx context.Context, args taskagent.AddAgentQueueArgs) (*taskagent.TaskAgentQueue, error)
	GetAgentQueue(ctx context.Context, args taskagent.GetAgentQueueArgs) (*taskagent.TaskAgentQueue, error)
	DeleteAgentQueue(ctx context.Context, args taskagent.DeleteAgentQueueArgs) error
}

// newAgentQueueClient builds the real Azure DevOps SDK client used by this
// controller from a resolved Azure DevOps SDK connection.
func newAgentQueueClient(ctx context.Context, connection *azuredevops.Connection) (AgentQueueClient, error) {
	return taskagent.NewClient(ctx, connection)
}
