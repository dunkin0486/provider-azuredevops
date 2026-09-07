// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the team
// package's TeamClient interface for unit tests.
package fake

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
)

// TeamClient is a fake implementation of the team package's TeamClient
// interface, backed by function fields so tests can stub only the methods
// they exercise.
type TeamClient struct {
	GetTeamFn    func(ctx context.Context, args core.GetTeamArgs) (*core.WebApiTeam, error)
	CreateTeamFn func(ctx context.Context, args core.CreateTeamArgs) (*core.WebApiTeam, error)
	UpdateTeamFn func(ctx context.Context, args core.UpdateTeamArgs) (*core.WebApiTeam, error)
	DeleteTeamFn func(ctx context.Context, args core.DeleteTeamArgs) error
}

// GetTeam calls GetTeamFn.
func (f *TeamClient) GetTeam(ctx context.Context, args core.GetTeamArgs) (*core.WebApiTeam, error) {
	return f.GetTeamFn(ctx, args)
}

// CreateTeam calls CreateTeamFn.
func (f *TeamClient) CreateTeam(ctx context.Context, args core.CreateTeamArgs) (*core.WebApiTeam, error) {
	return f.CreateTeamFn(ctx, args)
}

// UpdateTeam calls UpdateTeamFn.
func (f *TeamClient) UpdateTeam(ctx context.Context, args core.UpdateTeamArgs) (*core.WebApiTeam, error) {
	return f.UpdateTeamFn(ctx, args)
}

// DeleteTeam calls DeleteTeamFn.
func (f *TeamClient) DeleteTeam(ctx context.Context, args core.DeleteTeamArgs) error {
	return f.DeleteTeamFn(ctx, args)
}
