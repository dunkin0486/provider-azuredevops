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

// ServiceEndpointGenericParameters are the configurable fields of a generic
// Azure DevOps service connection.
type ServiceEndpointGenericParameters struct {
	// Name of the generic service connection.
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

	// ServerURL is the external service URL Azure DevOps should connect to.
	// +kubebuilder:validation:Required
	ServerURL string `json:"serverUrl"`

	// Username optionally configures the username sent when
	// authorizationScheme is UsernamePassword.
	// +optional
	Username string `json:"username,omitempty"`

	// PasswordSecretRef references the Kubernetes Secret key containing the
	// password when authorizationScheme=UsernamePassword, or the token when
	// authorizationScheme=Token. The secret value is resolved at reconcile
	// time and is never persisted to spec or status.
	// +optional
	PasswordSecretRef *xpv2.SecretKeySelector `json:"passwordSecretRef,omitempty"`

	// AuthorizationScheme is the Azure DevOps service endpoint authorization scheme.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=UsernamePassword;Token;None
	AuthorizationScheme string `json:"authorizationScheme"`
}

// ServiceEndpointGenericObservation are the observable fields of a service endpoint.
type ServiceEndpointGenericObservation struct {
	// ID is the Azure DevOps service endpoint GUID.
	ID string `json:"id,omitempty"`

	// IsReady reports whether Azure DevOps considers the endpoint ready for use.
	// +optional
	IsReady bool `json:"isReady,omitempty"`
}

// A ServiceEndpointGenericSpec defines the desired state of a ServiceEndpointGeneric.
type ServiceEndpointGenericSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ServiceEndpointGenericParameters `json:"forProvider"`
}

// A ServiceEndpointGenericStatus represents the observed state of a ServiceEndpointGeneric.
type ServiceEndpointGenericStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ServiceEndpointGenericObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A ServiceEndpointGeneric is a generic Azure DevOps service connection used
// for external services that do not have a dedicated endpoint type.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="ENDPOINT-ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type ServiceEndpointGeneric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceEndpointGenericSpec   `json:"spec"`
	Status ServiceEndpointGenericStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceEndpointGenericList contains a list of ServiceEndpointGeneric.
type ServiceEndpointGenericList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceEndpointGeneric `json:"items"`
}

// ServiceEndpointGeneric type metadata.
var (
	ServiceEndpointGenericKind             = reflect.TypeOf(ServiceEndpointGeneric{}).Name()
	ServiceEndpointGenericGroupKind        = schema.GroupKind{Group: Group, Kind: ServiceEndpointGenericKind}.String()
	ServiceEndpointGenericKindAPIVersion   = ServiceEndpointGenericKind + "." + SchemeGroupVersion.String()
	ServiceEndpointGenericGroupVersionKind = SchemeGroupVersion.WithKind(ServiceEndpointGenericKind)
)

func init() {
	SchemeBuilder.Register(&ServiceEndpointGeneric{}, &ServiceEndpointGenericList{})
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
