// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package serviceendpointazurerm

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/serviceendpoint"
)

// ServiceEndpointClient is the subset of the Azure DevOps Service Endpoint API
// used by this controller.
type ServiceEndpointClient interface {
	CreateServiceEndpoint(ctx context.Context, args serviceendpoint.CreateServiceEndpointArgs) (*serviceendpoint.ServiceEndpoint, error)
	DeleteServiceEndpoint(ctx context.Context, args serviceendpoint.DeleteServiceEndpointArgs) error
	GetServiceEndpointDetails(ctx context.Context, args serviceendpoint.GetServiceEndpointDetailsArgs) (*serviceendpoint.ServiceEndpoint, error)
	UpdateServiceEndpoint(ctx context.Context, args serviceendpoint.UpdateServiceEndpointArgs) (*serviceendpoint.ServiceEndpoint, error)
}

// newServiceEndpointClient builds the real Azure DevOps SDK client used by this controller.
func newServiceEndpointClient(ctx context.Context, connection *azuredevops.Connection) (ServiceEndpointClient, error) {
	return serviceendpoint.NewClient(ctx, connection)
}
