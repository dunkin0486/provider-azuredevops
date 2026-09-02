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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
)

// ProjectParameters are the configurable fields of a Project.
type ProjectParameters struct {
	// Name of the Azure DevOps project. Immutable once created.
	// +kubebuilder:validation:Required
	// +immutable
	Name string `json:"name"`

	// Description of the project.
	// +optional
	Description string `json:"description,omitempty"`

	// Visibility of the project.
	// +optional
	// +kubebuilder:validation:Enum=private;public
	// +kubebuilder:default=private
	Visibility string `json:"visibility,omitempty"`

	// VersionControl system used by the project. Immutable once created.
	// +optional
	// +kubebuilder:validation:Enum=Git;Tfvc
	// +kubebuilder:default=Git
	// +immutable
	VersionControl string `json:"versionControl,omitempty"`

	// WorkItemTemplate is the process template used to create the project,
	// e.g. "Agile", "Scrum", or "Basic". Immutable once created.
	// +optional
	// +kubebuilder:default=Agile
	// +immutable
	WorkItemTemplate string `json:"workItemTemplate,omitempty"`
}

// ProjectObservation are the observable fields of a Project.
type ProjectObservation struct {
	// ID is the project GUID assigned by Azure DevOps.
	ID string `json:"id,omitempty"`

	// State is the current lifecycle state of the project as reported by
	// Azure DevOps, e.g. "wellFormed", "createPending", or "deleting".
	State string `json:"state,omitempty"`

	// Revision is the project's revision number.
	Revision int64 `json:"revision,omitempty"`

	// URL is the fully qualified link to the project resource.
	URL string `json:"url,omitempty"`
}

// A ProjectSpec defines the desired state of a Project.
type ProjectSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ProjectParameters `json:"forProvider"`
}

// A ProjectStatus represents the observed state of a Project.
type ProjectStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ProjectObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A Project is an Azure DevOps project, the container for repos, pipelines,
// boards, and all other resources -- nearly every other managed resource in
// this provider references a Project via projectId/projectRef.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="STATE",type="string",JSONPath=".status.atProvider.state"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec   `json:"spec"`
	Status ProjectStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Project `json:"items"`
}

// Project type metadata.
var (
	ProjectKind             = reflect.TypeOf(Project{}).Name()
	ProjectGroupKind        = schema.GroupKind{Group: Group, Kind: ProjectKind}.String()
	ProjectKindAPIVersion   = ProjectKind + "." + SchemeGroupVersion.String()
	ProjectGroupVersionKind = SchemeGroupVersion.WithKind(ProjectKind)
)

func init() {
	SchemeBuilder.Register(&Project{}, &ProjectList{})
}
