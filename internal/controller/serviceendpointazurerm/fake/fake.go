// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a hand-written fake implementation of the
// serviceendpointazurerm package's ServiceEndpointClient interface.
package fake

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/serviceendpoint"
)

// ServiceEndpointClient is a fake implementation backed by function fields.
type ServiceEndpointClient struct {
	CreateServiceEndpointFn     func(ctx context.Context, args serviceendpoint.CreateServiceEndpointArgs) (*serviceendpoint.ServiceEndpoint, error)
	DeleteServiceEndpointFn     func(ctx context.Context, args serviceendpoint.DeleteServiceEndpointArgs) error
	GetServiceEndpointDetailsFn func(ctx context.Context, args serviceendpoint.GetServiceEndpointDetailsArgs) (*serviceendpoint.ServiceEndpoint, error)
	UpdateServiceEndpointFn     func(ctx context.Context, args serviceendpoint.UpdateServiceEndpointArgs) (*serviceendpoint.ServiceEndpoint, error)
}

// CreateServiceEndpoint calls CreateServiceEndpointFn.
func (f *ServiceEndpointClient) CreateServiceEndpoint(ctx context.Context, args serviceendpoint.CreateServiceEndpointArgs) (*serviceendpoint.ServiceEndpoint, error) {
	return f.CreateServiceEndpointFn(ctx, args)
}

// DeleteServiceEndpoint calls DeleteServiceEndpointFn.
func (f *ServiceEndpointClient) DeleteServiceEndpoint(ctx context.Context, args serviceendpoint.DeleteServiceEndpointArgs) error {
	return f.DeleteServiceEndpointFn(ctx, args)
}

// GetServiceEndpointDetails calls GetServiceEndpointDetailsFn.
func (f *ServiceEndpointClient) GetServiceEndpointDetails(ctx context.Context, args serviceendpoint.GetServiceEndpointDetailsArgs) (*serviceendpoint.ServiceEndpoint, error) {
	return f.GetServiceEndpointDetailsFn(ctx, args)
}

// UpdateServiceEndpoint calls UpdateServiceEndpointFn.
func (f *ServiceEndpointClient) UpdateServiceEndpoint(ctx context.Context, args serviceendpoint.UpdateServiceEndpointArgs) (*serviceendpoint.ServiceEndpoint, error) {
	return f.UpdateServiceEndpointFn(ctx, args)
}
