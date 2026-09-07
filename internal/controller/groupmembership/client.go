// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package groupmembership

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/graph"
)

// GroupMembershipClient is the subset of the Azure DevOps Graph API used by
// this controller to manage group memberships.
type GroupMembershipClient interface {
	AddMembership(ctx context.Context, args graph.AddMembershipArgs) (*graph.GraphMembership, error)
	GetMembership(ctx context.Context, args graph.GetMembershipArgs) (*graph.GraphMembership, error)
	GetMembershipState(ctx context.Context, args graph.GetMembershipStateArgs) (*graph.GraphMembershipState, error)
	RemoveMembership(ctx context.Context, args graph.RemoveMembershipArgs) error
}

// newGroupMembershipClient builds the real Azure DevOps SDK graph client used
// by this controller from a resolved Azure DevOps SDK connection.
func newGroupMembershipClient(ctx context.Context, connection *azuredevops.Connection) (GroupMembershipClient, error) {
	return graph.NewClient(ctx, connection)
}
