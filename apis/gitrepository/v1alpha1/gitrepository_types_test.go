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
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	projectv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1"
)

const testNamespace = "default"

func TestResolveReferences(t *testing.T) {
	s := runtime.NewScheme()
	if err := projectv1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("project scheme: %v", err)
	}
	if err := SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("gitrepository scheme: %v", err)
	}

	project := &projectv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "example-project", Namespace: testNamespace, Labels: map[string]string{"test": "true"}},
		Status:     projectv1alpha1.ProjectStatus{AtProvider: projectv1alpha1.ProjectObservation{ID: "11111111-1111-1111-1111-111111111111"}},
	}
	parent := &GitRepository{
		ObjectMeta: metav1.ObjectMeta{Name: "parent-repository", Namespace: testNamespace, Labels: map[string]string{"fork": "true"}},
		Status:     GitRepositoryStatus{AtProvider: GitRepositoryObservation{ID: "22222222-2222-2222-2222-222222222222"}},
	}

	t.Run("Reference", func(t *testing.T) {
		kube := fakeclient.NewClientBuilder().WithScheme(s).WithObjects(project).Build()
		gr := &GitRepository{
			ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: testNamespace},
			Spec: GitRepositorySpec{ForProvider: GitRepositoryParameters{
				ProjectIDRef: &xpv2.NamespacedReference{Name: "example-project"},
			}},
		}

		if err := gr.ResolveReferences(context.Background(), kube); err != nil {
			t.Fatalf("ResolveReferences() error = %v", err)
		}
		if got, want := gr.Spec.ForProvider.ProjectID, project.Status.AtProvider.ID; got != want {
			t.Fatalf("ProjectID = %q, want %q", got, want)
		}
		if gr.Spec.ForProvider.ProjectIDRef == nil || gr.Spec.ForProvider.ProjectIDRef.Name != "example-project" {
			t.Fatalf("ProjectIDRef not preserved: %#v", gr.Spec.ForProvider.ProjectIDRef)
		}
	})

	t.Run("SelectorAndParentReference", func(t *testing.T) {
		kube := fakeclient.NewClientBuilder().WithScheme(s).WithObjects(project, parent).Build()
		gr := &GitRepository{
			ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: testNamespace},
			Spec: GitRepositorySpec{ForProvider: GitRepositoryParameters{
				ProjectIDSelector:   &xpv2.NamespacedSelector{MatchLabels: map[string]string{"test": "true"}},
				ParentRepositoryRef: &xpv2.NamespacedReference{Name: "parent-repository"},
			}},
		}

		if err := gr.ResolveReferences(context.Background(), kube); err != nil {
			t.Fatalf("ResolveReferences() error = %v", err)
		}
		if got, want := gr.Spec.ForProvider.ProjectID, project.Status.AtProvider.ID; got != want {
			t.Fatalf("ProjectID = %q, want %q", got, want)
		}
		if got, want := gr.Spec.ForProvider.ParentRepositoryID, parent.Status.AtProvider.ID; got != want {
			t.Fatalf("ParentRepositoryID = %q, want %q", got, want)
		}
		if gr.Spec.ForProvider.ProjectIDRef == nil || gr.Spec.ForProvider.ProjectIDRef.Name != "example-project" || gr.Spec.ForProvider.ProjectIDRef.Namespace != testNamespace {
			t.Fatalf("ProjectIDRef not resolved from selector: %#v", gr.Spec.ForProvider.ProjectIDRef)
		}
	})
}
