// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the
// variablegroup package's VariableGroupClient interface, for use in controller
// unit tests. It lives in its own subpackage to avoid an import cycle with the
// variablegroup package's tests.
package fake

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
)

// VariableGroupClient is a fake implementation of the variablegroup package's
// VariableGroupClient interface, backed by function fields so individual test
// cases can stub out only the methods they exercise.
type VariableGroupClient struct {
	AddVariableGroupFn    func(ctx context.Context, args taskagent.AddVariableGroupArgs) (*taskagent.VariableGroup, error)
	GetVariableGroupFn    func(ctx context.Context, args taskagent.GetVariableGroupArgs) (*taskagent.VariableGroup, error)
	UpdateVariableGroupFn func(ctx context.Context, args taskagent.UpdateVariableGroupArgs) (*taskagent.VariableGroup, error)
	DeleteVariableGroupFn func(ctx context.Context, args taskagent.DeleteVariableGroupArgs) error
}

// AddVariableGroup calls AddVariableGroupFn.
func (f *VariableGroupClient) AddVariableGroup(ctx context.Context, args taskagent.AddVariableGroupArgs) (*taskagent.VariableGroup, error) {
	return f.AddVariableGroupFn(ctx, args)
}

// GetVariableGroup calls GetVariableGroupFn.
func (f *VariableGroupClient) GetVariableGroup(ctx context.Context, args taskagent.GetVariableGroupArgs) (*taskagent.VariableGroup, error) {
	return f.GetVariableGroupFn(ctx, args)
}

// UpdateVariableGroup calls UpdateVariableGroupFn.
func (f *VariableGroupClient) UpdateVariableGroup(ctx context.Context, args taskagent.UpdateVariableGroupArgs) (*taskagent.VariableGroup, error) {
	return f.UpdateVariableGroupFn(ctx, args)
}

// DeleteVariableGroup calls DeleteVariableGroupFn.
func (f *VariableGroupClient) DeleteVariableGroup(ctx context.Context, args taskagent.DeleteVariableGroupArgs) error {
	return f.DeleteVariableGroupFn(ctx, args)
}
