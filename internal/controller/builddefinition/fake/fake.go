// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the
// builddefinition package's BuildDefinitionClient interface for unit tests.
package fake

import (
	"context"

	adobuild "github.com/microsoft/azure-devops-go-api/azuredevops/v7/build"
)

// BuildDefinitionClient is a fake implementation of the builddefinition
// package's BuildDefinitionClient interface, backed by function fields so tests
// can stub only the methods they exercise.
type BuildDefinitionClient struct {
	GetDefinitionFn    func(ctx context.Context, args adobuild.GetDefinitionArgs) (*adobuild.BuildDefinition, error)
	CreateDefinitionFn func(ctx context.Context, args adobuild.CreateDefinitionArgs) (*adobuild.BuildDefinition, error)
	UpdateDefinitionFn func(ctx context.Context, args adobuild.UpdateDefinitionArgs) (*adobuild.BuildDefinition, error)
	DeleteDefinitionFn func(ctx context.Context, args adobuild.DeleteDefinitionArgs) error
}

// GetDefinition calls GetDefinitionFn.
func (f *BuildDefinitionClient) GetDefinition(ctx context.Context, args adobuild.GetDefinitionArgs) (*adobuild.BuildDefinition, error) {
	return f.GetDefinitionFn(ctx, args)
}

// CreateDefinition calls CreateDefinitionFn.
func (f *BuildDefinitionClient) CreateDefinition(ctx context.Context, args adobuild.CreateDefinitionArgs) (*adobuild.BuildDefinition, error) {
	return f.CreateDefinitionFn(ctx, args)
}

// UpdateDefinition calls UpdateDefinitionFn.
func (f *BuildDefinitionClient) UpdateDefinition(ctx context.Context, args adobuild.UpdateDefinitionArgs) (*adobuild.BuildDefinition, error) {
	return f.UpdateDefinitionFn(ctx, args)
}

// DeleteDefinition calls DeleteDefinitionFn.
func (f *BuildDefinitionClient) DeleteDefinition(ctx context.Context, args adobuild.DeleteDefinitionArgs) error {
	return f.DeleteDefinitionFn(ctx, args)
}
