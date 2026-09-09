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

	agentpoolv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/agentpool/v1alpha1"
	projectv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1"
)

// AgentQueueParameters are the configurable fields of an AgentQueue.
type AgentQueueParameters struct {
	// Name of the agent queue. Defaults to the referenced AgentPool's name
	// when omitted, which is the Azure DevOps convention for queues that
	// link a project 1:1 to a pool.
	// +optional
	Name string `json:"name,omitempty"`

	// ProjectID is the Azure DevOps project GUID that this queue belongs to.
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

	// AgentPoolID is the organization-level AgentPool ID to link this
	// project queue to. Supply it directly or via AgentPoolIDRef /
	// AgentPoolIDSelector.
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/agentpool/v1alpha1.AgentPool
	// +crossplane:generate:reference:extractor=AgentPoolID()
	// +optional
	// +immutable
	AgentPoolID string `json:"agentPoolId,omitempty"`

	// AgentPoolIDRef references the AgentPool whose ID should be used.
	// +optional
	AgentPoolIDRef *xpv2.NamespacedReference `json:"agentPoolIdRef,omitempty"`

	// AgentPoolIDSelector selects an AgentPool whose ID should be used.
	// +optional
	AgentPoolIDSelector *xpv2.NamespacedSelector `json:"agentPoolIdSelector,omitempty"`

	// AuthorizePipelines automatically authorizes this queue for use by all
	// YAML pipelines in the project when set.
	// +optional
	AuthorizePipelines *bool `json:"authorizePipelines,omitempty"`
}

// AgentQueueObservation are the observable fields of an AgentQueue.
type AgentQueueObservation struct {
	// ID is the integer agent queue ID assigned by Azure DevOps.
	ID string `json:"id,omitempty"`

	// Name is the queue name as reported by Azure DevOps.
	Name string `json:"name,omitempty"`

	// PoolID is the integer agent pool ID this queue is linked to.
	PoolID string `json:"poolId,omitempty"`
}

// An AgentQueueSpec defines the desired state of an AgentQueue.
type AgentQueueSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              AgentQueueParameters `json:"forProvider"`
}

// An AgentQueueStatus represents the observed state of an AgentQueue.
type AgentQueueStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 AgentQueueObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// An AgentQueue links an Azure DevOps Project to an organization-level
// AgentPool so that pipelines in that project can use the pool's agents.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="QUEUE-ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type AgentQueue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentQueueSpec   `json:"spec"`
	Status AgentQueueStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentQueueList contains a list of AgentQueue.
type AgentQueueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentQueue `json:"items"`
}

// AgentQueue type metadata.
var (
	AgentQueueKind             = reflect.TypeOf(AgentQueue{}).Name()
	AgentQueueGroupKind        = schema.GroupKind{Group: Group, Kind: AgentQueueKind}.String()
	AgentQueueKindAPIVersion   = AgentQueueKind + "." + SchemeGroupVersion.String()
	AgentQueueGroupVersionKind = SchemeGroupVersion.WithKind(AgentQueueKind)
)

func init() {
	SchemeBuilder.Register(&AgentQueue{}, &AgentQueueList{})
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

// AgentPoolID extracts a referenced AgentPool's observed integer ID.
func AgentPoolID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		r, ok := mg.(*agentpoolv1alpha1.AgentPool)
		if !ok {
			return ""
		}
		return r.Status.AtProvider.ID
	}
}
