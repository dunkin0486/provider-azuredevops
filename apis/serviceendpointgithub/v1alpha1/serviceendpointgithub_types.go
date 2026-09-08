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

// ServiceEndpointGitHubParameters are the configurable fields of a GitHub
// service connection.
// +kubebuilder:validation:XValidation:rule="(has(self.projectId) && self.projectId != \"\") || has(self.projectIdRef) || has(self.projectIdSelector)",message="one of projectId, projectIdRef, or projectIdSelector is required"
type ServiceEndpointGitHubParameters struct {
	// Name of the GitHub service connection.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ProjectID is the Azure DevOps project GUID that owns this service endpoint.
	// Supply it directly or via ProjectIDRef / ProjectIDSelector.
	// +crossplane:generate:reference:type=github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1.Project
	// +crossplane:generate:reference:extractor=ProjectID()
	// +optional
	// +immutable
	ProjectID string `json:"projectId,omitempty"`

	// ProjectIDRef references the Azure DevOps Project whose ID should be used.
	// +optional
	ProjectIDRef *xpv2.NamespacedReference `json:"projectIdRef,omitempty"`

	// ProjectIDSelector selects an Azure DevOps Project whose ID should be used.
	// +optional
	ProjectIDSelector *xpv2.NamespacedSelector `json:"projectIdSelector,omitempty"`

	// AuthScheme selects the Azure DevOps authorization scheme used for this
	// GitHub service connection.
	// +kubebuilder:validation:Enum=PersonalAccessToken;OAuth;InstallationToken
	// +kubebuilder:default=PersonalAccessToken
	// +optional
	AuthScheme string `json:"authScheme,omitempty"`

	// TokenSecretRef references the Kubernetes Secret key containing the token
	// used for token-backed authentication flows. The secret value is resolved
	// at reconcile time and is never persisted to spec or status.
	// +optional
	TokenSecretRef *xpv2.SecretKeySelector `json:"tokenSecretRef,omitempty"`
}

// ServiceEndpointGitHubObservation are the observable fields of a service endpoint.
type ServiceEndpointGitHubObservation struct {
	// ID is the Azure DevOps service endpoint GUID.
	ID string `json:"id,omitempty"`

	// IsReady reports whether Azure DevOps considers the endpoint ready for use.
	// +optional
	IsReady bool `json:"isReady,omitempty"`

	// AuthorizationScheme is the active Azure DevOps authorization scheme.
	AuthorizationScheme string `json:"authorizationScheme,omitempty"`
}

// A ServiceEndpointGitHubSpec defines the desired state of a ServiceEndpointGitHub.
type ServiceEndpointGitHubSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ServiceEndpointGitHubParameters `json:"forProvider"`
}

// A ServiceEndpointGitHubStatus represents the observed state of a ServiceEndpointGitHub.
type ServiceEndpointGitHubStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ServiceEndpointGitHubObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A ServiceEndpointGitHub is a GitHub service connection used by Azure DevOps
// pipelines to access GitHub repositories and related APIs.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="AUTH-SCHEME",type="string",JSONPath=".status.atProvider.authorizationScheme"
// +kubebuilder:printcolumn:name="ENDPOINT-ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type ServiceEndpointGitHub struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceEndpointGitHubSpec   `json:"spec"`
	Status ServiceEndpointGitHubStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceEndpointGitHubList contains a list of ServiceEndpointGitHub.
type ServiceEndpointGitHubList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceEndpointGitHub `json:"items"`
}

// ServiceEndpointGitHub type metadata.
var (
	ServiceEndpointGitHubKind             = reflect.TypeOf(ServiceEndpointGitHub{}).Name()
	ServiceEndpointGitHubGroupKind        = schema.GroupKind{Group: Group, Kind: ServiceEndpointGitHubKind}.String()
	ServiceEndpointGitHubKindAPIVersion   = ServiceEndpointGitHubKind + "." + SchemeGroupVersion.String()
	ServiceEndpointGitHubGroupVersionKind = SchemeGroupVersion.WithKind(ServiceEndpointGitHubKind)
)

func init() {
	SchemeBuilder.Register(&ServiceEndpointGitHub{}, &ServiceEndpointGitHubList{})
}

// ProjectID extracts a Project's Azure DevOps GUID from status.atProvider.id.
// The generated ResolveReferences (in zz_generated.resolvers.go) uses this as
// its extractor for the ProjectID field.
func ProjectID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		if r, ok := mg.(*projectv1alpha1.Project); ok {
			return r.Status.AtProvider.ID
		}
		return ""
	}
}
