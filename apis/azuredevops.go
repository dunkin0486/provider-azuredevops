/*
Copyright 2025 The Crossplane Authors.

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

// Package apis contains Kubernetes API for the AzureDevOps provider.
package apis

import (
	"k8s.io/apimachinery/pkg/runtime"

	agentpoolv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/agentpool/v1alpha1"
	branchpolicyminreviewersv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/branchpolicyminreviewers/v1alpha1"
	builddefinitionv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/builddefinition/v1alpha1"
	environmentv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/environment/v1alpha1"
	gitrepositoryv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/gitrepository/v1alpha1"
	groupmembershipv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/groupmembership/v1alpha1"
	projectv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1"
	serviceendpointazurermv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointazurerm/v1alpha1"
	serviceendpointgenericv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointgeneric/v1alpha1"
	serviceendpointgithubv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointgithub/v1alpha1"
	serviceendpointkubernetesv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointkubernetes/v1alpha1"
	teamv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/team/v1alpha1"
	azuredevopsv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/v1alpha1"
	variablegroupv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/variablegroup/v1alpha1"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes,
		azuredevopsv1alpha1.SchemeBuilder.AddToScheme,
		agentpoolv1alpha1.SchemeBuilder.AddToScheme,
		branchpolicyminreviewersv1alpha1.SchemeBuilder.AddToScheme,
		builddefinitionv1alpha1.SchemeBuilder.AddToScheme,
		environmentv1alpha1.SchemeBuilder.AddToScheme,
		gitrepositoryv1alpha1.SchemeBuilder.AddToScheme,
		groupmembershipv1alpha1.SchemeBuilder.AddToScheme,
		projectv1alpha1.SchemeBuilder.AddToScheme,
		serviceendpointazurermv1alpha1.SchemeBuilder.AddToScheme,
		serviceendpointgenericv1alpha1.SchemeBuilder.AddToScheme,
		serviceendpointgithubv1alpha1.SchemeBuilder.AddToScheme,
		serviceendpointkubernetesv1alpha1.SchemeBuilder.AddToScheme,
		teamv1alpha1.SchemeBuilder.AddToScheme,
		variablegroupv1alpha1.SchemeBuilder.AddToScheme,
	)
}

// AddToSchemes may be used to add all resources defined in the project to a Scheme
var AddToSchemes runtime.SchemeBuilder

// AddToScheme adds all Resources to the Scheme
func AddToScheme(s *runtime.Scheme) error {
	return AddToSchemes.AddToScheme(s)
}
