// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the
// userentitlement package's UserEntitlementClient interface, for use in
// controller unit tests. It lives in its own subpackage to avoid an import
// cycle with the userentitlement package's tests.
package fake

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/memberentitlementmanagement"
)

// UserEntitlementClient is a fake implementation of the userentitlement
// package's UserEntitlementClient interface, backed by function fields so
// individual test cases can stub out only the methods they exercise.
type UserEntitlementClient struct {
	AddUserEntitlementFn    func(ctx context.Context, args memberentitlementmanagement.AddUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPostResponse, error)
	GetUserEntitlementFn    func(ctx context.Context, args memberentitlementmanagement.GetUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlement, error)
	UpdateUserEntitlementFn func(ctx context.Context, args memberentitlementmanagement.UpdateUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPatchResponse, error)
	DeleteUserEntitlementFn func(ctx context.Context, args memberentitlementmanagement.DeleteUserEntitlementArgs) error
}

// AddUserEntitlement calls AddUserEntitlementFn.
func (f *UserEntitlementClient) AddUserEntitlement(ctx context.Context, args memberentitlementmanagement.AddUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPostResponse, error) {
	return f.AddUserEntitlementFn(ctx, args)
}

// GetUserEntitlement calls GetUserEntitlementFn.
func (f *UserEntitlementClient) GetUserEntitlement(ctx context.Context, args memberentitlementmanagement.GetUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlement, error) {
	return f.GetUserEntitlementFn(ctx, args)
}

// UpdateUserEntitlement calls UpdateUserEntitlementFn.
func (f *UserEntitlementClient) UpdateUserEntitlement(ctx context.Context, args memberentitlementmanagement.UpdateUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPatchResponse, error) {
	return f.UpdateUserEntitlementFn(ctx, args)
}

// DeleteUserEntitlement calls DeleteUserEntitlementFn.
func (f *UserEntitlementClient) DeleteUserEntitlement(ctx context.Context, args memberentitlementmanagement.DeleteUserEntitlementArgs) error {
	return f.DeleteUserEntitlementFn(ctx, args)
}
