// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the agentqueue
// package's AgentQueueClient interface for unit tests.
package fake

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
)

// AgentQueueClient is a fake implementation of the agentqueue package's
// AgentQueueClient interface, backed by function fields so tests can stub
// only the methods they exercise.
type AgentQueueClient struct {
	AddAgentQueueFn    func(ctx context.Context, args taskagent.AddAgentQueueArgs) (*taskagent.TaskAgentQueue, error)
	GetAgentQueueFn    func(ctx context.Context, args taskagent.GetAgentQueueArgs) (*taskagent.TaskAgentQueue, error)
	DeleteAgentQueueFn func(ctx context.Context, args taskagent.DeleteAgentQueueArgs) error
}

// AddAgentQueue calls AddAgentQueueFn.
func (f *AgentQueueClient) AddAgentQueue(ctx context.Context, args taskagent.AddAgentQueueArgs) (*taskagent.TaskAgentQueue, error) {
	return f.AddAgentQueueFn(ctx, args)
}

// GetAgentQueue calls GetAgentQueueFn.
func (f *AgentQueueClient) GetAgentQueue(ctx context.Context, args taskagent.GetAgentQueueArgs) (*taskagent.TaskAgentQueue, error) {
	return f.GetAgentQueueFn(ctx, args)
}

// DeleteAgentQueue calls DeleteAgentQueueFn.
func (f *AgentQueueClient) DeleteAgentQueue(ctx context.Context, args taskagent.DeleteAgentQueueArgs) error {
	return f.DeleteAgentQueueFn(ctx, args)
}
