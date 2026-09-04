// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package variablegroup

import (
	"context"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
)

// VariableGroupClient is the subset of the Azure DevOps TaskAgent API used by
// this controller to manage variable groups.
type VariableGroupClient interface {
	AddVariableGroup(ctx context.Context, args taskagent.AddVariableGroupArgs) (*taskagent.VariableGroup, error)
	GetVariableGroup(ctx context.Context, args taskagent.GetVariableGroupArgs) (*taskagent.VariableGroup, error)
	UpdateVariableGroup(ctx context.Context, args taskagent.UpdateVariableGroupArgs) (*taskagent.VariableGroup, error)
	DeleteVariableGroup(ctx context.Context, args taskagent.DeleteVariableGroupArgs) error
}

// newVariableGroupClient builds the real Azure DevOps SDK client used by this
// controller from a resolved Azure DevOps SDK connection.
func newVariableGroupClient(ctx context.Context, connection *azuredevops.Connection) (VariableGroupClient, error) {
	return taskagent.NewClient(ctx, connection)
}
