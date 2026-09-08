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

// EnvironmentParameters are the configurable fields of an Environment.
// +kubebuilder:validation:XValidation:rule="(has(self.projectId) && self.projectId != \"\") || has(self.projectIdRef) || has(self.projectIdSelector)",message="one of projectId, projectIdRef, or projectIdSelector is required"
type EnvironmentParameters struct {
	// ProjectID is the Azure DevOps project GUID that owns this environment.
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
	// +immutable
	ProjectIDRef *xpv2.NamespacedReference `json:"projectIdRef,omitempty"`

	// ProjectIDSelector selects the Azure DevOps Project whose ID should be used.
	// +optional
	// +immutable
	ProjectIDSelector *xpv2.NamespacedSelector `json:"projectIdSelector,omitempty"`

	// Name of the Azure DevOps environment.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Description of the Azure DevOps environment.
	// +optional
	Description string `json:"description,omitempty"`
}

// EnvironmentObservation are the observable fields of an Environment.
type EnvironmentObservation struct {
	// ID is the integer environment ID assigned by Azure DevOps.
	ID string `json:"id,omitempty"`

	// CreatedBy is the display name or unique name of the identity that created the environment.
	CreatedBy string `json:"createdBy,omitempty"`

	// CreatedOn is the time the environment was created in Azure DevOps.
	// +optional
	CreatedOn *metav1.Time `json:"createdOn,omitempty"`
}

// An EnvironmentSpec defines the desired state of an Environment.
type EnvironmentSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              EnvironmentParameters `json:"forProvider"`
}

// An EnvironmentStatus represents the observed state of an Environment.
type EnvironmentStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 EnvironmentObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// An Environment is an Azure DevOps Pipelines environment used as a deployment target and gate.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="ENVIRONMENT-ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type Environment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvironmentSpec   `json:"spec"`
	Status EnvironmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EnvironmentList contains a list of Environment.
type EnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Environment `json:"items"`
}

// Environment type metadata.
var (
	EnvironmentKind             = reflect.TypeOf(Environment{}).Name()
	EnvironmentGroupKind        = schema.GroupKind{Group: Group, Kind: EnvironmentKind}.String()
	EnvironmentKindAPIVersion   = EnvironmentKind + "." + SchemeGroupVersion.String()
	EnvironmentGroupVersionKind = SchemeGroupVersion.WithKind(EnvironmentKind)
)

func init() {
	SchemeBuilder.Register(&Environment{}, &EnvironmentList{})
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
