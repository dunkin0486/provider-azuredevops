// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the
// gitrepository package's GitRepositoryClient interface for unit tests.
package fake

import (
	"context"

	adogit "github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

// GitRepositoryClient is a fake implementation of the gitrepository package's
// GitRepositoryClient interface, backed by function fields so tests can stub
// only the methods they exercise.
type GitRepositoryClient struct {
	GetRepositoryFn    func(ctx context.Context, args adogit.GetRepositoryArgs) (*adogit.GitRepository, error)
	CreateRepositoryFn func(ctx context.Context, args adogit.CreateRepositoryArgs) (*adogit.GitRepository, error)
	UpdateRepositoryFn func(ctx context.Context, args adogit.UpdateRepositoryArgs) (*adogit.GitRepository, error)
	DeleteRepositoryFn func(ctx context.Context, args adogit.DeleteRepositoryArgs) error
}

// GetRepository calls GetRepositoryFn.
func (f *GitRepositoryClient) GetRepository(ctx context.Context, args adogit.GetRepositoryArgs) (*adogit.GitRepository, error) {
	return f.GetRepositoryFn(ctx, args)
}

// CreateRepository calls CreateRepositoryFn.
func (f *GitRepositoryClient) CreateRepository(ctx context.Context, args adogit.CreateRepositoryArgs) (*adogit.GitRepository, error) {
	return f.CreateRepositoryFn(ctx, args)
}

// UpdateRepository calls UpdateRepositoryFn.
func (f *GitRepositoryClient) UpdateRepository(ctx context.Context, args adogit.UpdateRepositoryArgs) (*adogit.GitRepository, error) {
	return f.UpdateRepositoryFn(ctx, args)
}

// DeleteRepository calls DeleteRepositoryFn.
func (f *GitRepositoryClient) DeleteRepository(ctx context.Context, args adogit.DeleteRepositoryArgs) error {
	return f.DeleteRepositoryFn(ctx, args)
}
