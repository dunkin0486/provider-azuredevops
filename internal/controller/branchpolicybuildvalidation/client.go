// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package branchpolicybuildvalidation

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/policy"
)

// BranchPolicyBuildValidationClient is the subset of the Azure DevOps Policy
// API client used by this controller to manage build validation branch policies.
type BranchPolicyBuildValidationClient interface {
	GetPolicyConfiguration(ctx context.Context, args policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error)
	CreatePolicyConfiguration(ctx context.Context, args policy.CreatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error)
	UpdatePolicyConfiguration(ctx context.Context, args policy.UpdatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error)
	DeletePolicyConfiguration(ctx context.Context, args policy.DeletePolicyConfigurationArgs) error
}

// newBranchPolicyBuildValidationClient builds the real Azure DevOps Policy SDK
// client used by this controller from a connection resolved via the shared
// internal/clients/azuredevops package.
func newBranchPolicyBuildValidationClient(ctx context.Context, connection *azuredevops.Connection) (BranchPolicyBuildValidationClient, error) {
	return policy.NewClient(ctx, connection)
}
