// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the
// branchpolicyminreviewers package's BranchPolicyMinReviewersClient interface
// for unit tests.
package fake

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/policy"
)

// BranchPolicyMinReviewersClient is a fake implementation of the
// branchpolicyminreviewers package's BranchPolicyMinReviewersClient interface,
// backed by function fields so tests can stub only the methods they exercise.
type BranchPolicyMinReviewersClient struct {
	GetPolicyConfigurationFn    func(ctx context.Context, args policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error)
	CreatePolicyConfigurationFn func(ctx context.Context, args policy.CreatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error)
	UpdatePolicyConfigurationFn func(ctx context.Context, args policy.UpdatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error)
	DeletePolicyConfigurationFn func(ctx context.Context, args policy.DeletePolicyConfigurationArgs) error
}

// GetPolicyConfiguration calls GetPolicyConfigurationFn.
func (f *BranchPolicyMinReviewersClient) GetPolicyConfiguration(ctx context.Context, args policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
	return f.GetPolicyConfigurationFn(ctx, args)
}

// CreatePolicyConfiguration calls CreatePolicyConfigurationFn.
func (f *BranchPolicyMinReviewersClient) CreatePolicyConfiguration(ctx context.Context, args policy.CreatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
	return f.CreatePolicyConfigurationFn(ctx, args)
}

// UpdatePolicyConfiguration calls UpdatePolicyConfigurationFn.
func (f *BranchPolicyMinReviewersClient) UpdatePolicyConfiguration(ctx context.Context, args policy.UpdatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
	return f.UpdatePolicyConfigurationFn(ctx, args)
}

// DeletePolicyConfiguration calls DeletePolicyConfigurationFn.
func (f *BranchPolicyMinReviewersClient) DeletePolicyConfiguration(ctx context.Context, args policy.DeletePolicyConfigurationArgs) error {
	return f.DeletePolicyConfigurationFn(ctx, args)
}
