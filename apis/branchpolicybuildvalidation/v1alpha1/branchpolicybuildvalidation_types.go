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

package v1alpha1

import (
	"reflect"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reference"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	builddefinitionv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/builddefinition/v1alpha1"
)

// BranchPolicyBuildValidationParameters are the configurable fields of a
// BranchPolicyBuildValidation policy configuration.
type BranchPolicyBuildValidationParameters struct {
	// ProjectID is the Azure DevOps project GUID that owns this branch policy.
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1.Project
	// +crossplane:generate:reference:extractor=github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1.ProjectID()
	// +optional
	// +kubebuilder:validation:MinLength=36
	// +kubebuilder:validation:MaxLength=36
	ProjectID string `json:"projectId,omitempty"`

	// ProjectIDRef references the Project that owns this policy.
	// +optional
	ProjectIDRef *xpv2.NamespacedReference `json:"projectIdRef,omitempty"`

	// ProjectIDSelector selects a Project that owns this policy.
	// +optional
	ProjectIDSelector *xpv2.NamespacedSelector `json:"projectIdSelector,omitempty"`

	// RepositoryID is the Azure DevOps Git repository GUID this policy applies to.
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/gitrepository/v1alpha1.GitRepository
	// +crossplane:generate:reference:extractor=github.com/dunkin0486/provider-azuredevops/apis/gitrepository/v1alpha1.GitRepositoryID()
	// +optional
	// +kubebuilder:validation:MinLength=36
	// +kubebuilder:validation:MaxLength=36
	RepositoryID string `json:"repositoryId,omitempty"`

	// RepositoryIDRef references the GitRepository this policy applies to.
	// +optional
	RepositoryIDRef *xpv2.NamespacedReference `json:"repositoryIdRef,omitempty"`

	// RepositoryIDSelector selects a GitRepository this policy applies to.
	// +optional
	RepositoryIDSelector *xpv2.NamespacedSelector `json:"repositoryIdSelector,omitempty"`

	// Branch is the branch ref this policy applies to, for example
	// "refs/heads/main" or the wildcard prefix form "refs/heads/release/*".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Branch string `json:"branch"`

	// BuildDefinitionID is the Azure DevOps integer build definition ID that
	// must succeed before pull requests can complete.
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/builddefinition/v1alpha1.BuildDefinition
	// +crossplane:generate:reference:extractor=github.com/dunkin0486/provider-azuredevops/apis/branchpolicybuildvalidation/v1alpha1.BuildDefinitionID()
	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9]+$`
	BuildDefinitionID string `json:"buildDefinitionId,omitempty"`

	// BuildDefinitionIDRef references the BuildDefinition this policy requires.
	// +optional
	BuildDefinitionIDRef *xpv2.NamespacedReference `json:"buildDefinitionIdRef,omitempty"`

	// BuildDefinitionIDSelector selects a BuildDefinition this policy requires.
	// +optional
	BuildDefinitionIDSelector *xpv2.NamespacedSelector `json:"buildDefinitionIdSelector,omitempty"`

	// DisplayName overrides Azure DevOps' default "Build" policy name when set.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// ManualQueueOnly disables automatic build queueing and requires users to
	// manually queue validation builds.
	// +optional
	// +kubebuilder:default=false
	ManualQueueOnly bool `json:"manualQueueOnly,omitempty"`

	// ValidDuration is the number of minutes an automatically queued build
	// remains valid after the source branch updates. When set above zero, the
	// Azure DevOps queueOnSourceUpdateOnly setting is enabled.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ValidDuration int `json:"validDuration,omitempty"`

	// Enabled controls whether the policy is active.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Blocking controls whether the policy blocks pull request completion.
	// +optional
	// +kubebuilder:default=true
	Blocking bool `json:"blocking,omitempty"`
}

// BranchPolicyBuildValidationObservation are the observable fields of a
// BranchPolicyBuildValidation policy configuration.
type BranchPolicyBuildValidationObservation struct {
	// ID is the Azure DevOps policy configuration ID.
	ID string `json:"id,omitempty"`

	// IsEnabled is the enablement state currently reported by Azure DevOps.
	IsEnabled bool `json:"isEnabled,omitempty"`

	// IsEnterpriseManaged reports whether this policy requires the Azure
	// DevOps "Manage Enterprise Policies" permission to modify.
	IsEnterpriseManaged bool `json:"isEnterpriseManaged,omitempty"`
}

// A BranchPolicyBuildValidationSpec defines the desired state of a
// BranchPolicyBuildValidation.
type BranchPolicyBuildValidationSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              BranchPolicyBuildValidationParameters `json:"forProvider"`
}

// A BranchPolicyBuildValidationStatus represents the observed state of a
// BranchPolicyBuildValidation.
type BranchPolicyBuildValidationStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 BranchPolicyBuildValidationObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A BranchPolicyBuildValidation manages an Azure DevOps build validation
// branch policy.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type BranchPolicyBuildValidation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BranchPolicyBuildValidationSpec   `json:"spec"`
	Status BranchPolicyBuildValidationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BranchPolicyBuildValidationList contains a list of BranchPolicyBuildValidation.
type BranchPolicyBuildValidationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BranchPolicyBuildValidation `json:"items"`
}

// BranchPolicyBuildValidation type metadata.
var (
	BranchPolicyBuildValidationKind             = reflect.TypeOf(BranchPolicyBuildValidation{}).Name()
	BranchPolicyBuildValidationGroupKind        = schema.GroupKind{Group: Group, Kind: BranchPolicyBuildValidationKind}.String()
	BranchPolicyBuildValidationKindAPIVersion   = BranchPolicyBuildValidationKind + "." + SchemeGroupVersion.String()
	BranchPolicyBuildValidationGroupVersionKind = SchemeGroupVersion.WithKind(BranchPolicyBuildValidationKind)
)

func init() {
	SchemeBuilder.Register(&BranchPolicyBuildValidation{}, &BranchPolicyBuildValidationList{})
}

// BuildDefinitionID extracts a referenced BuildDefinition's observed Azure
// DevOps integer ID.
func BuildDefinitionID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		r, ok := mg.(*builddefinitionv1alpha1.BuildDefinition)
		if !ok {
			return ""
		}
		return r.Status.AtProvider.ID
	}
}
