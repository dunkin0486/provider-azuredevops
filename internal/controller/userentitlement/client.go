// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package userentitlement

import (
	"context"

	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/memberentitlementmanagement"
)

// UserEntitlementClient is the subset of the Azure DevOps Member
// Entitlement Management API used by this controller to manage user
// entitlements.
type UserEntitlementClient interface {
	AddUserEntitlement(ctx context.Context, args memberentitlementmanagement.AddUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPostResponse, error)
	GetUserEntitlement(ctx context.Context, args memberentitlementmanagement.GetUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlement, error)
	UpdateUserEntitlement(ctx context.Context, args memberentitlementmanagement.UpdateUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPatchResponse, error)
	DeleteUserEntitlement(ctx context.Context, args memberentitlementmanagement.DeleteUserEntitlementArgs) error
}

// newUserEntitlementClient builds the real Azure DevOps SDK member
// entitlement management client used by this controller from a resolved
// Azure DevOps SDK connection.
func newUserEntitlementClient(ctx context.Context, connection *azuredevops.Connection) (UserEntitlementClient, error) {
	return memberentitlementmanagement.NewClient(ctx, connection)
}

// parseUserID parses the external-name annotation (the user entitlement's
// GUID) into a uuid.UUID for use in SDK calls that key on UserId.
func parseUserID(externalName string) (*uuid.UUID, error) {
	id, err := uuid.Parse(externalName)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
