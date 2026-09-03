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
	"context"
	"reflect"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reference"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	projectv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1"
)

const errResolveProjectID = "cannot resolve spec.forProvider.projectId"

// VariableValueSource identifies where a variable's value should be read from.
type VariableValueSource struct {
	// SecretKeyRef references a Kubernetes Secret key containing the variable value.
	SecretKeyRef xpv2.SecretKeySelector `json:"secretKeyRef"`
}

// VariableGroupVariable configures one entry in an Azure DevOps variable group.
type VariableGroupVariable struct {
	// Name of the variable.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Value is the plaintext value. Mutually exclusive with ValueFrom.
	// +optional
	Value string `json:"value,omitempty"`

	// IsSecret marks this variable as a secret in Azure DevOps.
	// +optional
	IsSecret bool `json:"isSecret,omitempty"`

	// ValueFrom sources the value from a Kubernetes Secret. Required when
	// IsSecret is true; never store secret values in Value.
	// +optional
	ValueFrom *VariableValueSource `json:"valueFrom,omitempty"`
}

// KeyVaultReference configures an Azure Key Vault-linked variable group.
type KeyVaultReference struct {
	// ServiceEndpointID is the Azure DevOps service endpoint ID that grants
	// access to the Azure Key Vault.
	// +kubebuilder:validation:Required
	ServiceEndpointID string `json:"serviceEndpointId"`

	// Name is the Azure Key Vault name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// VariableGroupParameters are the configurable fields of a VariableGroup.
type VariableGroupParameters struct {
	// ProjectID is the Azure DevOps project UUID that scopes this variable group.
	// +optional
	ProjectID string `json:"projectId,omitempty"`

	// ProjectIDRef references the Azure DevOps Project resource whose observed ID
	// should be used as this variable group's projectId.
	// +optional
	ProjectIDRef *xpv2.Reference `json:"projectIdRef,omitempty"`

	// ProjectIDSelector selects an Azure DevOps Project resource whose observed ID
	// should be used as this variable group's projectId.
	// +optional
	ProjectIDSelector *xpv2.Selector `json:"projectIdSelector,omitempty"`

	// Name of the Azure DevOps variable group.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Description of the variable group.
	// +optional
	Description string `json:"description,omitempty"`

	// Variables contained in this variable group.
	// +optional
	// +listType=map
	// +listMapKey=name
	Variables []VariableGroupVariable `json:"variables,omitempty"`

	// KeyVault links this variable group to Azure Key Vault instead of storing
	// variables directly in Azure DevOps.
	// +optional
	KeyVault *KeyVaultReference `json:"keyVault,omitempty"`
}

// VariableGroupObservation are the observable fields of a VariableGroup.
type VariableGroupObservation struct {
	// ID is the integer variable group ID assigned by Azure DevOps.
	ID string `json:"id,omitempty"`
}

// A VariableGroupSpec defines the desired state of a VariableGroup.
type VariableGroupSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              VariableGroupParameters `json:"forProvider"`
}

// A VariableGroupStatus represents the observed state of a VariableGroup.
type VariableGroupStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 VariableGroupObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A VariableGroup is an Azure DevOps pipeline variable group.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,azuredevops}
type VariableGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VariableGroupSpec   `json:"spec"`
	Status VariableGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VariableGroupList contains a list of VariableGroup.
type VariableGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VariableGroup `json:"items"`
}

// VariableGroup type metadata.
var (
	VariableGroupKind             = reflect.TypeOf(VariableGroup{}).Name()
	VariableGroupGroupKind        = schema.GroupKind{Group: Group, Kind: VariableGroupKind}.String()
	VariableGroupKindAPIVersion   = VariableGroupKind + "." + SchemeGroupVersion.String()
	VariableGroupGroupVersionKind = SchemeGroupVersion.WithKind(VariableGroupKind)
)

func init() {
	SchemeBuilder.Register(&VariableGroup{}, &VariableGroupList{})
}

// ResolveReferences of this VariableGroup.
func (mg *VariableGroup) ResolveReferences(ctx context.Context, c client.Reader) error {
	r := reference.NewAPIResolver(c, any(mg).(resource.Managed))

	rsp, err := r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.ForProvider.ProjectID,
		Reference:    mg.Spec.ForProvider.ProjectIDRef,
		Selector:     mg.Spec.ForProvider.ProjectIDSelector,
		To: reference.To{
			Managed: &projectv1alpha1.Project{},
			List:    &projectv1alpha1.ProjectList{},
		},
		Extract: ExtractProjectID(),
	})
	if err != nil {
		return errors.Wrap(err, errResolveProjectID)
	}

	mg.Spec.ForProvider.ProjectID = rsp.ResolvedValue
	mg.Spec.ForProvider.ProjectIDRef = rsp.ResolvedReference
	return nil
}

// ExtractProjectID extracts a referenced Project's observed Azure DevOps GUID.
func ExtractProjectID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		r, ok := mg.(*projectv1alpha1.Project)
		if !ok {
			return ""
		}
		return r.Status.AtProvider.ID
	}
}
