// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the environment
// package's EnvironmentClient interface for unit tests.
package fake

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
)

// EnvironmentClient is a fake implementation of the environment package's
// EnvironmentClient interface, backed by function fields so tests can stub
// only the methods they exercise.
type EnvironmentClient struct {
	AddEnvironmentFn     func(ctx context.Context, args taskagent.AddEnvironmentArgs) (*taskagent.EnvironmentInstance, error)
	GetEnvironmentByIdFn func(ctx context.Context, args taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error)
	UpdateEnvironmentFn  func(ctx context.Context, args taskagent.UpdateEnvironmentArgs) (*taskagent.EnvironmentInstance, error)
	DeleteEnvironmentFn  func(ctx context.Context, args taskagent.DeleteEnvironmentArgs) error
}

// AddEnvironment calls AddEnvironmentFn.
func (f *EnvironmentClient) AddEnvironment(ctx context.Context, args taskagent.AddEnvironmentArgs) (*taskagent.EnvironmentInstance, error) {
	return f.AddEnvironmentFn(ctx, args)
}

// GetEnvironmentById calls GetEnvironmentByIdFn.
func (f *EnvironmentClient) GetEnvironmentById(ctx context.Context, args taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
	return f.GetEnvironmentByIdFn(ctx, args)
}

// UpdateEnvironment calls UpdateEnvironmentFn.
func (f *EnvironmentClient) UpdateEnvironment(ctx context.Context, args taskagent.UpdateEnvironmentArgs) (*taskagent.EnvironmentInstance, error) {
	return f.UpdateEnvironmentFn(ctx, args)
}

// DeleteEnvironment calls DeleteEnvironmentFn.
func (f *EnvironmentClient) DeleteEnvironment(ctx context.Context, args taskagent.DeleteEnvironmentArgs) error {
	return f.DeleteEnvironmentFn(ctx, args)
}
