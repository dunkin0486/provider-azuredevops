// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the agentpool
// package's AgentPoolClient interface for unit tests.
package fake

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
)

// AgentPoolClient is a fake implementation of the agentpool package's
// AgentPoolClient interface, backed by function fields so tests can stub only
// the methods they exercise.
type AgentPoolClient struct {
	AddAgentPoolFn    func(ctx context.Context, args taskagent.AddAgentPoolArgs) (*taskagent.TaskAgentPool, error)
	GetAgentPoolFn    func(ctx context.Context, args taskagent.GetAgentPoolArgs) (*taskagent.TaskAgentPool, error)
	UpdateAgentPoolFn func(ctx context.Context, args taskagent.UpdateAgentPoolArgs) (*taskagent.TaskAgentPool, error)
	DeleteAgentPoolFn func(ctx context.Context, args taskagent.DeleteAgentPoolArgs) error
}

// AddAgentPool calls AddAgentPoolFn.
func (f *AgentPoolClient) AddAgentPool(ctx context.Context, args taskagent.AddAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
	return f.AddAgentPoolFn(ctx, args)
}

// GetAgentPool calls GetAgentPoolFn.
func (f *AgentPoolClient) GetAgentPool(ctx context.Context, args taskagent.GetAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
	return f.GetAgentPoolFn(ctx, args)
}

// UpdateAgentPool calls UpdateAgentPoolFn.
func (f *AgentPoolClient) UpdateAgentPool(ctx context.Context, args taskagent.UpdateAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
	return f.UpdateAgentPoolFn(ctx, args)
}

// DeleteAgentPool calls DeleteAgentPoolFn.
func (f *AgentPoolClient) DeleteAgentPool(ctx context.Context, args taskagent.DeleteAgentPoolArgs) error {
	return f.DeleteAgentPoolFn(ctx, args)
}
