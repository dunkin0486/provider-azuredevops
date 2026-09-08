// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

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

// ServiceEndpointDockerRegistryParameters are the configurable fields of a
// Docker Registry service connection.
// +kubebuilder:validation:XValidation:rule="(has(self.projectId) && self.projectId != \"\") || has(self.projectIdRef) || has(self.projectIdSelector)",message="one of projectId, projectIdRef, or projectIdSelector is required"
type ServiceEndpointDockerRegistryParameters struct {
	// Name of the Docker registry service connection.
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

	// RegistryURL is the Docker/OCI registry URL Azure DevOps should connect to.
	// For example https://index.docker.io/v1/ or https://example.azurecr.io.
	// +kubebuilder:validation:Required
	RegistryURL string `json:"registryUrl"`

	// RegistryType selects the Azure DevOps Docker registry endpoint variant.
	// DockerHub and Others are supported by this resource. Azure Container
	// Registry requires additional Azure-specific fields and is reserved for a
	// future API revision instead of being exposed as a currently valid value.
	// +kubebuilder:validation:Enum=DockerHub;Others
	// +kubebuilder:default=DockerHub
	// +optional
	RegistryType string `json:"registryType,omitempty"`

	// UsernameSecretRef references the Kubernetes Secret key containing the
	// registry username. The secret value is resolved at reconcile time and is
	// never persisted to spec or status.
	// +optional
	UsernameSecretRef *xpv2.SecretKeySelector `json:"usernameSecretRef,omitempty"`

	// PasswordSecretRef references the Kubernetes Secret key containing the
	// registry password. The secret value is resolved at reconcile time and is
	// never persisted to spec or status.
	// +optional
	PasswordSecretRef *xpv2.SecretKeySelector `json:"passwordSecretRef,omitempty"`

	// DockerEmail optionally configures the email metadata associated with this
	// Docker registry login.
	// +optional
	DockerEmail string `json:"dockerEmail,omitempty"`

	// AzureSubscriptionRef is reserved for future Azure Container Registry
	// subscription-based authentication support in a future API revision.
	// +optional
	AzureSubscriptionRef *xpv2.NamespacedReference `json:"azureSubscriptionRef,omitempty"`
}

// ServiceEndpointDockerRegistryObservation are the observable fields of a service endpoint.
type ServiceEndpointDockerRegistryObservation struct {
	// ID is the Azure DevOps service endpoint GUID.
	ID string `json:"id,omitempty"`

	// IsReady reports whether Azure DevOps considers the endpoint ready for use.
	// +optional
	IsReady bool `json:"isReady,omitempty"`
}

// A ServiceEndpointDockerRegistrySpec defines the desired state of a ServiceEndpointDockerRegistry.
type ServiceEndpointDockerRegistrySpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ServiceEndpointDockerRegistryParameters `json:"forProvider"`
}

// A ServiceEndpointDockerRegistryStatus represents the observed state of a ServiceEndpointDockerRegistry.
type ServiceEndpointDockerRegistryStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ServiceEndpointDockerRegistryObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A ServiceEndpointDockerRegistry is a Docker registry service connection used
// by Azure DevOps pipelines to authenticate to container registries.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="REGISTRY-TYPE",type="string",JSONPath=".spec.forProvider.registryType"
// +kubebuilder:printcolumn:name="ENDPOINT-ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type ServiceEndpointDockerRegistry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceEndpointDockerRegistrySpec   `json:"spec"`
	Status ServiceEndpointDockerRegistryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceEndpointDockerRegistryList contains a list of ServiceEndpointDockerRegistry.
type ServiceEndpointDockerRegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceEndpointDockerRegistry `json:"items"`
}

// ServiceEndpointDockerRegistry type metadata.
var (
	ServiceEndpointDockerRegistryKind             = reflect.TypeOf(ServiceEndpointDockerRegistry{}).Name()
	ServiceEndpointDockerRegistryGroupKind        = schema.GroupKind{Group: Group, Kind: ServiceEndpointDockerRegistryKind}.String()
	ServiceEndpointDockerRegistryKindAPIVersion   = ServiceEndpointDockerRegistryKind + "." + SchemeGroupVersion.String()
	ServiceEndpointDockerRegistryGroupVersionKind = SchemeGroupVersion.WithKind(ServiceEndpointDockerRegistryKind)
)

func init() {
	SchemeBuilder.Register(&ServiceEndpointDockerRegistry{}, &ServiceEndpointDockerRegistryList{})
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
