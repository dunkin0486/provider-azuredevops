// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package branchpolicyworkitemlinking

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/policy"
)

// BranchPolicyWorkItemLinkingClient is the subset of the Azure DevOps Policy
// API client used by this controller to manage work item linking branch
// policies.
type BranchPolicyWorkItemLinkingClient interface {
	GetPolicyConfiguration(ctx context.Context, args policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error)
	CreatePolicyConfiguration(ctx context.Context, args policy.CreatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error)
	UpdatePolicyConfiguration(ctx context.Context, args policy.UpdatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error)
	DeletePolicyConfiguration(ctx context.Context, args policy.DeletePolicyConfigurationArgs) error
}

// newBranchPolicyWorkItemLinkingClient builds the real Azure DevOps Policy SDK
// client used by this controller from a connection resolved via the shared
// internal/clients/azuredevops package.
func newBranchPolicyWorkItemLinkingClient(ctx context.Context, connection *azuredevops.Connection) (BranchPolicyWorkItemLinkingClient, error) {
	return policy.NewClient(ctx, connection)
}
