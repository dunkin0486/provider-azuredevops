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

	projectv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1"
)

// GitRepositoryParameters are the configurable fields of a GitRepository.
type GitRepositoryParameters struct {
	// Name of the Azure DevOps Git repository.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ProjectID is the Azure DevOps project GUID that owns this repository.
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1.Project
	// +crossplane:generate:reference:extractor=ProjectID()
	// +optional
	// +kubebuilder:validation:MinLength=36
	// +kubebuilder:validation:MaxLength=36
	ProjectID string `json:"projectId,omitempty"`

	// ProjectIDRef references the Project that owns this repository.
	// +optional
	ProjectIDRef *xpv2.NamespacedReference `json:"projectIdRef,omitempty"`

	// ProjectIDSelector selects a reference to a Project that owns this repository.
	// +optional
	ProjectIDSelector *xpv2.NamespacedSelector `json:"projectIdSelector,omitempty"`

	// DefaultBranch is the full ref name of the repository's default branch,
	// for example "refs/heads/main".
	// +optional
	DefaultBranch string `json:"defaultBranch,omitempty"`

	// ParentRepositoryID is the Azure DevOps repository GUID to fork from.
	// +crossplane:generate:reference:type=GitRepository
	// +crossplane:generate:reference:extractor=GitRepositoryID()
	// +crossplane:generate:reference:refFieldName=ParentRepositoryRef
	// +crossplane:generate:reference:selectorFieldName=ParentRepositorySelector
	// +optional
	// +kubebuilder:validation:MinLength=36
	// +kubebuilder:validation:MaxLength=36
	ParentRepositoryID string `json:"parentRepositoryId,omitempty"`

	// ParentRepositoryRef references another GitRepository to fork from.
	// +optional
	ParentRepositoryRef *xpv2.NamespacedReference `json:"parentRepositoryRef,omitempty"`

	// ParentRepositorySelector selects another GitRepository to fork from.
	// +optional
	ParentRepositorySelector *xpv2.NamespacedSelector `json:"parentRepositorySelector,omitempty"`

	// Disabled indicates whether the repository should be disabled.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`
}

// GitRepositoryObservation are the observable fields of a GitRepository.
type GitRepositoryObservation struct {
	// ID is the repository GUID assigned by Azure DevOps.
	ID string `json:"id,omitempty"`

	// RemoteURL is the repository's HTTPS remote URL.
	RemoteURL string `json:"remoteUrl,omitempty"`

	// SSHURL is the repository's SSH remote URL.
	SSHURL string `json:"sshUrl,omitempty"`

	// WebURL is the browser URL for the repository.
	WebURL string `json:"webUrl,omitempty"`

	// Size is the compressed size of the repository in bytes.
	Size int64 `json:"size,omitempty"`
}

// A GitRepositorySpec defines the desired state of a GitRepository.
type GitRepositorySpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              GitRepositoryParameters `json:"forProvider"`
}

// A GitRepositoryStatus represents the observed state of a GitRepository.
type GitRepositoryStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 GitRepositoryObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A GitRepository is an Azure DevOps Git repository managed by Crossplane.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type GitRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitRepositorySpec   `json:"spec"`
	Status GitRepositoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitRepositoryList contains a list of GitRepository.
type GitRepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitRepository `json:"items"`
}

// GitRepository type metadata.
var (
	GitRepositoryKind             = reflect.TypeOf(GitRepository{}).Name()
	GitRepositoryGroupKind        = schema.GroupKind{Group: Group, Kind: GitRepositoryKind}.String()
	GitRepositoryKindAPIVersion   = GitRepositoryKind + "." + SchemeGroupVersion.String()
	GitRepositoryGroupVersionKind = SchemeGroupVersion.WithKind(GitRepositoryKind)
)

func init() {
	SchemeBuilder.Register(&GitRepository{}, &GitRepositoryList{})
}

// ProjectID extracts a Project's Azure DevOps GUID from status.atProvider.id.
func ProjectID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		if r, ok := mg.(*projectv1alpha1.Project); ok {
			return r.Status.AtProvider.ID
		}
		return ""
	}
}

// GitRepositoryID extracts a GitRepository's Azure DevOps GUID from status.atProvider.id.
func GitRepositoryID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		v := reflect.ValueOf(mg)
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return ""
			}
			v = v.Elem()
		}
		status := v.FieldByName("Status")
		if !status.IsValid() {
			return ""
		}
		atProvider := status.FieldByName("AtProvider")
		if !atProvider.IsValid() {
			return ""
		}
		id := atProvider.FieldByName("ID")
		if !id.IsValid() || id.Kind() != reflect.String {
			return ""
		}
		return id.String()
	}
}
