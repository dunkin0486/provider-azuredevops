/*
Copyright 2020 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/dunkin0486/provider-azuredevops/internal/controller/agentpool"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/branchpolicyminreviewers"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/builddefinition"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/config"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/environment"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/gitrepository"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/groupmembership"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/project"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/serviceendpointazurerm"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/serviceendpointdockerregistry"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/serviceendpointgeneric"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/serviceendpointgithub"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/serviceendpointkubernetes"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/team"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/variablegroup"
)

// SetupGated creates all AzureDevOps controllers with safe-start support and adds them to
// the supplied manager.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		config.Setup,
		agentpool.SetupGated,
		branchpolicyminreviewers.SetupGated,
		builddefinition.SetupGated,
		environment.SetupGated,
		gitrepository.SetupGated,
		groupmembership.SetupGated,
		project.SetupGated,
		serviceendpointazurerm.SetupGated,
		serviceendpointdockerregistry.SetupGated,
		serviceendpointgeneric.SetupGated,
		serviceendpointgithub.SetupGated,
		serviceendpointkubernetes.SetupGated,
		team.SetupGated,
		variablegroup.SetupGated,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}
