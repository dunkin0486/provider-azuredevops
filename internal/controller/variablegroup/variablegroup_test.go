// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package variablegroup

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/variablegroup/v1alpha1"
	fakevg "github.com/dunkin0486/provider-azuredevops/internal/controller/variablegroup/fake"
)

const (
	testPlainVarName = "plain"
	testPlainValue   = "plain"
	testExampleName  = "example"
)

func variableGroupCRWith(name string, mutate func(*v1alpha1.VariableGroup)) *v1alpha1.VariableGroup {
	cr := &v1alpha1.VariableGroup{}
	if name != "" {
		meta.SetExternalName(cr, name)
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

func variableGroupNotFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func fakeClient(t *testing.T, objs ...runtime.Object) ctrlclient.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1.AddToScheme(...): %v", err)
	}
	if err := v1alpha1.SchemeBuilder.AddToScheme(scheme); err != nil {
		t.Fatalf("v1alpha1.SchemeBuilder.AddToScheme(...): %v", err)
	}
	return ctrlclientfake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
}

func TestObserve(t *testing.T) {
	projectID := uuid.New()
	secretValue := taskagent.VariableValue{IsSecret: boolPtr(true)}

	type fields struct {
		client VariableGroupClient
	}
	type args struct {
		cr *v1alpha1.VariableGroup
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
			fields: fields{client: &fakevg.VariableGroupClient{}},
			args: args{cr: variableGroupCRWith("", func(cr *v1alpha1.VariableGroup) {
				cr.Spec.ForProvider.ProjectID = projectID.String()
			})},
			want: want{observation: managed.ExternalObservation{}},
		},
		"NotFound": {
			fields: fields{client: &fakevg.VariableGroupClient{GetVariableGroupFn: func(_ context.Context, _ taskagent.GetVariableGroupArgs) (*taskagent.VariableGroup, error) {
				return nil, variableGroupNotFoundErr()
			}}},
			args: args{cr: variableGroupCRWith("41", func(cr *v1alpha1.VariableGroup) {
				cr.Spec.ForProvider.ProjectID = projectID.String()
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: false}},
		},
		"UpToDate": {
			fields: fields{client: &fakevg.VariableGroupClient{GetVariableGroupFn: func(_ context.Context, args taskagent.GetVariableGroupArgs) (*taskagent.VariableGroup, error) {
				if args.Project == nil || *args.Project != projectID.String() {
					t.Fatalf("GetVariableGroup project = %v, want %s", args.Project, projectID)
				}
				return &taskagent.VariableGroup{
					Id:          intPtr(41),
					Name:        strPtr(testExampleName),
					Description: strPtr("shared vars"),
					Type:        strPtr(variableGroupTypeVsts),
					VariableGroupProjectReferences: &[]taskagent.VariableGroupProjectReference{{
						ProjectReference: &taskagent.ProjectReference{Id: uuidPtr(projectID)},
					}},
					Variables: &map[string]interface{}{
						testPlainVarName: taskagent.VariableValue{Value: strPtr(testPlainValue)},
						"secret":         secretValue,
					},
				}, nil
			}}},
			args: args{cr: variableGroupCRWith("41", func(cr *v1alpha1.VariableGroup) {
				cr.Spec.ForProvider.ProjectID = projectID.String()
				cr.Spec.ForProvider.Name = testExampleName
				cr.Spec.ForProvider.Description = "shared vars"
				cr.Spec.ForProvider.Variables = []v1alpha1.VariableGroupVariable{{Name: testPlainVarName, Value: testPlainValue}, {Name: "secret", IsSecret: true, ValueFrom: &v1alpha1.VariableValueSource{SecretKeyRef: xpv2.SecretKeySelector{SecretReference: xpv2.SecretReference{Name: "ignored", Namespace: "default"}, Key: "value"}}}}
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
		"NotUpToDate": {
			fields: fields{client: &fakevg.VariableGroupClient{GetVariableGroupFn: func(_ context.Context, _ taskagent.GetVariableGroupArgs) (*taskagent.VariableGroup, error) {
				return &taskagent.VariableGroup{
					Id:   intPtr(41),
					Name: strPtr(testExampleName),
					Type: strPtr(variableGroupTypeVsts),
					VariableGroupProjectReferences: &[]taskagent.VariableGroupProjectReference{{
						ProjectReference: &taskagent.ProjectReference{Id: uuidPtr(projectID)},
					}},
					Variables: &map[string]interface{}{testPlainVarName: taskagent.VariableValue{Value: strPtr("old")}},
				}, nil
			}}},
			args: args{cr: variableGroupCRWith("41", func(cr *v1alpha1.VariableGroup) {
				cr.Spec.ForProvider.ProjectID = projectID.String()
				cr.Spec.ForProvider.Name = testExampleName
				cr.Spec.ForProvider.Variables = []v1alpha1.VariableGroupVariable{{Name: testPlainVarName, Value: "new"}}
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{variablegroups: tc.fields.client}
			got, err := e.Observe(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
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

func TestCreateResolvesSecretsAndDoesNotPersistThem(t *testing.T) {
	projectID := uuid.New()
	secretValue := "super-secret"
	var payload *taskagent.VariableGroupParameters

	kube := fakeClient(t, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "vg-secret", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte(secretValue)},
	})

	cr := variableGroupCRWith("", func(cr *v1alpha1.VariableGroup) {
		cr.SetNamespace("default")
		cr.Spec.ForProvider.ProjectID = projectID.String()
		cr.Spec.ForProvider.Name = testExampleName
		cr.Spec.ForProvider.Description = "shared vars"
		cr.Spec.ForProvider.Variables = []v1alpha1.VariableGroupVariable{
			{Name: testPlainVarName, Value: "hello"},
			{
				Name:     "secret",
				IsSecret: true,
				ValueFrom: &v1alpha1.VariableValueSource{SecretKeyRef: xpv2.SecretKeySelector{
					SecretReference: xpv2.SecretReference{Name: "vg-secret", Namespace: "default"},
					Key:             "password",
				}},
			},
		}
	})

	e := external{
		kube: kube,
		variablegroups: &fakevg.VariableGroupClient{AddVariableGroupFn: func(_ context.Context, args taskagent.AddVariableGroupArgs) (*taskagent.VariableGroup, error) {
			payload = args.VariableGroupParameters
			return &taskagent.VariableGroup{Id: intPtr(24)}, nil
		}},
	}

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create(...): unexpected error: %v", err)
	}

	if got := meta.GetExternalName(cr); got != "24" {
		t.Fatalf("Create(...): external name = %q, want %q", got, "24")
	}
	if got := cr.Status.AtProvider.ID; got != "24" {
		t.Fatalf("Create(...): status id = %q, want %q", got, "24")
	}
	if cr.Spec.ForProvider.Variables[1].Value != "" {
		t.Fatalf("Create(...): secret variable value leaked into spec: %q", cr.Spec.ForProvider.Variables[1].Value)
	}
	if payload == nil || payload.Variables == nil {
		t.Fatal("Create(...): AddVariableGroup payload missing variables")
	}
	secretVar, err := decodeVariableValue((*payload.Variables)["secret"])
	if err != nil {
		t.Fatalf("decodeVariableValue(...): %v", err)
	}
	if !boolValue(secretVar.IsSecret) {
		t.Fatal("Create(...): secret variable not marked secret")
	}
	if valueOrEmpty(secretVar.Value) != secretValue {
		t.Fatalf("Create(...): secret variable value = %q, want %q", valueOrEmpty(secretVar.Value), secretValue)
	}
}

func TestCreateRejectsPlaintextSecretVariables(t *testing.T) {
	projectID := uuid.New()
	cr := variableGroupCRWith("", func(cr *v1alpha1.VariableGroup) {
		cr.Spec.ForProvider.ProjectID = projectID.String()
		cr.Spec.ForProvider.Name = testExampleName
		cr.Spec.ForProvider.Variables = []v1alpha1.VariableGroupVariable{{Name: "bad", Value: "plaintext", IsSecret: true}}
	})

	e := external{variablegroups: &fakevg.VariableGroupClient{AddVariableGroupFn: func(_ context.Context, _ taskagent.AddVariableGroupArgs) (*taskagent.VariableGroup, error) {
		t.Fatal("AddVariableGroup should not be called for invalid secret configuration")
		return nil, nil
	}}}

	if _, err := e.Create(context.Background(), cr); err == nil {
		t.Fatal("Create(...): expected error for plaintext secret variable, got nil")
	}
}

func TestUpdate(t *testing.T) {
	projectID := uuid.New()
	var gotArgs taskagent.UpdateVariableGroupArgs

	cr := variableGroupCRWith("42", func(cr *v1alpha1.VariableGroup) {
		cr.Spec.ForProvider.ProjectID = projectID.String()
		cr.Spec.ForProvider.Name = testExampleName
		cr.Spec.ForProvider.Description = "updated"
	})

	e := external{variablegroups: &fakevg.VariableGroupClient{UpdateVariableGroupFn: func(_ context.Context, args taskagent.UpdateVariableGroupArgs) (*taskagent.VariableGroup, error) {
		gotArgs = args
		return &taskagent.VariableGroup{Id: intPtr(42)}, nil
	}}}

	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatalf("Update(...): unexpected error: %v", err)
	}
	if gotArgs.GroupId == nil || *gotArgs.GroupId != 42 {
		t.Fatalf("Update(...): group id = %v, want 42", gotArgs.GroupId)
	}
	if gotArgs.VariableGroupParameters == nil || gotArgs.VariableGroupParameters.Name == nil || *gotArgs.VariableGroupParameters.Name != testExampleName {
		t.Fatalf("Update(...): unexpected payload: %+v", gotArgs.VariableGroupParameters)
	}
}

func TestDelete(t *testing.T) {
	projectID := uuid.New()
	deleted := false

	cr := variableGroupCRWith("55", func(cr *v1alpha1.VariableGroup) {
		cr.Spec.ForProvider.ProjectID = projectID.String()
	})

	e := external{variablegroups: &fakevg.VariableGroupClient{DeleteVariableGroupFn: func(_ context.Context, args taskagent.DeleteVariableGroupArgs) error {
		deleted = true
		if args.GroupId == nil || *args.GroupId != 55 {
			t.Fatalf("Delete(...): group id = %v, want 55", args.GroupId)
		}
		if args.ProjectIds == nil || len(*args.ProjectIds) != 1 || (*args.ProjectIds)[0] != projectID.String() {
			t.Fatalf("Delete(...): project ids = %v, want [%s]", args.ProjectIds, projectID)
		}
		return nil
	}}}

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete(...): unexpected error: %v", err)
	}
	if !deleted {
		t.Fatal("Delete(...): DeleteVariableGroup was not called")
	}
}

func TestIsUpToDateKeyVault(t *testing.T) {
	projectID := uuid.New()
	serviceEndpointID := uuid.New()
	upToDate, err := isUpToDate(v1alpha1.VariableGroupParameters{
		ProjectID: projectID.String(),
		Name:      "kv-group",
		KeyVault:  &v1alpha1.KeyVaultReference{Name: "vault-name", ServiceEndpointID: serviceEndpointID.String()},
	}, &taskagent.VariableGroup{
		Name: strPtr("kv-group"),
		Type: strPtr(variableGroupTypeAzureKeyVault),
		VariableGroupProjectReferences: &[]taskagent.VariableGroupProjectReference{{
			ProjectReference: &taskagent.ProjectReference{Id: uuidPtr(projectID)},
		}},
		ProviderData: map[string]interface{}{
			"vault":             "vault-name",
			"serviceEndpointId": serviceEndpointID.String(),
		},
	})
	if err != nil {
		t.Fatalf("isUpToDate(...): unexpected error: %v", err)
	}
	if !upToDate {
		t.Fatal("isUpToDate(...): got false, want true")
	}
}

func TestObservationDoesNotLeakSecrets(t *testing.T) {
	got := observationFromVariableGroup(&taskagent.VariableGroup{Id: intPtr(1)})
	want := v1alpha1.VariableGroupObservation{ID: "1"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("observationFromVariableGroup(...): -want, +got:\n%s", diff)
	}
}

func strPtr(s string) *string         { return &s }
func intPtr(i int) *int               { return &i }
func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }
