// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the group
// package's GroupClient interface, for use in controller unit tests. It
// lives in its own subpackage to avoid an import cycle with the group
// package's tests.
package fake

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/graph"
)

// GroupClient is a fake implementation of the group package's GroupClient
// interface, backed by function fields so individual test cases can stub out
// only the methods they exercise.
type GroupClient struct {
	CreateGroupVstsFn func(ctx context.Context, args graph.CreateGroupVstsArgs) (*graph.GraphGroup, error)
	GetGroupFn        func(ctx context.Context, args graph.GetGroupArgs) (*graph.GraphGroup, error)
	UpdateGroupFn     func(ctx context.Context, args graph.UpdateGroupArgs) (*graph.GraphGroup, error)
	DeleteGroupFn     func(ctx context.Context, args graph.DeleteGroupArgs) error
}

// CreateGroupVsts calls CreateGroupVstsFn.
func (f *GroupClient) CreateGroupVsts(ctx context.Context, args graph.CreateGroupVstsArgs) (*graph.GraphGroup, error) {
	return f.CreateGroupVstsFn(ctx, args)
}

// GetGroup calls GetGroupFn.
func (f *GroupClient) GetGroup(ctx context.Context, args graph.GetGroupArgs) (*graph.GraphGroup, error) {
	return f.GetGroupFn(ctx, args)
}

// UpdateGroup calls UpdateGroupFn.
func (f *GroupClient) UpdateGroup(ctx context.Context, args graph.UpdateGroupArgs) (*graph.GraphGroup, error) {
	return f.UpdateGroupFn(ctx, args)
}

// DeleteGroup calls DeleteGroupFn.
func (f *GroupClient) DeleteGroup(ctx context.Context, args graph.DeleteGroupArgs) error {
	return f.DeleteGroupFn(ctx, args)
}
