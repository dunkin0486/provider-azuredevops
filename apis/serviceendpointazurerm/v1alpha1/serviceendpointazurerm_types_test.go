package v1alpha1

import (
	"context"
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	projectv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	if err := SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("SchemeBuilder.AddToScheme() error = %v", err)
	}
	if err := projectv1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("project SchemeBuilder.AddToScheme() error = %v", err)
	}
	return s
}

func TestResolveReferences(t *testing.T) {
	projectID := "4d01f0e2-96ce-4ebb-aed9-70a1f96681d5"
	project := &projectv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "example-project", Namespace: "default"},
		Status: projectv1alpha1.ProjectStatus{
			AtProvider: projectv1alpha1.ProjectObservation{ID: projectID},
		},
	}

	kube := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(project).
		Build()

	cr := &ServiceEndpointAzureRM{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: ServiceEndpointAzureRMSpec{
			ForProvider: ServiceEndpointAzureRMParameters{
				ProjectIDRef: &xpv2.Reference{Name: "example-project"},
			},
		},
	}

	if err := cr.ResolveReferences(context.Background(), kube); err != nil {
		t.Fatalf("ResolveReferences() error = %v", err)
	}

	if cr.Spec.ForProvider.ProjectID != projectID {
		t.Fatalf("ResolveReferences() projectId = %q, want %q", cr.Spec.ForProvider.ProjectID, projectID)
	}
	if cr.Spec.ForProvider.ProjectIDRef == nil || cr.Spec.ForProvider.ProjectIDRef.Name != "example-project" {
		t.Fatalf("ResolveReferences() projectIdRef = %#v, want name example-project", cr.Spec.ForProvider.ProjectIDRef)
	}
}
