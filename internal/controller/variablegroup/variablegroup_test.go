// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package variablegroup

import (
	"context"
	"errors"
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
	testPlainVarName  = "plain"
	testPlainValue    = "plain"
	testExampleName   = "example"
	testSecretValue   = "s3cr3t"
	testSecretVarName = "secret"
	testNamespace     = "default"
	testSecretDataKey = "value"
	testVaultName     = "vault-name"
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
		kube   ctrlclient.Client
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
			fields: fields{
				client: &fakevg.VariableGroupClient{GetVariableGroupFn: func(_ context.Context, args taskagent.GetVariableGroupArgs) (*taskagent.VariableGroup, error) {
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
							testPlainVarName:  taskagent.VariableValue{Value: strPtr(testPlainValue)},
							testSecretVarName: secretValue,
						},
					}, nil
				}},
				kube: fakeClient(t, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "ignored", Namespace: testNamespace},
					Data:       map[string][]byte{testSecretDataKey: []byte(testSecretValue)},
				}),
			},
			args: args{cr: variableGroupCRWith("41", func(cr *v1alpha1.VariableGroup) {
				cr.Spec.ForProvider.ProjectID = projectID.String()
				cr.Spec.ForProvider.Name = testExampleName
				cr.Spec.ForProvider.Description = "shared vars"
				cr.Spec.ForProvider.Variables = []v1alpha1.VariableGroupVariable{{Name: testPlainVarName, Value: testPlainValue}, {Name: testSecretVarName, IsSecret: true, ValueFrom: &v1alpha1.VariableValueSource{SecretKeyRef: xpv2.SecretKeySelector{SecretReference: xpv2.SecretReference{Name: "ignored", Namespace: testNamespace}, Key: testSecretDataKey}}}}
				setSecretsHashAnnotation(cr, &map[string]interface{}{testSecretVarName: taskagent.VariableValue{IsSecret: boolPtr(true), Value: strPtr(testSecretValue)}})
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
		"SecretRotated": {
			fields: fields{
				client: &fakevg.VariableGroupClient{GetVariableGroupFn: func(_ context.Context, _ taskagent.GetVariableGroupArgs) (*taskagent.VariableGroup, error) {
					return &taskagent.VariableGroup{
						Id:   intPtr(41),
						Name: strPtr(testExampleName),
						Type: strPtr(variableGroupTypeVsts),
						VariableGroupProjectReferences: &[]taskagent.VariableGroupProjectReference{{
							ProjectReference: &taskagent.ProjectReference{Id: uuidPtr(projectID)},
						}},
						Variables: &map[string]interface{}{testSecretVarName: secretValue},
					}, nil
				}},
				// The Secret now holds a different value than what was
				// hashed at last Create/Update -- Observe must detect this
				// even though Azure DevOps' response above is unchanged.
				kube: fakeClient(t, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "ignored", Namespace: testNamespace},
					Data:       map[string][]byte{testSecretDataKey: []byte("rotated-value")},
				}),
			},
			args: args{cr: variableGroupCRWith("41", func(cr *v1alpha1.VariableGroup) {
				cr.Spec.ForProvider.ProjectID = projectID.String()
				cr.Spec.ForProvider.Name = testExampleName
				cr.Spec.ForProvider.Variables = []v1alpha1.VariableGroupVariable{{Name: testSecretVarName, IsSecret: true, ValueFrom: &v1alpha1.VariableValueSource{SecretKeyRef: xpv2.SecretKeySelector{SecretReference: xpv2.SecretReference{Name: "ignored", Namespace: testNamespace}, Key: testSecretDataKey}}}}
				setSecretsHashAnnotation(cr, &map[string]interface{}{testSecretVarName: taskagent.VariableValue{IsSecret: boolPtr(true), Value: strPtr(testSecretValue)}})
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{variablegroups: tc.fields.client, kube: tc.fields.kube}
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
		ObjectMeta: metav1.ObjectMeta{Name: "vg-secret", Namespace: testNamespace},
		Data:       map[string][]byte{"password": []byte(secretValue)},
	})

	cr := variableGroupCRWith("", func(cr *v1alpha1.VariableGroup) {
		cr.SetNamespace(testNamespace)
		cr.Spec.ForProvider.ProjectID = projectID.String()
		cr.Spec.ForProvider.Name = testExampleName
		cr.Spec.ForProvider.Description = "shared vars"
		cr.Spec.ForProvider.Variables = []v1alpha1.VariableGroupVariable{
			{Name: testPlainVarName, Value: "hello"},
			{
				Name:     testSecretVarName,
				IsSecret: true,
				ValueFrom: &v1alpha1.VariableValueSource{SecretKeyRef: xpv2.SecretKeySelector{
					SecretReference: xpv2.SecretReference{Name: "vg-secret", Namespace: testNamespace},
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
	secretVar, err := decodeVariableValue((*payload.Variables)[testSecretVarName])
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

// TestDeleteDoesNotResolveSecrets guards against a regression where Delete
// resolved secret variable values (via k8s Secret lookups) before deleting,
// which meant a variable group with a secret variable whose referenced
// Secret had already been removed (e.g. during a cascading namespace
// teardown) could never actually be deleted -- the finalizer would be
// stuck forever even though deleting only requires the group/project IDs.
func TestDeleteDoesNotResolveSecrets(t *testing.T) {
	projectID := uuid.New()
	deleted := false

	cr := variableGroupCRWith("55", func(cr *v1alpha1.VariableGroup) {
		cr.Spec.ForProvider.ProjectID = projectID.String()
		cr.Spec.ForProvider.Variables = []v1alpha1.VariableGroupVariable{{
			Name:     testSecretVarName,
			IsSecret: true,
			ValueFrom: &v1alpha1.VariableValueSource{SecretKeyRef: xpv2.SecretKeySelector{
				SecretReference: xpv2.SecretReference{Name: "already-deleted", Namespace: testNamespace},
				Key:             testSecretDataKey,
			}},
		}}
	})

	// Deliberately no kube client / Secret provided: resolving the
	// referenced Secret would fail (nil kube client panics; a real client
	// would return NotFound), simulating the Secret already being gone.
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
		KeyVault:  &v1alpha1.KeyVaultReference{Name: testVaultName, ServiceEndpointID: serviceEndpointID.String()},
	}, &taskagent.VariableGroup{
		Name: strPtr("kv-group"),
		Type: strPtr(variableGroupTypeAzureKeyVault),
		VariableGroupProjectReferences: &[]taskagent.VariableGroupProjectReference{{
			ProjectReference: &taskagent.ProjectReference{Id: uuidPtr(projectID)},
		}},
		ProviderData: map[string]interface{}{
			"vault":             testVaultName,
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

func TestUpdateNoObservedID(t *testing.T) {
	cr := variableGroupCRWith("", nil)
	e := external{variablegroups: &fakevg.VariableGroupClient{UpdateVariableGroupFn: func(_ context.Context, _ taskagent.UpdateVariableGroupArgs) (*taskagent.VariableGroup, error) {
		t.Fatal("UpdateVariableGroup should not be called when no variable group id has been observed")
		return nil, nil
	}}}

	if _, err := e.Update(context.Background(), cr); err == nil {
		t.Fatal("Update(...): expected error when no variable group id has been observed, got nil")
	}
}

func TestUpdateBuildParametersError(t *testing.T) {
	cr := variableGroupCRWith("42", func(cr *v1alpha1.VariableGroup) {
		// ProjectID left empty: validateParameters must fail.
		cr.Spec.ForProvider.Name = testExampleName
	})

	e := external{variablegroups: &fakevg.VariableGroupClient{UpdateVariableGroupFn: func(_ context.Context, _ taskagent.UpdateVariableGroupArgs) (*taskagent.VariableGroup, error) {
		t.Fatal("UpdateVariableGroup should not be called when the payload cannot be built")
		return nil, nil
	}}}

	if _, err := e.Update(context.Background(), cr); err == nil {
		t.Fatal("Update(...): expected error when buildVariableGroupParameters fails, got nil")
	}
}

func TestUpdateAPIError(t *testing.T) {
	projectID := uuid.New()
	cr := variableGroupCRWith("42", func(cr *v1alpha1.VariableGroup) {
		cr.Spec.ForProvider.ProjectID = projectID.String()
		cr.Spec.ForProvider.Name = testExampleName
	})

	e := external{variablegroups: &fakevg.VariableGroupClient{UpdateVariableGroupFn: func(_ context.Context, _ taskagent.UpdateVariableGroupArgs) (*taskagent.VariableGroup, error) {
		return nil, errBoom
	}}}

	if _, err := e.Update(context.Background(), cr); err == nil {
		t.Fatal("Update(...): expected error when UpdateVariableGroup fails, got nil")
	}
}

func TestDeleteNoObservedID(t *testing.T) {
	cr := variableGroupCRWith("", nil)
	e := external{variablegroups: &fakevg.VariableGroupClient{DeleteVariableGroupFn: func(_ context.Context, _ taskagent.DeleteVariableGroupArgs) error {
		t.Fatal("DeleteVariableGroup should not be called when no variable group id has been observed")
		return nil
	}}}

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete(...): unexpected error: %v", err)
	}
}

func TestDeleteMissingProjectID(t *testing.T) {
	cr := variableGroupCRWith("55", nil)
	e := external{variablegroups: &fakevg.VariableGroupClient{DeleteVariableGroupFn: func(_ context.Context, _ taskagent.DeleteVariableGroupArgs) error {
		t.Fatal("DeleteVariableGroup should not be called when projectId is empty")
		return nil
	}}}

	if _, err := e.Delete(context.Background(), cr); err == nil {
		t.Fatal("Delete(...): expected error when projectId is empty, got nil")
	}
}

func TestDeleteNotFound(t *testing.T) {
	projectID := uuid.New()
	cr := variableGroupCRWith("55", func(cr *v1alpha1.VariableGroup) {
		cr.Spec.ForProvider.ProjectID = projectID.String()
	})

	e := external{variablegroups: &fakevg.VariableGroupClient{DeleteVariableGroupFn: func(_ context.Context, _ taskagent.DeleteVariableGroupArgs) error {
		return variableGroupNotFoundErr()
	}}}

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete(...): unexpected error for a not-found variable group: %v", err)
	}
}

func TestDeleteAPIError(t *testing.T) {
	projectID := uuid.New()
	cr := variableGroupCRWith("55", func(cr *v1alpha1.VariableGroup) {
		cr.Spec.ForProvider.ProjectID = projectID.String()
	})

	e := external{variablegroups: &fakevg.VariableGroupClient{DeleteVariableGroupFn: func(_ context.Context, _ taskagent.DeleteVariableGroupArgs) error {
		return errBoom
	}}}

	if _, err := e.Delete(context.Background(), cr); err == nil {
		t.Fatal("Delete(...): expected error when DeleteVariableGroup fails, got nil")
	}
}

func TestBuildVariableGroupParametersKeyVault(t *testing.T) {
	projectID := uuid.New()
	serviceEndpointID := uuid.New()

	t.Run("Valid", func(t *testing.T) {
		p := v1alpha1.VariableGroupParameters{
			ProjectID: projectID.String(),
			Name:      testExampleName,
			KeyVault:  &v1alpha1.KeyVaultReference{Name: testVaultName, ServiceEndpointID: serviceEndpointID.String()},
		}
		e := external{}
		payload, err := e.buildVariableGroupParameters(context.Background(), variableGroupCRWith("", func(cr *v1alpha1.VariableGroup) { cr.Spec.ForProvider = p }))
		if err != nil {
			t.Fatalf("buildVariableGroupParameters(...): unexpected error: %v", err)
		}
		if payload.Type == nil || *payload.Type != variableGroupTypeAzureKeyVault {
			t.Fatalf("buildVariableGroupParameters(...): type = %v, want %s", payload.Type, variableGroupTypeAzureKeyVault)
		}
		if payload.ProviderData == nil {
			t.Fatal("buildVariableGroupParameters(...): expected ProviderData to be set")
		}
	})

	t.Run("InvalidServiceEndpointID", func(t *testing.T) {
		p := v1alpha1.VariableGroupParameters{
			ProjectID: projectID.String(),
			Name:      testExampleName,
			KeyVault:  &v1alpha1.KeyVaultReference{Name: testVaultName, ServiceEndpointID: "not-a-uuid"},
		}
		e := external{}
		if _, err := e.buildVariableGroupParameters(context.Background(), variableGroupCRWith("", func(cr *v1alpha1.VariableGroup) { cr.Spec.ForProvider = p })); err == nil {
			t.Fatal("buildVariableGroupParameters(...): expected error for an invalid service endpoint id, got nil")
		}
	})

	t.Run("InvalidProjectID", func(t *testing.T) {
		p := v1alpha1.VariableGroupParameters{
			ProjectID: "not-a-uuid",
			Name:      testExampleName,
		}
		e := external{}
		if _, err := e.buildVariableGroupParameters(context.Background(), variableGroupCRWith("", func(cr *v1alpha1.VariableGroup) { cr.Spec.ForProvider = p })); err == nil {
			t.Fatal("buildVariableGroupParameters(...): expected error for an invalid project id, got nil")
		}
	})

	t.Run("InvalidParameters", func(t *testing.T) {
		e := external{}
		if _, err := e.buildVariableGroupParameters(context.Background(), variableGroupCRWith("", nil)); err == nil {
			t.Fatal("buildVariableGroupParameters(...): expected error when required parameters are missing, got nil")
		}
	})
}

func TestValidateKeyVaultParameters(t *testing.T) {
	cases := map[string]struct {
		p       v1alpha1.VariableGroupParameters
		wantErr bool
	}{
		"Valid": {
			p: v1alpha1.VariableGroupParameters{
				KeyVault: &v1alpha1.KeyVaultReference{Name: testVaultName, ServiceEndpointID: uuid.New().String()},
			},
		},
		"MissingServiceEndpointID": {
			p: v1alpha1.VariableGroupParameters{
				KeyVault: &v1alpha1.KeyVaultReference{Name: testVaultName},
			},
			wantErr: true,
		},
		"MissingName": {
			p: v1alpha1.VariableGroupParameters{
				KeyVault: &v1alpha1.KeyVaultReference{ServiceEndpointID: uuid.New().String()},
			},
			wantErr: true,
		},
		"InlineVariablesNotSupported": {
			p: v1alpha1.VariableGroupParameters{
				KeyVault:  &v1alpha1.KeyVaultReference{Name: testVaultName, ServiceEndpointID: uuid.New().String()},
				Variables: []v1alpha1.VariableGroupVariable{{Name: "foo", Value: "bar"}},
			},
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateKeyVaultParameters(tc.p)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateKeyVaultParameters(...): error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestParseServiceEndpointID(t *testing.T) {
	id := uuid.New()
	got, err := parseServiceEndpointID(id.String())
	if err != nil {
		t.Fatalf("parseServiceEndpointID(...): unexpected error: %v", err)
	}
	if got == nil || *got != id {
		t.Fatalf("parseServiceEndpointID(...): got %v, want %v", got, id)
	}

	if _, err := parseServiceEndpointID("not-a-uuid"); err == nil {
		t.Fatal("parseServiceEndpointID(...): expected error for an invalid uuid, got nil")
	}
}

func TestDecodeVariableValue(t *testing.T) {
	t.Run("Struct", func(t *testing.T) {
		got, err := decodeVariableValue(taskagent.VariableValue{Value: strPtr("v")})
		if err != nil {
			t.Fatalf("decodeVariableValue(...): unexpected error: %v", err)
		}
		if valueOrEmpty(got.Value) != "v" {
			t.Fatalf("decodeVariableValue(...): value = %q, want %q", valueOrEmpty(got.Value), "v")
		}
	})

	t.Run("PointerNil", func(t *testing.T) {
		got, err := decodeVariableValue((*taskagent.VariableValue)(nil))
		if err != nil {
			t.Fatalf("decodeVariableValue(...): unexpected error: %v", err)
		}
		if got != (taskagent.VariableValue{}) {
			t.Fatalf("decodeVariableValue(...): got %+v, want zero value", got)
		}
	})

	t.Run("PointerNonNil", func(t *testing.T) {
		vv := taskagent.VariableValue{Value: strPtr("v")}
		got, err := decodeVariableValue(&vv)
		if err != nil {
			t.Fatalf("decodeVariableValue(...): unexpected error: %v", err)
		}
		if valueOrEmpty(got.Value) != "v" {
			t.Fatalf("decodeVariableValue(...): value = %q, want %q", valueOrEmpty(got.Value), "v")
		}
	})

	t.Run("RawMap", func(t *testing.T) {
		got, err := decodeVariableValue(map[string]interface{}{"value": "from-json", "isSecret": true})
		if err != nil {
			t.Fatalf("decodeVariableValue(...): unexpected error: %v", err)
		}
		if valueOrEmpty(got.Value) != "from-json" || !boolValue(got.IsSecret) {
			t.Fatalf("decodeVariableValue(...): got %+v", got)
		}
	})

	t.Run("RemarshalError", func(t *testing.T) {
		if _, err := decodeVariableValue(func() {}); err == nil {
			t.Fatal("decodeVariableValue(...): expected error when the value cannot be marshaled, got nil")
		}
	})
}

func TestDecodeProviderData(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		got, err := decodeProviderData(nil)
		if err != nil {
			t.Fatalf("decodeProviderData(...): unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("decodeProviderData(...): got %+v, want nil", got)
		}
	})

	t.Run("Struct", func(t *testing.T) {
		id := uuid.New()
		got, err := decodeProviderData(taskagent.AzureKeyVaultVariableGroupProviderData{Vault: strPtr("v"), ServiceEndpointId: &id})
		if err != nil {
			t.Fatalf("decodeProviderData(...): unexpected error: %v", err)
		}
		if got == nil || valueOrEmpty(got.Vault) != "v" {
			t.Fatalf("decodeProviderData(...): got %+v", got)
		}
	})

	t.Run("Pointer", func(t *testing.T) {
		id := uuid.New()
		in := &taskagent.AzureKeyVaultVariableGroupProviderData{Vault: strPtr("v"), ServiceEndpointId: &id}
		got, err := decodeProviderData(in)
		if err != nil {
			t.Fatalf("decodeProviderData(...): unexpected error: %v", err)
		}
		if got != in {
			t.Fatalf("decodeProviderData(...): got %p, want the same pointer %p", got, in)
		}
	})

	t.Run("RawMap", func(t *testing.T) {
		id := uuid.New()
		got, err := decodeProviderData(map[string]interface{}{"vault": "v", "serviceEndpointId": id.String()})
		if err != nil {
			t.Fatalf("decodeProviderData(...): unexpected error: %v", err)
		}
		if got == nil || valueOrEmpty(got.Vault) != "v" {
			t.Fatalf("decodeProviderData(...): got %+v", got)
		}
	})

	t.Run("RemarshalError", func(t *testing.T) {
		if _, err := decodeProviderData(func() {}); err == nil {
			t.Fatal("decodeProviderData(...): expected error when the value cannot be marshaled, got nil")
		}
	})
}

func TestProjectReferencesMatch(t *testing.T) {
	projectID := uuid.New()

	cases := map[string]struct {
		refs *[]taskagent.VariableGroupProjectReference
		want bool
	}{
		"Nil":   {refs: nil, want: false},
		"Empty": {refs: &[]taskagent.VariableGroupProjectReference{}, want: false},
		"TooMany": {
			refs: &[]taskagent.VariableGroupProjectReference{
				{ProjectReference: &taskagent.ProjectReference{Id: uuidPtr(projectID)}},
				{ProjectReference: &taskagent.ProjectReference{Id: uuidPtr(uuid.New())}},
			},
			want: false,
		},
		"NilProjectReference": {
			refs: &[]taskagent.VariableGroupProjectReference{{ProjectReference: nil}},
			want: false,
		},
		"NilID": {
			refs: &[]taskagent.VariableGroupProjectReference{{ProjectReference: &taskagent.ProjectReference{}}},
			want: false,
		},
		"Match": {
			refs: &[]taskagent.VariableGroupProjectReference{{ProjectReference: &taskagent.ProjectReference{Id: uuidPtr(projectID)}}},
			want: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := projectReferencesMatch(projectID.String(), tc.refs); got != tc.want {
				t.Fatalf("projectReferencesMatch(...): got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeType(t *testing.T) {
	if got := normalizeType(nil); got != variableGroupTypeVsts {
		t.Fatalf("normalizeType(nil) = %q, want %q", got, variableGroupTypeVsts)
	}
	empty := ""
	if got := normalizeType(&empty); got != variableGroupTypeVsts {
		t.Fatalf("normalizeType(\"\") = %q, want %q", got, variableGroupTypeVsts)
	}
	kv := variableGroupTypeAzureKeyVault
	if got := normalizeType(&kv); got != variableGroupTypeAzureKeyVault {
		t.Fatalf("normalizeType(%q) = %q, want %q", kv, got, kv)
	}
}

func TestUUIDStringOrEmpty(t *testing.T) {
	if got := uuidStringOrEmpty(nil); got != "" {
		t.Fatalf("uuidStringOrEmpty(nil) = %q, want \"\"", got)
	}
	id := uuid.New()
	if got := uuidStringOrEmpty(&id); got != id.String() {
		t.Fatalf("uuidStringOrEmpty(...) = %q, want %q", got, id.String())
	}
}

func TestKeyVaultUpToDate(t *testing.T) {
	serviceEndpointID := uuid.New()
	desired := &v1alpha1.KeyVaultReference{Name: testVaultName, ServiceEndpointID: serviceEndpointID.String()}

	t.Run("WrongObservedType", func(t *testing.T) {
		got, err := keyVaultUpToDate(desired, variableGroupTypeVsts, nil)
		if err != nil {
			t.Fatalf("keyVaultUpToDate(...): unexpected error: %v", err)
		}
		if got {
			t.Fatal("keyVaultUpToDate(...): got true, want false when observed type is not AzureKeyVault")
		}
	})

	t.Run("NilProviderData", func(t *testing.T) {
		got, err := keyVaultUpToDate(desired, variableGroupTypeAzureKeyVault, nil)
		if err != nil {
			t.Fatalf("keyVaultUpToDate(...): unexpected error: %v", err)
		}
		if got {
			t.Fatal("keyVaultUpToDate(...): got true, want false when provider data is nil")
		}
	})

	t.Run("DecodeError", func(t *testing.T) {
		if _, err := keyVaultUpToDate(desired, variableGroupTypeAzureKeyVault, func() {}); err == nil {
			t.Fatal("keyVaultUpToDate(...): expected error when provider data cannot be decoded, got nil")
		}
	})

	t.Run("Match", func(t *testing.T) {
		got, err := keyVaultUpToDate(desired, variableGroupTypeAzureKeyVault, map[string]interface{}{
			"vault":             testVaultName,
			"serviceEndpointId": serviceEndpointID.String(),
		})
		if err != nil {
			t.Fatalf("keyVaultUpToDate(...): unexpected error: %v", err)
		}
		if !got {
			t.Fatal("keyVaultUpToDate(...): got false, want true")
		}
	})

	t.Run("Mismatch", func(t *testing.T) {
		got, err := keyVaultUpToDate(desired, variableGroupTypeAzureKeyVault, map[string]interface{}{
			"vault":             "different-vault",
			"serviceEndpointId": serviceEndpointID.String(),
		})
		if err != nil {
			t.Fatalf("keyVaultUpToDate(...): unexpected error: %v", err)
		}
		if got {
			t.Fatal("keyVaultUpToDate(...): got true, want false")
		}
	})
}

func TestSecretsUpToDate(t *testing.T) {
	t.Run("KeyVaultGroupsSkipSecretCheck", func(t *testing.T) {
		cr := variableGroupCRWith("", func(cr *v1alpha1.VariableGroup) {
			cr.Spec.ForProvider.KeyVault = &v1alpha1.KeyVaultReference{Name: "v", ServiceEndpointID: uuid.New().String()}
		})
		e := external{}
		got, err := e.secretsUpToDate(context.Background(), cr)
		if err != nil {
			t.Fatalf("secretsUpToDate(...): unexpected error: %v", err)
		}
		if !got {
			t.Fatal("secretsUpToDate(...): got false, want true for a KeyVault-linked group")
		}
	})

	t.Run("NoSecretVariables", func(t *testing.T) {
		cr := variableGroupCRWith("", func(cr *v1alpha1.VariableGroup) {
			cr.Spec.ForProvider.Variables = []v1alpha1.VariableGroupVariable{{Name: testPlainVarName, Value: testPlainValue}}
		})
		e := external{}
		got, err := e.secretsUpToDate(context.Background(), cr)
		if err != nil {
			t.Fatalf("secretsUpToDate(...): unexpected error: %v", err)
		}
		if !got {
			t.Fatal("secretsUpToDate(...): got false, want true when there are no secret variables")
		}
	})

	t.Run("ResolveError", func(t *testing.T) {
		cr := variableGroupCRWith("", func(cr *v1alpha1.VariableGroup) {
			cr.Spec.ForProvider.Variables = []v1alpha1.VariableGroupVariable{{
				Name:     testSecretVarName,
				IsSecret: true,
				ValueFrom: &v1alpha1.VariableValueSource{SecretKeyRef: xpv2.SecretKeySelector{
					SecretReference: xpv2.SecretReference{Name: "missing", Namespace: testNamespace},
					Key:             testSecretDataKey,
				}},
			}}
		})
		e := external{kube: fakeClient(t)}
		if _, err := e.secretsUpToDate(context.Background(), cr); err == nil {
			t.Fatal("secretsUpToDate(...): expected error when the referenced Secret cannot be resolved, got nil")
		}
	})
}

func strPtr(s string) *string         { return &s }
func intPtr(i int) *int               { return &i }
func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

var errBoom = errors.New("boom")
