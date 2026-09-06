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

// BuildDefinitionParameters are the configurable fields of a BuildDefinition.
type BuildDefinitionParameters struct {
	// Name of the Azure DevOps build definition.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ProjectID is the Azure DevOps project GUID that owns this build definition.
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1.Project
	// +crossplane:generate:reference:extractor=github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1.ProjectID()
	// +optional
	// +kubebuilder:validation:MinLength=36
	// +kubebuilder:validation:MaxLength=36
	ProjectID string `json:"projectId,omitempty"`

	// ProjectIDRef references the Project that owns this build definition.
	// +optional
	ProjectIDRef *xpv2.NamespacedReference `json:"projectIdRef,omitempty"`

	// ProjectIDSelector selects a Project that owns this build definition.
	// +optional
	ProjectIDSelector *xpv2.NamespacedSelector `json:"projectIdSelector,omitempty"`

	// RepositoryID is the Azure DevOps Git repository GUID this build definition uses.
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/gitrepository/v1alpha1.GitRepository
	// +crossplane:generate:reference:extractor=github.com/dunkin0486/provider-azuredevops/apis/gitrepository/v1alpha1.GitRepositoryID()
	// +optional
	// +kubebuilder:validation:MinLength=36
	// +kubebuilder:validation:MaxLength=36
	RepositoryID string `json:"repositoryId,omitempty"`

	// RepositoryIDRef references the GitRepository this build definition uses.
	// +optional
	RepositoryIDRef *xpv2.NamespacedReference `json:"repositoryIdRef,omitempty"`

	// RepositoryIDSelector selects a GitRepository this build definition uses.
	// +optional
	RepositoryIDSelector *xpv2.NamespacedSelector `json:"repositoryIdSelector,omitempty"`

	// YamlPath is the repository-relative path to the pipeline YAML file.
	// +kubebuilder:validation:Required
	YamlPath string `json:"yamlPath"`

	// Path is the Azure DevOps pipeline folder path. Defaults to "\" when omitted.
	// +optional
	Path string `json:"path,omitempty"`

	// VariableGroupIDs are the Azure DevOps variable group integer IDs linked
	// into this build definition.
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/variablegroup/v1alpha1.VariableGroup
	// +crossplane:generate:reference:extractor=github.com/dunkin0486/provider-azuredevops/apis/variablegroup/v1alpha1.VariableGroupID()
	// +crossplane:generate:reference:refFieldName=VariableGroupIDRefs
	// +crossplane:generate:reference:selectorFieldName=VariableGroupIDSelector
	// +optional
	VariableGroupIDs []string `json:"variableGroupIds,omitempty"`

	// VariableGroupIDRefs references the VariableGroups linked into this build definition.
	// +optional
	VariableGroupIDRefs []xpv2.NamespacedReference `json:"variableGroupIdRefs,omitempty"`

	// VariableGroupIDSelector selects VariableGroups linked into this build definition.
	// +optional
	VariableGroupIDSelector *xpv2.NamespacedSelector `json:"variableGroupIdSelector,omitempty"`

	// CIEnabled adds an Azure DevOps continuous integration trigger when true.
	// +optional
	CIEnabled *bool `json:"ciEnabled,omitempty"`

	// PREnabled adds an Azure DevOps pull request trigger when true.
	// +optional
	PREnabled *bool `json:"prEnabled,omitempty"`
}

// BuildDefinitionObservation are the observable fields of a BuildDefinition.
type BuildDefinitionObservation struct {
	// ID is the integer build definition ID assigned by Azure DevOps.
	ID string `json:"id,omitempty"`

	// Revision is the current Azure DevOps revision of the build definition.
	Revision int `json:"revision,omitempty"`

	// URL is the Azure DevOps REST URL of the build definition.
	URL string `json:"url,omitempty"`
}

// A BuildDefinitionSpec defines the desired state of a BuildDefinition.
type BuildDefinitionSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              BuildDefinitionParameters `json:"forProvider"`
}

// A BuildDefinitionStatus represents the observed state of a BuildDefinition.
type BuildDefinitionStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 BuildDefinitionObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A BuildDefinition is an Azure DevOps YAML build definition.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="REVISION",type="integer",JSONPath=".status.atProvider.revision"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type BuildDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuildDefinitionSpec   `json:"spec"`
	Status BuildDefinitionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BuildDefinitionList contains a list of BuildDefinition.
type BuildDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BuildDefinition `json:"items"`
}

// BuildDefinition type metadata.
var (
	BuildDefinitionKind             = reflect.TypeOf(BuildDefinition{}).Name()
	BuildDefinitionGroupKind        = schema.GroupKind{Group: Group, Kind: BuildDefinitionKind}.String()
	BuildDefinitionKindAPIVersion   = BuildDefinitionKind + "." + SchemeGroupVersion.String()
	BuildDefinitionGroupVersionKind = SchemeGroupVersion.WithKind(BuildDefinitionKind)
)

func init() {
	SchemeBuilder.Register(&BuildDefinition{}, &BuildDefinitionList{})
}
