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

// BranchPolicyMinReviewersParameters are the configurable fields of a
// BranchPolicyMinReviewers policy configuration.
type BranchPolicyMinReviewersParameters struct {
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

	// MinimumApproverCount is the minimum number of reviewers required to
	// approve a pull request.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	MinimumApproverCount int `json:"minimumApproverCount"`

	// CreatorVoteCounts allows the pull request creator's vote to count
	// toward the minimum approval count.
	// +optional
	// +kubebuilder:default=false
	CreatorVoteCounts bool `json:"creatorVoteCounts,omitempty"`

	// AllowCompletionWithRejects allows pull requests to complete even when
	// there are reject or wait votes.
	// +optional
	// +kubebuilder:default=false
	AllowCompletionWithRejects bool `json:"allowCompletionWithRejects,omitempty"`

	// ResetOnSourcePush resets approval votes when new changes are pushed.
	// +optional
	// +kubebuilder:default=false
	ResetOnSourcePush bool `json:"resetOnSourcePush,omitempty"`

	// Enabled controls whether the policy is active.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Blocking controls whether the policy blocks pull request completion.
	// +optional
	// +kubebuilder:default=true
	Blocking bool `json:"blocking,omitempty"`
}

// BranchPolicyMinReviewersObservation are the observable fields of a
// BranchPolicyMinReviewers policy configuration.
type BranchPolicyMinReviewersObservation struct {
	// ID is the Azure DevOps policy configuration ID.
	ID string `json:"id,omitempty"`

	// IsEnterpriseManaged reports whether this policy requires the Azure
	// DevOps "Manage Enterprise Policies" permission to modify.
	IsEnterpriseManaged bool `json:"isEnterpriseManaged,omitempty"`
}

// A BranchPolicyMinReviewersSpec defines the desired state of a
// BranchPolicyMinReviewers.
type BranchPolicyMinReviewersSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              BranchPolicyMinReviewersParameters `json:"forProvider"`
}

// A BranchPolicyMinReviewersStatus represents the observed state of a
// BranchPolicyMinReviewers.
type BranchPolicyMinReviewersStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 BranchPolicyMinReviewersObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A BranchPolicyMinReviewers manages Azure DevOps' "Minimum approval count"
// branch policy for one repository branch or branch prefix.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type BranchPolicyMinReviewers struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BranchPolicyMinReviewersSpec   `json:"spec"`
	Status BranchPolicyMinReviewersStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BranchPolicyMinReviewersList contains a list of BranchPolicyMinReviewers.
type BranchPolicyMinReviewersList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BranchPolicyMinReviewers `json:"items"`
}

// BranchPolicyMinReviewers type metadata.
var (
	BranchPolicyMinReviewersKind             = reflect.TypeOf(BranchPolicyMinReviewers{}).Name()
	BranchPolicyMinReviewersGroupKind        = schema.GroupKind{Group: Group, Kind: BranchPolicyMinReviewersKind}.String()
	BranchPolicyMinReviewersKindAPIVersion   = BranchPolicyMinReviewersKind + "." + SchemeGroupVersion.String()
	BranchPolicyMinReviewersGroupVersionKind = SchemeGroupVersion.WithKind(BranchPolicyMinReviewersKind)
)

func init() {
	SchemeBuilder.Register(&BranchPolicyMinReviewers{}, &BranchPolicyMinReviewersList{})
}
