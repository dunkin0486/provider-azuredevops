package v1alpha1

import (
	"context"
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	projectv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	if err := projectv1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("project scheme: %v", err)
	}
	if err := SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("serviceendpointgithub scheme: %v", err)
	}
	return s
}

func TestResolveReferences(t *testing.T) {
	projectID := "4d01f0e2-96ce-4ebb-aed9-70a1f96681d5"
	project := &projectv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "example-project", Namespace: "default", Labels: map[string]string{"test": "true"}},
		Status: projectv1alpha1.ProjectStatus{
			AtProvider: projectv1alpha1.ProjectObservation{ID: projectID},
		},
	}

	t.Run("Reference", func(t *testing.T) {
		kube := fakeclient.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(project).Build()
		cr := &ServiceEndpointGitHub{
			ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
			Spec: ServiceEndpointGitHubSpec{
				ForProvider: ServiceEndpointGitHubParameters{
					ProjectIDRef: &xpv2.NamespacedReference{Name: "example-project"},
				},
			},
		}

		if err := cr.ResolveReferences(context.Background(), kube); err != nil {
			t.Fatalf("ResolveReferences() error = %v", err)
		}
		if got, want := cr.Spec.ForProvider.ProjectID, projectID; got != want {
			t.Fatalf("ProjectID = %q, want %q", got, want)
		}
		if cr.Spec.ForProvider.ProjectIDRef == nil || cr.Spec.ForProvider.ProjectIDRef.Name != "example-project" {
			t.Fatalf("ProjectIDRef not preserved: %#v", cr.Spec.ForProvider.ProjectIDRef)
		}
	})

	t.Run("Selector", func(t *testing.T) {
		kube := fakeclient.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(project).Build()
		cr := &ServiceEndpointGitHub{
			ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
			Spec: ServiceEndpointGitHubSpec{
				ForProvider: ServiceEndpointGitHubParameters{
					ProjectIDSelector: &xpv2.NamespacedSelector{MatchLabels: map[string]string{"test": "true"}},
				},
			},
		}

		if err := cr.ResolveReferences(context.Background(), kube); err != nil {
			t.Fatalf("ResolveReferences() error = %v", err)
		}
		if got, want := cr.Spec.ForProvider.ProjectID, projectID; got != want {
			t.Fatalf("ProjectID = %q, want %q", got, want)
		}
		if cr.Spec.ForProvider.ProjectIDRef == nil || cr.Spec.ForProvider.ProjectIDRef.Name != "example-project" || cr.Spec.ForProvider.ProjectIDRef.Namespace != "default" {
			t.Fatalf("ProjectIDRef not resolved from selector: %#v", cr.Spec.ForProvider.ProjectIDRef)
		}
	})
}
