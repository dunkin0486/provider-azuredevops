// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the
// groupmembership package's GroupMembershipClient interface, for use in
// controller unit tests. It lives in its own subpackage to avoid an import
// cycle with the groupmembership package's tests.
package fake

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/graph"
)

// GroupMembershipClient is a fake implementation of the groupmembership
// package's GroupMembershipClient interface, backed by function fields so
// individual test cases can stub out only the methods they exercise.
type GroupMembershipClient struct {
	AddMembershipFn      func(ctx context.Context, args graph.AddMembershipArgs) (*graph.GraphMembership, error)
	GetMembershipFn      func(ctx context.Context, args graph.GetMembershipArgs) (*graph.GraphMembership, error)
	GetMembershipStateFn func(ctx context.Context, args graph.GetMembershipStateArgs) (*graph.GraphMembershipState, error)
	RemoveMembershipFn   func(ctx context.Context, args graph.RemoveMembershipArgs) error
}

// AddMembership calls AddMembershipFn.
func (f *GroupMembershipClient) AddMembership(ctx context.Context, args graph.AddMembershipArgs) (*graph.GraphMembership, error) {
	return f.AddMembershipFn(ctx, args)
}

// GetMembership calls GetMembershipFn.
func (f *GroupMembershipClient) GetMembership(ctx context.Context, args graph.GetMembershipArgs) (*graph.GraphMembership, error) {
	return f.GetMembershipFn(ctx, args)
}

// GetMembershipState calls GetMembershipStateFn.
func (f *GroupMembershipClient) GetMembershipState(ctx context.Context, args graph.GetMembershipStateArgs) (*graph.GraphMembershipState, error) {
	return f.GetMembershipStateFn(ctx, args)
}

// RemoveMembership calls RemoveMembershipFn.
func (f *GroupMembershipClient) RemoveMembership(ctx context.Context, args graph.RemoveMembershipArgs) error {
	return f.RemoveMembershipFn(ctx, args)
}
