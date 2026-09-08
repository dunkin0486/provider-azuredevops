// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package group

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/graph"
)

// GroupClient is the subset of the Azure DevOps Graph API used by this
// controller to manage groups.
type GroupClient interface {
	CreateGroupVsts(ctx context.Context, args graph.CreateGroupVstsArgs) (*graph.GraphGroup, error)
	GetGroup(ctx context.Context, args graph.GetGroupArgs) (*graph.GraphGroup, error)
	UpdateGroup(ctx context.Context, args graph.UpdateGroupArgs) (*graph.GraphGroup, error)
	DeleteGroup(ctx context.Context, args graph.DeleteGroupArgs) error
}

// newGroupClient builds the real Azure DevOps SDK graph client used by this
// controller from a resolved Azure DevOps SDK connection.
func newGroupClient(ctx context.Context, connection *azuredevops.Connection) (GroupClient, error) {
	return graph.NewClient(ctx, connection)
}
