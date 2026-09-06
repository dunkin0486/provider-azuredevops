// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package builddefinition

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	adobuild "github.com/microsoft/azure-devops-go-api/azuredevops/v7/build"
)

// BuildDefinitionClient is the subset of the Azure DevOps Build API client
// used by this controller to manage BuildDefinition resources.
type BuildDefinitionClient interface {
	GetDefinition(ctx context.Context, args adobuild.GetDefinitionArgs) (*adobuild.BuildDefinition, error)
	CreateDefinition(ctx context.Context, args adobuild.CreateDefinitionArgs) (*adobuild.BuildDefinition, error)
	UpdateDefinition(ctx context.Context, args adobuild.UpdateDefinitionArgs) (*adobuild.BuildDefinition, error)
	DeleteDefinition(ctx context.Context, args adobuild.DeleteDefinitionArgs) error
}

// newBuildDefinitionClient builds the real Azure DevOps Build SDK client used
// by this controller from a connection resolved via the shared
// internal/clients/azuredevops package.
func newBuildDefinitionClient(ctx context.Context, connection *azuredevops.Connection) (BuildDefinitionClient, error) {
	return adobuild.NewClient(ctx, connection)
}
