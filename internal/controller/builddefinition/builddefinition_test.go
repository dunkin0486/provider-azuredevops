// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package builddefinition

import (
	"context"
	stderrors "errors"
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	adobuild "github.com/microsoft/azure-devops-go-api/azuredevops/v7/build"
	adocore "github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	runtimeTest "github.com/crossplane/crossplane-runtime/v2/pkg/test"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/builddefinition/v1alpha1"
	gitrepositoryv1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/gitrepository/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/builddefinition/fake"
)

const (
	testProjectID       = "11111111-1111-1111-1111-111111111111"
	testRepositoryID    = "22222222-2222-2222-2222-222222222222"
	testDefinitionID    = "24"
	testVariableGroupID = "42"
	testYAMLPath        = "azure-pipelines.yml"
	testRepositoryName  = "example-repository"
)

func buildDefinitionCRWith(externalName string, mutate func(*v1alpha1.BuildDefinition)) *v1alpha1.BuildDefinition {
	cr := &v1alpha1.BuildDefinition{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: v1alpha1.BuildDefinitionSpec{
			ForProvider: v1alpha1.BuildDefinitionParameters{
				Name:         "example-pipeline",
				ProjectID:    testProjectID,
				RepositoryID: testRepositoryID,
				YamlPath:     testYAMLPath,
			},
		},
	}
	if externalName != "" {
		meta.SetExternalName(cr, externalName)
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

func buildDefinition(id int, mutate func(*adobuild.BuildDefinition)) *adobuild.BuildDefinition {
	projectUUID := uuid.MustParse(testProjectID)
	definition := &adobuild.BuildDefinition{
		Id:       intPtr(id),
		Name:     stringPtr("example-pipeline"),
		Path:     stringPtr("\\"),
		Project:  &adocore.TeamProjectReference{Id: &projectUUID},
		Revision: intPtr(7),
		Type:     definitionTypePtr(adobuild.DefinitionTypeValues.Build),
		Url:      stringPtr("https://dev.azure.com/example/project/_apis/build/Definitions/24"),
		Repository: &adobuild.BuildRepository{
			Id:            stringPtr(testRepositoryID),
			Type:          stringPtr(repositoryTypeTfsGit),
			Name:          stringPtr("example-repository"),
			DefaultBranch: stringPtr("refs/heads/main"),
			Url:           stringPtr("https://dev.azure.com/example/project/_git/example-repository"),
		},
		Process: &adobuild.YamlProcess{
			Type:         intPtr(yamlProcessType),
			YamlFilename: stringPtr(testYAMLPath),
		},
		Triggers: &[]interface{}{
			&adobuild.ContinuousIntegrationTrigger{TriggerType: definitionTriggerTypePtr(adobuild.DefinitionTriggerTypeValues.ContinuousIntegration)},
		},
		VariableGroups: &[]adobuild.VariableGroup{{Id: intPtr(42)}},
	}
	if mutate != nil {
		mutate(definition)
	}
	return definition
}

func buildDefinitionKubeClient(t *testing.T, objs ...runtime.Object) ctrlclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		v1alpha1.SchemeBuilder.AddToScheme,
		gitrepositoryv1alpha1.SchemeBuilder.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatalf("AddToScheme(...): %v", err)
		}
	}
	return ctrlclientfake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
}

func buildDefinitionNotFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func TestObserve(t *testing.T) {
	type fields struct {
		client BuildDefinitionClient
	}
	type args struct {
		cr *v1alpha1.BuildDefinition
	}
	type want struct {
		observation managed.ExternalObservation
		err         error
		condition   xpv2.ConditionType
		reason      xpv2.ConditionReason
	}

	cases := map[string]struct {
		fields fields
		args   args
		want   want
	}{
		"NoExternalName": {
			fields: fields{client: &fake.BuildDefinitionClient{}},
			args:   args{cr: buildDefinitionCRWith("", nil)},
			want:   want{observation: managed.ExternalObservation{}},
		},
		"NotFound": {
			fields: fields{client: &fake.BuildDefinitionClient{GetDefinitionFn: func(_ context.Context, _ adobuild.GetDefinitionArgs) (*adobuild.BuildDefinition, error) {
				return nil, buildDefinitionNotFoundErr()
			}}},
			args: args{cr: buildDefinitionCRWith(testDefinitionID, nil)},
			want: want{observation: managed.ExternalObservation{ResourceExists: false}},
		},
		"UpToDate": {
			fields: fields{client: &fake.BuildDefinitionClient{GetDefinitionFn: func(_ context.Context, _ adobuild.GetDefinitionArgs) (*adobuild.BuildDefinition, error) {
				return buildDefinition(24, nil), nil
			}}},
			args: args{cr: buildDefinitionCRWith(testDefinitionID, func(cr *v1alpha1.BuildDefinition) {
				cr.Spec.ForProvider.CIEnabled = boolPtr(true)
				cr.Spec.ForProvider.VariableGroupIDs = []string{testVariableGroupID}
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
		"NeedsUpdate": {
			fields: fields{client: &fake.BuildDefinitionClient{GetDefinitionFn: func(_ context.Context, _ adobuild.GetDefinitionArgs) (*adobuild.BuildDefinition, error) {
				return buildDefinition(24, nil), nil
			}}},
			args: args{cr: buildDefinitionCRWith(testDefinitionID, func(cr *v1alpha1.BuildDefinition) {
				cr.Spec.ForProvider.Name = "renamed-pipeline"
				cr.Spec.ForProvider.CIEnabled = boolPtr(true)
				cr.Spec.ForProvider.VariableGroupIDs = []string{testVariableGroupID}
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{builddefinitions: tc.fields.client}
			got, err := e.Observe(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, runtimeTest.EquateErrors()); diff != "" {
				t.Fatalf("Observe(...): -want error, +got error:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.observation, got); diff != "" {
				t.Fatalf("Observe(...): -want, +got:\n%s", diff)
			}
			if tc.want.condition != "" {
				gotCondition := tc.args.cr.Status.GetCondition(tc.want.condition)
				if gotCondition.Reason != tc.want.reason {
					t.Fatalf("Observe(...): condition %s reason = %q, want %q", tc.want.condition, gotCondition.Reason, tc.want.reason)
				}
			}
		})
	}
}

func TestCreate(t *testing.T) {
	repo := &gitrepositoryv1alpha1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{Name: testRepositoryName, Namespace: "default"},
		Spec:       gitrepositoryv1alpha1.GitRepositorySpec{ForProvider: gitrepositoryv1alpha1.GitRepositoryParameters{Name: testRepositoryName, DefaultBranch: "refs/heads/main"}},
		Status:     gitrepositoryv1alpha1.GitRepositoryStatus{AtProvider: gitrepositoryv1alpha1.GitRepositoryObservation{RemoteURL: "https://dev.azure.com/example/project/_git/example-repository"}},
	}

	var gotCreate adobuild.CreateDefinitionArgs
	e := external{
		kube: buildDefinitionKubeClient(t, repo),
		builddefinitions: &fake.BuildDefinitionClient{
			CreateDefinitionFn: func(_ context.Context, args adobuild.CreateDefinitionArgs) (*adobuild.BuildDefinition, error) {
				gotCreate = args
				return buildDefinition(24, nil), nil
			},
		},
	}

	cr := buildDefinitionCRWith("", func(cr *v1alpha1.BuildDefinition) {
		cr.Spec.ForProvider.RepositoryIDRef = &xpv2.NamespacedReference{Name: testRepositoryName}
		cr.Spec.ForProvider.CIEnabled = boolPtr(true)
		cr.Spec.ForProvider.PREnabled = boolPtr(true)
		cr.Spec.ForProvider.VariableGroupIDs = []string{testVariableGroupID}
	})

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create(...): unexpected error: %v", err)
	}
	if got := meta.GetExternalName(cr); got != testDefinitionID {
		t.Fatalf("Create(...): external name = %q, want %q", got, testDefinitionID)
	}
	if gotCreate.Definition == nil || gotCreate.Definition.Repository == nil || gotCreate.Definition.Repository.Name == nil || *gotCreate.Definition.Repository.Name != testRepositoryName {
		t.Fatalf("Create(...): repository payload = %#v", gotCreate.Definition.Repository)
	}
	if gotCreate.Definition.Process == nil || !yamlProcessMatches(testYAMLPath, gotCreate.Definition.Process) {
		t.Fatalf("Create(...): process payload = %#v", gotCreate.Definition.Process)
	}
	if gotCreate.Definition.VariableGroups == nil || len(*gotCreate.Definition.VariableGroups) != 1 || (*gotCreate.Definition.VariableGroups)[0].Id == nil || *(*gotCreate.Definition.VariableGroups)[0].Id != 42 {
		t.Fatalf("Create(...): variable groups payload = %#v", gotCreate.Definition.VariableGroups)
	}
	if gotCreate.Definition.Triggers == nil || len(*gotCreate.Definition.Triggers) != 2 {
		t.Fatalf("Create(...): triggers payload = %#v", gotCreate.Definition.Triggers)
	}
}

func TestUpdate(t *testing.T) {
	repo := &gitrepositoryv1alpha1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{Name: testRepositoryName, Namespace: "default"},
		Spec:       gitrepositoryv1alpha1.GitRepositorySpec{ForProvider: gitrepositoryv1alpha1.GitRepositoryParameters{Name: testRepositoryName, DefaultBranch: "refs/heads/main"}},
		Status:     gitrepositoryv1alpha1.GitRepositoryStatus{AtProvider: gitrepositoryv1alpha1.GitRepositoryObservation{RemoteURL: "https://dev.azure.com/example/project/_git/example-repository"}},
	}

	t.Run("Success", func(t *testing.T) {
		var gotUpdate adobuild.UpdateDefinitionArgs
		e := external{
			kube: buildDefinitionKubeClient(t, repo),
			builddefinitions: &fake.BuildDefinitionClient{
				GetDefinitionFn: func(_ context.Context, _ adobuild.GetDefinitionArgs) (*adobuild.BuildDefinition, error) {
					return buildDefinition(24, nil), nil
				},
				UpdateDefinitionFn: func(_ context.Context, args adobuild.UpdateDefinitionArgs) (*adobuild.BuildDefinition, error) {
					gotUpdate = args
					return buildDefinition(24, func(definition *adobuild.BuildDefinition) {
						definition.Name = stringPtr("renamed-pipeline")
						definition.Revision = intPtr(8)
					}), nil
				},
			},
		}

		cr := buildDefinitionCRWith(testDefinitionID, func(cr *v1alpha1.BuildDefinition) {
			cr.Spec.ForProvider.Name = "renamed-pipeline"
			cr.Spec.ForProvider.RepositoryIDRef = &xpv2.NamespacedReference{Name: testRepositoryName}
			cr.Spec.ForProvider.CIEnabled = boolPtr(true)
			cr.Spec.ForProvider.VariableGroupIDs = []string{testVariableGroupID}
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("Update(...): unexpected error: %v", err)
		}
		if gotUpdate.Definition == nil || gotUpdate.Definition.Name == nil || *gotUpdate.Definition.Name != "renamed-pipeline" {
			t.Fatalf("Update(...): definition payload = %#v", gotUpdate.Definition)
		}
		if gotUpdate.Definition.Revision == nil || *gotUpdate.Definition.Revision != 7 {
			t.Fatalf("Update(...): revision = %v, want 7", gotUpdate.Definition.Revision)
		}
	})

	t.Run("ImmutableProjectChange", func(t *testing.T) {
		e := external{
			builddefinitions: &fake.BuildDefinitionClient{
				GetDefinitionFn: func(_ context.Context, _ adobuild.GetDefinitionArgs) (*adobuild.BuildDefinition, error) {
					return buildDefinition(24, nil), nil
				},
				UpdateDefinitionFn: func(_ context.Context, _ adobuild.UpdateDefinitionArgs) (*adobuild.BuildDefinition, error) {
					t.Fatal("UpdateDefinition should not be called when projectId changes")
					return nil, nil
				},
			},
		}

		cr := buildDefinitionCRWith(testDefinitionID, func(cr *v1alpha1.BuildDefinition) {
			cr.Spec.ForProvider.ProjectID = "33333333-3333-3333-3333-333333333333"
		})

		if _, err := e.Update(context.Background(), cr); err == nil {
			t.Fatal("Update(...): expected immutable project error, got nil")
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("NoObservedID", func(t *testing.T) {
		e := external{builddefinitions: &fake.BuildDefinitionClient{DeleteDefinitionFn: func(_ context.Context, _ adobuild.DeleteDefinitionArgs) error {
			t.Fatal("DeleteDefinition should not be called without an observed ID")
			return nil
		}}}
		cr := buildDefinitionCRWith("", nil)
		cr.Status.AtProvider.ID = ""
		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("Delete(...): unexpected error: %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		var gotDelete adobuild.DeleteDefinitionArgs
		e := external{builddefinitions: &fake.BuildDefinitionClient{DeleteDefinitionFn: func(_ context.Context, args adobuild.DeleteDefinitionArgs) error {
			gotDelete = args
			return nil
		}}}
		cr := buildDefinitionCRWith(testDefinitionID, nil)
		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("Delete(...): unexpected error: %v", err)
		}
		if gotDelete.DefinitionId == nil || *gotDelete.DefinitionId != 24 {
			t.Fatalf("Delete(...): definition id = %v, want 24", gotDelete.DefinitionId)
		}
	})

	t.Run("NotFoundIgnored", func(t *testing.T) {
		e := external{builddefinitions: &fake.BuildDefinitionClient{DeleteDefinitionFn: func(_ context.Context, _ adobuild.DeleteDefinitionArgs) error {
			return buildDefinitionNotFoundErr()
		}}}
		cr := buildDefinitionCRWith(testDefinitionID, nil)
		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("Delete(...): unexpected error: %v", err)
		}
	})
}

func TestObservationAndComparisonHelpers(t *testing.T) {
	t.Run("Observation", func(t *testing.T) {
		got := observationFromBuildDefinition(buildDefinition(24, nil))
		want := v1alpha1.BuildDefinitionObservation{
			ID:       testDefinitionID,
			Revision: 7,
			URL:      "https://dev.azure.com/example/project/_apis/build/Definitions/24",
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("observationFromBuildDefinition(...): -want, +got:\n%s", diff)
		}
	})

	t.Run("IsUpToDateWithGenericProcessAndTriggerMaps", func(t *testing.T) {
		current := buildDefinition(24, func(definition *adobuild.BuildDefinition) {
			definition.Process = map[string]interface{}{"yamlFilename": testYAMLPath}
			definition.Triggers = &[]interface{}{
				map[string]interface{}{"triggerType": string(adobuild.DefinitionTriggerTypeValues.ContinuousIntegration)},
				map[string]interface{}{"triggerType": string(adobuild.DefinitionTriggerTypeValues.PullRequest)},
			}
		})

		desired := buildDefinitionCRWith(testDefinitionID, func(cr *v1alpha1.BuildDefinition) {
			cr.Spec.ForProvider.CIEnabled = boolPtr(true)
			cr.Spec.ForProvider.PREnabled = boolPtr(true)
			cr.Spec.ForProvider.VariableGroupIDs = []string{testVariableGroupID}
		}).Spec.ForProvider

		if !isUpToDate(desired, current) {
			t.Fatal("isUpToDate(...): got false, want true")
		}
	})

	t.Run("CreateErrorWrapped", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{builddefinitions: &fake.BuildDefinitionClient{CreateDefinitionFn: func(_ context.Context, _ adobuild.CreateDefinitionArgs) (*adobuild.BuildDefinition, error) {
			return nil, wantErr
		}}}

		_, err := e.Create(context.Background(), buildDefinitionCRWith("", nil))
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("Create(...): -want wrapped error, +got:\n%s", diff)
		}
	})
}

func boolPtr(v bool) *bool { return &v }
