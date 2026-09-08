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

	groupv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/group/v1alpha1"
)

// GroupMembershipParameters are the configurable fields of a GroupMembership.
// +kubebuilder:validation:XValidation:rule="(has(self.groupDescriptor) && self.groupDescriptor != \"\") || has(self.groupDescriptorRef) || has(self.groupDescriptorSelector)",message="one of groupDescriptor, groupDescriptorRef, or groupDescriptorSelector is required"
type GroupMembershipParameters struct {
	// MemberDescriptor identifies the user, group, or service principal to
	// add as a member of the target Azure DevOps graph group. Immutable once
	// created.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +immutable
	MemberDescriptor string `json:"memberDescriptor"`

	// GroupDescriptor identifies the target Azure DevOps graph group that
	// should contain the member. Supply it directly or via
	// GroupDescriptorRef / GroupDescriptorSelector to reference a Group
	// managed resource's observed descriptor. Immutable once created.
	// +kubebuilder:validation:MinLength=1
	// +optional
	// +immutable
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/group/v1alpha1.Group
	// +crossplane:generate:reference:extractor=GroupDescriptor()
	GroupDescriptor string `json:"groupDescriptor,omitempty"`

	// GroupDescriptorRef references the Group whose descriptor should be used.
	// +optional
	GroupDescriptorRef *xpv2.NamespacedReference `json:"groupDescriptorRef,omitempty"`

	// GroupDescriptorSelector selects a Group whose descriptor should be used.
	// +optional
	GroupDescriptorSelector *xpv2.NamespacedSelector `json:"groupDescriptorSelector,omitempty"`

	// Mode controls how the membership is reconciled. Azure DevOps' Graph
	// Memberships API currently supports idempotent add/remove semantics for a
	// single membership edge; therefore overwrite currently behaves the same
	// as add and is reserved for a future enhancement. Immutable once created.
	// +optional
	// +kubebuilder:validation:Enum=add;overwrite
	// +kubebuilder:default=add
	// +immutable
	Mode string `json:"mode,omitempty"`
}

// GroupMembershipObservation are the observable fields of a GroupMembership.
type GroupMembershipObservation struct {
	// Active reports whether Azure DevOps currently considers the member
	// subject active.
	Active bool `json:"active"`
}

// A GroupMembershipSpec defines the desired state of a GroupMembership.
type GroupMembershipSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              GroupMembershipParameters `json:"forProvider"`
}

// A GroupMembershipStatus represents the observed state of a GroupMembership.
type GroupMembershipStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 GroupMembershipObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A GroupMembership manages membership of a graph subject within an Azure
// DevOps graph group.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="ACTIVE",type="boolean",JSONPath=".status.atProvider.active"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type GroupMembership struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GroupMembershipSpec   `json:"spec"`
	Status GroupMembershipStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GroupMembershipList contains a list of GroupMembership.
type GroupMembershipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GroupMembership `json:"items"`
}

// GroupMembership type metadata.
var (
	GroupMembershipKind             = reflect.TypeOf(GroupMembership{}).Name()
	GroupMembershipGroupKind        = schema.GroupKind{Group: Group, Kind: GroupMembershipKind}.String()
	GroupMembershipKindAPIVersion   = GroupMembershipKind + "." + SchemeGroupVersion.String()
	GroupMembershipGroupVersionKind = SchemeGroupVersion.WithKind(GroupMembershipKind)
)

func init() {
	SchemeBuilder.Register(&GroupMembership{}, &GroupMembershipList{})
}

// GroupDescriptor extracts a referenced Group's observed Azure DevOps graph
// descriptor.
func GroupDescriptor() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		r, ok := mg.(*groupv1alpha1.Group)
		if !ok {
			return ""
		}
		return r.Status.AtProvider.Descriptor
	}
}
