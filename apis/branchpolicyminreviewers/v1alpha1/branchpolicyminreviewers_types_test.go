package v1alpha1

import (
	"context"
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gitrepositoryv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/gitrepository/v1alpha1"
	projectv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1"
)

func branchPolicyScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		SchemeBuilder.AddToScheme,
		projectv1alpha1.SchemeBuilder.AddToScheme,
		gitrepositoryv1alpha1.SchemeBuilder.AddToScheme,
	} {
		if err := add(s); err != nil {
			t.Fatalf("AddToScheme() error = %v", err)
		}
	}
	return s
}

func TestResolveReferences(t *testing.T) {
	projectID := "4d01f0e2-96ce-4ebb-aed9-70a1f96681d5"
	repositoryID := "11111111-1111-1111-1111-111111111111"

	kube := fake.NewClientBuilder().
		WithScheme(branchPolicyScheme(t)).
		WithObjects(
			&projectv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Name: "example-project", Namespace: "default"},
				Status:     projectv1alpha1.ProjectStatus{AtProvider: projectv1alpha1.ProjectObservation{ID: projectID}},
			},
			&gitrepositoryv1alpha1.GitRepository{
				ObjectMeta: metav1.ObjectMeta{Name: "example-repository", Namespace: "default"},
				Status:     gitrepositoryv1alpha1.GitRepositoryStatus{AtProvider: gitrepositoryv1alpha1.GitRepositoryObservation{ID: repositoryID}},
			},
		).
		Build()

	cr := &BranchPolicyMinReviewers{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: BranchPolicyMinReviewersSpec{
			ForProvider: BranchPolicyMinReviewersParameters{
				ProjectIDRef:         &xpv2.NamespacedReference{Name: "example-project"},
				RepositoryIDRef:      &xpv2.NamespacedReference{Name: "example-repository"},
				Branch:               "refs/heads/main",
				Enabled:              true,
				Blocking:             true,
				MinimumApproverCount: 2,
			},
		},
	}

	if err := cr.ResolveReferences(context.Background(), kube); err != nil {
		t.Fatalf("ResolveReferences() error = %v", err)
	}

	if cr.Spec.ForProvider.ProjectID != projectID {
		t.Fatalf("ResolveReferences() projectId = %q, want %q", cr.Spec.ForProvider.ProjectID, projectID)
	}
	if cr.Spec.ForProvider.RepositoryID != repositoryID {
		t.Fatalf("ResolveReferences() repositoryId = %q, want %q", cr.Spec.ForProvider.RepositoryID, repositoryID)
	}
}
