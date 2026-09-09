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

// ProjectEntitlementParameters describes a project-scoped group assignment
// for a user.
type ProjectEntitlementParameters struct {
	// ProjectID is the Azure DevOps project GUID the user should be
	// assigned to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=36
	// +kubebuilder:validation:MaxLength=36
	// +kubebuilder:validation:Pattern=`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
	ProjectID string `json:"projectId"`

	// GroupType is the well-known project permission group the user should
	// be assigned to within the project (e.g. Contributor, Reader).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=projectStakeholder;projectReader;projectContributor;projectAdministrator;custom
	GroupType string `json:"groupType"`
}

// UserEntitlementParameters are the configurable fields of a UserEntitlement.
type UserEntitlementParameters struct {
	// PrincipalName is the unique name (typically email or UPN) of the user
	// to grant an entitlement to. Immutable once created.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +immutable
	PrincipalName string `json:"principalName"`

	// AccessLevel is the account license type to assign to the user.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=none;earlyAdopter;express;professional;advanced;stakeholder
	AccessLevel string `json:"accessLevel"`

	// ProjectEntitlements assigns the user to one or more projects with a
	// specific permission group.
	// +optional
	ProjectEntitlements []ProjectEntitlementParameters `json:"projectEntitlements,omitempty"`
}

// UserEntitlementObservation are the observable fields of a UserEntitlement.
type UserEntitlementObservation struct {
	// ID is the Azure DevOps user entitlement GUID (matches the user's
	// Identity ID).
	ID string `json:"id,omitempty"`

	// Origin is the type of source provider for the origin identifier
	// (e.g. "aad", "vsts").
	Origin string `json:"origin,omitempty"`

	// OriginID is the unique identifier from the origin (AAD, GitHub, etc).
	OriginID string `json:"originId,omitempty"`

	// LastAccessedDate is the date the member last accessed the
	// collection, in RFC3339 format.
	LastAccessedDate string `json:"lastAccessedDate,omitempty"`
}

// A UserEntitlementSpec defines the desired state of a UserEntitlement.
type UserEntitlementSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              UserEntitlementParameters `json:"forProvider"`
}

// A UserEntitlementStatus represents the observed state of a UserEntitlement.
type UserEntitlementStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 UserEntitlementObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A UserEntitlement grants an Azure DevOps user an account license and,
// optionally, project-scoped group assignments.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type UserEntitlement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserEntitlementSpec   `json:"spec"`
	Status UserEntitlementStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UserEntitlementList contains a list of UserEntitlement
type UserEntitlementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UserEntitlement `json:"items"`
}

// UserEntitlement type metadata.
var (
	UserEntitlementKind             = reflect.TypeOf(UserEntitlement{}).Name()
	UserEntitlementGroupKind        = schema.GroupKind{Group: Group, Kind: UserEntitlementKind}.String()
	UserEntitlementKindAPIVersion   = UserEntitlementKind + "." + SchemeGroupVersion.String()
	UserEntitlementGroupVersionKind = SchemeGroupVersion.WithKind(UserEntitlementKind)
)

func init() {
	SchemeBuilder.Register(&UserEntitlement{}, &UserEntitlementList{})
}
