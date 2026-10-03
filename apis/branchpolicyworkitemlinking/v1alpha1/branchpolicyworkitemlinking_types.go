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

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// BranchPolicyWorkItemLinkingParameters are the configurable fields of a
// BranchPolicyWorkItemLinking policy configuration.
type BranchPolicyWorkItemLinkingParameters struct {
	// ProjectID is the Azure DevOps project GUID that owns this branch policy.
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1.Project
	// +crossplane:generate:reference:extractor=github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1.ProjectID()
	// +optional
	// +kubebuilder:validation:MinLength=36
	// +kubebuilder:validation:MaxLength=36
	ProjectID string `json:"projectId,omitempty"`

	// ProjectIDRef references the Project that owns this branch policy.
	// +optional
	ProjectIDRef *xpv2.NamespacedReference `json:"projectIdRef,omitempty"`

	// ProjectIDSelector selects a Project that owns this branch policy.
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
	// "refs/heads/main" or the wildcard prefix form "refs/heads/*".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Branch string `json:"branch"`

	// Enabled controls whether the policy is active.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Blocking controls whether the policy blocks pull request completion.
	// +optional
	// +kubebuilder:default=true
	Blocking bool `json:"blocking,omitempty"`
}

// BranchPolicyWorkItemLinkingObservation are the observable fields of a
// BranchPolicyWorkItemLinking policy configuration.
type BranchPolicyWorkItemLinkingObservation struct {
	// ID is the Azure DevOps policy configuration ID.
	ID string `json:"id,omitempty"`
}

// A BranchPolicyWorkItemLinkingSpec defines the desired state of a
// BranchPolicyWorkItemLinking.
type BranchPolicyWorkItemLinkingSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              BranchPolicyWorkItemLinkingParameters `json:"forProvider"`
}

// A BranchPolicyWorkItemLinkingStatus represents the observed state of a
// BranchPolicyWorkItemLinking.
type BranchPolicyWorkItemLinkingStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 BranchPolicyWorkItemLinkingObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A BranchPolicyWorkItemLinking manages Azure DevOps' "Work item linking"
// branch policy for one repository branch or branch prefix.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type BranchPolicyWorkItemLinking struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BranchPolicyWorkItemLinkingSpec   `json:"spec"`
	Status BranchPolicyWorkItemLinkingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BranchPolicyWorkItemLinkingList contains a list of BranchPolicyWorkItemLinking.
type BranchPolicyWorkItemLinkingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BranchPolicyWorkItemLinking `json:"items"`
}

// BranchPolicyWorkItemLinking type metadata.
var (
	BranchPolicyWorkItemLinkingKind             = reflect.TypeOf(BranchPolicyWorkItemLinking{}).Name()
	BranchPolicyWorkItemLinkingGroupKind        = schema.GroupKind{Group: Group, Kind: BranchPolicyWorkItemLinkingKind}.String()
	BranchPolicyWorkItemLinkingKindAPIVersion   = BranchPolicyWorkItemLinkingKind + "." + SchemeGroupVersion.String()
	BranchPolicyWorkItemLinkingGroupVersionKind = SchemeGroupVersion.WithKind(BranchPolicyWorkItemLinkingKind)
)

func init() {
	SchemeBuilder.Register(&BranchPolicyWorkItemLinking{}, &BranchPolicyWorkItemLinkingList{})
}
