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

// ServiceEndpointKubernetesParameters are the configurable fields of a
// Kubernetes service connection.
// +kubebuilder:validation:XValidation:rule="(has(self.projectId) && self.projectId != \"\") || has(self.projectIdRef) || has(self.projectIdSelector)",message="one of projectId, projectIdRef, or projectIdSelector is required"
// +kubebuilder:validation:XValidation:rule="!has(self.authorizationType) || self.authorizationType != 'Kubeconfig' || has(self.kubeconfigSecretRef)",message="kubeconfigSecretRef is required when authorizationType is Kubeconfig"
// +kubebuilder:validation:XValidation:rule="!has(self.authorizationType) || self.authorizationType != 'ServiceAccount' || has(self.serviceAccountTokenSecretRef)",message="serviceAccountTokenSecretRef is required when authorizationType is ServiceAccount"
// +kubebuilder:validation:XValidation:rule="!has(self.authorizationType) || self.authorizationType != 'ServiceAccount' || (has(self.clusterCACertificate) && self.clusterCACertificate != \"\")",message="clusterCACertificate is required when authorizationType is ServiceAccount"
// +kubebuilder:validation:XValidation:rule="!has(self.authorizationType) || self.authorizationType != 'AzureSubscription'",message="authorizationType AzureSubscription is not yet supported; use Kubeconfig or ServiceAccount"
type ServiceEndpointKubernetesParameters struct {
	// Name of the Kubernetes service connection.
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

	// ClusterServer is the Kubernetes API server URL Azure DevOps should connect to.
	// +kubebuilder:validation:Required
	ClusterServer string `json:"clusterServer"`

	// AuthorizationType selects the Azure DevOps authorization flow used for this
	// Kubernetes service connection.
	// +kubebuilder:validation:Enum=Kubeconfig;ServiceAccount;AzureSubscription
	// +kubebuilder:default=ServiceAccount
	// +optional
	AuthorizationType string `json:"authorizationType,omitempty"`

	// KubeconfigSecretRef references the Kubernetes Secret key containing the
	// kubeconfig content when authorizationType=Kubeconfig. The secret value is
	// resolved at reconcile time and is never persisted to spec or status.
	// +optional
	KubeconfigSecretRef *xpv2.SecretKeySelector `json:"kubeconfigSecretRef,omitempty"`

	// ServiceAccountTokenSecretRef references the Kubernetes Secret key
	// containing the service account bearer token when
	// authorizationType=ServiceAccount. The secret value is resolved at
	// reconcile time and is never persisted to spec or status.
	// +optional
	ServiceAccountTokenSecretRef *xpv2.SecretKeySelector `json:"serviceAccountTokenSecretRef,omitempty"`

	// ClusterCACertificate optionally supplies the cluster CA certificate used by
	// service account-based authentication.
	// +optional
	ClusterCACertificate string `json:"clusterCACertificate,omitempty"`

	// AzureSubscriptionRef is reserved for a future AzureSubscription-backed
	// authorization flow.
	// +optional
	AzureSubscriptionRef *xpv2.NamespacedReference `json:"azureSubscriptionRef,omitempty"`
}

// ServiceEndpointKubernetesObservation are the observable fields of a service endpoint.
type ServiceEndpointKubernetesObservation struct {
	// ID is the Azure DevOps service endpoint GUID.
	ID string `json:"id,omitempty"`

	// IsReady reports whether Azure DevOps considers the endpoint ready for use.
	// +optional
	IsReady bool `json:"isReady,omitempty"`
}

// A ServiceEndpointKubernetesSpec defines the desired state of a ServiceEndpointKubernetes.
type ServiceEndpointKubernetesSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ServiceEndpointKubernetesParameters `json:"forProvider"`
}

// A ServiceEndpointKubernetesStatus represents the observed state of a ServiceEndpointKubernetes.
type ServiceEndpointKubernetesStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ServiceEndpointKubernetesObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A ServiceEndpointKubernetes is a Kubernetes service connection used by Azure
// DevOps pipelines to deploy to Kubernetes clusters.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="ENDPOINT-ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type ServiceEndpointKubernetes struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceEndpointKubernetesSpec   `json:"spec"`
	Status ServiceEndpointKubernetesStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceEndpointKubernetesList contains a list of ServiceEndpointKubernetes.
type ServiceEndpointKubernetesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceEndpointKubernetes `json:"items"`
}

// ServiceEndpointKubernetes type metadata.
var (
	ServiceEndpointKubernetesKind             = reflect.TypeOf(ServiceEndpointKubernetes{}).Name()
	ServiceEndpointKubernetesGroupKind        = schema.GroupKind{Group: Group, Kind: ServiceEndpointKubernetesKind}.String()
	ServiceEndpointKubernetesKindAPIVersion   = ServiceEndpointKubernetesKind + "." + SchemeGroupVersion.String()
	ServiceEndpointKubernetesGroupVersionKind = SchemeGroupVersion.WithKind(ServiceEndpointKubernetesKind)
)

func init() {
	SchemeBuilder.Register(&ServiceEndpointKubernetes{}, &ServiceEndpointKubernetesList{})
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
