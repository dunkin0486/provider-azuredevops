// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the project
// package's ProjectClient and OperationsClient interfaces, for use in
// controller unit tests. It lives in its own subpackage to avoid an import
// cycle with the project package's tests.
package fake

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/operations"
)

// ProjectClient is a fake implementation of the project package's
// ProjectClient interface, backed by function fields so individual test
// cases can stub out only the methods they exercise.
type ProjectClient struct {
	GetProjectFn         func(ctx context.Context, args core.GetProjectArgs) (*core.TeamProject, error)
	GetProcessesFn       func(ctx context.Context, args core.GetProcessesArgs) (*[]core.Process, error)
	QueueCreateProjectFn func(ctx context.Context, args core.QueueCreateProjectArgs) (*operations.OperationReference, error)
	UpdateProjectFn      func(ctx context.Context, args core.UpdateProjectArgs) (*operations.OperationReference, error)
	QueueDeleteProjectFn func(ctx context.Context, args core.QueueDeleteProjectArgs) (*operations.OperationReference, error)
}

// GetProject calls GetProjectFn.
func (f *ProjectClient) GetProject(ctx context.Context, args core.GetProjectArgs) (*core.TeamProject, error) {
	return f.GetProjectFn(ctx, args)
}

// GetProcesses calls GetProcessesFn.
func (f *ProjectClient) GetProcesses(ctx context.Context, args core.GetProcessesArgs) (*[]core.Process, error) {
	return f.GetProcessesFn(ctx, args)
}

// QueueCreateProject calls QueueCreateProjectFn.
func (f *ProjectClient) QueueCreateProject(ctx context.Context, args core.QueueCreateProjectArgs) (*operations.OperationReference, error) {
	return f.QueueCreateProjectFn(ctx, args)
}

// UpdateProject calls UpdateProjectFn.
func (f *ProjectClient) UpdateProject(ctx context.Context, args core.UpdateProjectArgs) (*operations.OperationReference, error) {
	return f.UpdateProjectFn(ctx, args)
}

// QueueDeleteProject calls QueueDeleteProjectFn.
func (f *ProjectClient) QueueDeleteProject(ctx context.Context, args core.QueueDeleteProjectArgs) (*operations.OperationReference, error) {
	return f.QueueDeleteProjectFn(ctx, args)
}

// OperationsClient is a fake implementation of the project package's
// OperationsClient interface.
type OperationsClient struct {
	GetOperationFn func(ctx context.Context, args operations.GetOperationArgs) (*operations.Operation, error)
}

// GetOperation calls GetOperationFn.
func (f *OperationsClient) GetOperation(ctx context.Context, args operations.GetOperationArgs) (*operations.Operation, error) {
	return f.GetOperationFn(ctx, args)
}
