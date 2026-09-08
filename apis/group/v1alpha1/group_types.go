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

// GroupParameters are the configurable fields of a Group.
type GroupParameters struct {
	// DisplayName is the display name of the Azure DevOps group.
	// +kubebuilder:validation:Required
	DisplayName string `json:"displayName"`

	// Description of the Azure DevOps group.
	// +optional
	Description string `json:"description,omitempty"`

	// ScopeDescriptor is the descriptor of the project (or other scope)
	// that this group should be created within. If omitted, the group is
	// created at the organization/collection scope. This field is only
	// used at creation time; Azure DevOps does not return a corresponding
	// value on GET, so it cannot be reconciled after creation.
	// +optional
	// +immutable
	ScopeDescriptor string `json:"scopeDescriptor,omitempty"`
}

// GroupObservation are the observable fields of a Group.
type GroupObservation struct {
	// Descriptor is the unique SID-like identifier for the group used to
	// reference it in other Azure DevOps Graph API calls (e.g. as the
	// containing group of a GroupMembership).
	Descriptor string `json:"descriptor,omitempty"`

	// OriginID is the unique identifier from the origin (AAD, GitHub, etc).
	OriginID string `json:"originId,omitempty"`

	// Origin is the type of source provider for the origin identifier
	// (e.g. "vsts", "aad").
	Origin string `json:"origin,omitempty"`

	// URL is the Azure DevOps REST URL of the group.
	URL string `json:"url,omitempty"`
}

// A GroupSpec defines the desired state of a Group.
type GroupSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              GroupParameters `json:"forProvider"`
}

// A GroupStatus represents the observed state of a Group.
type GroupStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 GroupObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A Group is an Azure DevOps group (a native, Azure DevOps-hosted
// identity used to grant permissions and manage membership).
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="DESCRIPTOR",type="string",JSONPath=".status.atProvider.descriptor"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type Group struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GroupSpec   `json:"spec"`
	Status GroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GroupList contains a list of Group
type GroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Group `json:"items"`
}

// Group type metadata.
var (
	GroupKind             = reflect.TypeOf(Group{}).Name()
	GroupGroupKind        = schema.GroupKind{Group: CRDGroup, Kind: GroupKind}.String()
	GroupKindAPIVersion   = GroupKind + "." + SchemeGroupVersion.String()
	GroupGroupVersionKind = SchemeGroupVersion.WithKind(GroupKind)
)

func init() {
	SchemeBuilder.Register(&Group{}, &GroupList{})
}
