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

// TeamParameters are the configurable fields of a Team.
type TeamParameters struct {
	// ProjectID is the Azure DevOps project GUID that owns this team.
	// Supply it directly or via ProjectIDRef / ProjectIDSelector.
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1.Project
	// +crossplane:generate:reference:extractor=ProjectID()
	// +optional
	// +immutable
	// +kubebuilder:validation:MinLength=36
	// +kubebuilder:validation:MaxLength=36
	ProjectID string `json:"projectId,omitempty"`

	// ProjectIDRef references the Azure DevOps Project whose ID should be used.
	// +optional
	ProjectIDRef *xpv2.NamespacedReference `json:"projectIdRef,omitempty"`

	// ProjectIDSelector selects an Azure DevOps Project whose ID should be used.
	// +optional
	ProjectIDSelector *xpv2.NamespacedSelector `json:"projectIdSelector,omitempty"`

	// Name of the Azure DevOps team.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Description of the Azure DevOps team.
	// +optional
	Description string `json:"description,omitempty"`
}

// TeamObservation are the observable fields of a Team.
type TeamObservation struct {
	// ID is the Azure DevOps team GUID.
	ID string `json:"id,omitempty"`

	// URL is the Azure DevOps REST URL of the team.
	URL string `json:"url,omitempty"`
}

// A TeamSpec defines the desired state of a Team.
type TeamSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              TeamParameters `json:"forProvider"`
}

// A TeamStatus represents the observed state of a Team.
type TeamStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 TeamObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A Team is an Azure DevOps team within a Project, used for default area
// paths, board configuration, and permissions grouping.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type Team struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TeamSpec   `json:"spec"`
	Status TeamStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TeamList contains a list of Team
type TeamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Team `json:"items"`
}

// Team type metadata.
var (
	TeamKind             = reflect.TypeOf(Team{}).Name()
	TeamGroupKind        = schema.GroupKind{Group: Group, Kind: TeamKind}.String()
	TeamKindAPIVersion   = TeamKind + "." + SchemeGroupVersion.String()
	TeamGroupVersionKind = SchemeGroupVersion.WithKind(TeamKind)
)

func init() {
	SchemeBuilder.Register(&Team{}, &TeamList{})
}

// ProjectID extracts a referenced Project's observed Azure DevOps GUID.
func ProjectID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		r, ok := mg.(*projectv1alpha1.Project)
		if !ok {
			return ""
		}
		return r.Status.AtProvider.ID
	}
}
