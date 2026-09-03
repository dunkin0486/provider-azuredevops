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

// ServiceEndpointAzureRMCredentials configures exactly one supported
// Azure RM authorization scheme.
type ServiceEndpointAzureRMCredentials struct {
	// ServicePrincipal configures classic service principal secret auth.
	// +optional
	ServicePrincipal *ServicePrincipalCredentials `json:"servicePrincipal,omitempty"`

	// WorkloadIdentityFederation configures OIDC-based authentication.
	// +optional
	WorkloadIdentityFederation *WorkloadIdentityFederationCredentials `json:"workloadIdentityFederation,omitempty"`
}

// ServicePrincipalCredentials configures service principal secret auth.
type ServicePrincipalCredentials struct {
	// ClientID is the Microsoft Entra application or service principal client ID.
	// +kubebuilder:validation:Required
	ClientID string `json:"clientId"`

	// ClientSecretRef references the Kubernetes Secret key containing the
	// service principal client secret. The secret value is resolved at
	// reconcile time and is never persisted to spec or status.
	// +kubebuilder:validation:Required
	ClientSecretRef xpv2.SecretKeySelector `json:"clientSecretRef"`
}

// WorkloadIdentityFederationCredentials configures OIDC-based auth.
type WorkloadIdentityFederationCredentials struct {
	// ClientID is the Microsoft Entra application or managed identity client ID.
	// +kubebuilder:validation:Required
	ClientID string `json:"clientId"`
}

// ServiceEndpointAzureRMParameters are the configurable fields of an Azure
// Resource Manager service connection.
type ServiceEndpointAzureRMParameters struct {
	// Name of the Azure Resource Manager service connection.
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

	// AzureSubscriptionID is the target Azure subscription GUID.
	// +kubebuilder:validation:Required
	AzureSubscriptionID string `json:"azureSubscriptionId"`

	// AzureSubscriptionName is the human-readable Azure subscription name.
	// +kubebuilder:validation:Required
	AzureSubscriptionName string `json:"azureSubscriptionName"`

	// AzureTenantID is the Microsoft Entra tenant GUID.
	// +kubebuilder:validation:Required
	AzureTenantID string `json:"azureTenantId"`

	// ResourceGroup optionally scopes this service connection to one Azure resource group.
	// +optional
	ResourceGroup string `json:"resourceGroup,omitempty"`

	// Credentials configures the Azure authentication scheme used by this service connection.
	// +kubebuilder:validation:Required
	Credentials ServiceEndpointAzureRMCredentials `json:"credentials"`
}

// ServiceEndpointAzureRMObservation are the observable fields of a service endpoint.
type ServiceEndpointAzureRMObservation struct {
	// ID is the Azure DevOps service endpoint GUID.
	ID string `json:"id,omitempty"`

	// IsReady reports whether Azure DevOps considers the endpoint ready for use.
	// +optional
	IsReady bool `json:"isReady,omitempty"`

	// AuthorizationScheme is the active Azure DevOps authorization scheme.
	AuthorizationScheme string `json:"authorizationScheme,omitempty"`
}

// A ServiceEndpointAzureRMSpec defines the desired state of a ServiceEndpointAzureRM.
type ServiceEndpointAzureRMSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ServiceEndpointAzureRMParameters `json:"forProvider"`
}

// A ServiceEndpointAzureRMStatus represents the observed state of a ServiceEndpointAzureRM.
type ServiceEndpointAzureRMStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ServiceEndpointAzureRMObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A ServiceEndpointAzureRM is an Azure Resource Manager service connection
// used by Azure DevOps pipelines to authenticate to Azure subscriptions.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="AUTH-SCHEME",type="string",JSONPath=".status.atProvider.authorizationScheme"
// +kubebuilder:printcolumn:name="ENDPOINT-ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type ServiceEndpointAzureRM struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceEndpointAzureRMSpec   `json:"spec"`
	Status ServiceEndpointAzureRMStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceEndpointAzureRMList contains a list of ServiceEndpointAzureRM.
type ServiceEndpointAzureRMList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceEndpointAzureRM `json:"items"`
}

// ServiceEndpointAzureRM type metadata.
var (
	ServiceEndpointAzureRMKind             = reflect.TypeOf(ServiceEndpointAzureRM{}).Name()
	ServiceEndpointAzureRMGroupKind        = schema.GroupKind{Group: Group, Kind: ServiceEndpointAzureRMKind}.String()
	ServiceEndpointAzureRMKindAPIVersion   = ServiceEndpointAzureRMKind + "." + SchemeGroupVersion.String()
	ServiceEndpointAzureRMGroupVersionKind = SchemeGroupVersion.WithKind(ServiceEndpointAzureRMKind)
)

func init() {
	SchemeBuilder.Register(&ServiceEndpointAzureRM{}, &ServiceEndpointAzureRMList{})
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
