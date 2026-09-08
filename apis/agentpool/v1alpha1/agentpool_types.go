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

// AgentPoolParameters are the configurable fields of an AgentPool.
type AgentPoolParameters struct {
	// Name is the Azure DevOps agent pool name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// AutoProvision controls whether a queue is automatically provisioned for each project collection.
	// +optional
	AutoProvision *bool `json:"autoProvision,omitempty"`

	// AutoUpdate controls whether agents in this pool may automatically update.
	// +optional
	AutoUpdate *bool `json:"autoUpdate,omitempty"`

	// IsHosted indicates whether the agent pool is service-managed.
	// This provider manages self-hosted pools by default.
	// +optional
	// +kubebuilder:default=false
	IsHosted *bool `json:"isHosted,omitempty"`
}

// AgentPoolObservation are the observable fields of an AgentPool.
type AgentPoolObservation struct {
	// ID is the integer agent pool ID assigned by Azure DevOps.
	ID string `json:"id,omitempty"`

	// Size is the number of agents currently registered in the pool.
	Size int `json:"size,omitempty"`
}

// An AgentPoolSpec defines the desired state of an AgentPool.
type AgentPoolSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              AgentPoolParameters `json:"forProvider"`
}

// An AgentPoolStatus represents the observed state of an AgentPool.
type AgentPoolStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 AgentPoolObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// An AgentPool is an organization-level Azure DevOps self-hosted agent pool.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="POOL-ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="SIZE",type="integer",JSONPath=".status.atProvider.size"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type AgentPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentPoolSpec   `json:"spec"`
	Status AgentPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentPoolList contains a list of AgentPool.
type AgentPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentPool `json:"items"`
}

// AgentPool type metadata.
var (
	AgentPoolKind             = reflect.TypeOf(AgentPool{}).Name()
	AgentPoolGroupKind        = schema.GroupKind{Group: Group, Kind: AgentPoolKind}.String()
	AgentPoolKindAPIVersion   = AgentPoolKind + "." + SchemeGroupVersion.String()
	AgentPoolGroupVersionKind = SchemeGroupVersion.WithKind(AgentPoolKind)
)

func init() {
	SchemeBuilder.Register(&AgentPool{}, &AgentPoolList{})
}
