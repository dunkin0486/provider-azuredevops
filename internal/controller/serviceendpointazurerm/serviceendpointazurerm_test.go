// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package serviceendpointazurerm

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	adoserviceendpoint "github.com/microsoft/azure-devops-go-api/azuredevops/v7/serviceendpoint"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointazurerm/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/serviceendpointazurerm/fake"
)

const defaultNamespace = "default"

const (
	testTenantID          = "f052ea28-cd44-43e4-83a4-6e1f5b3f67fd"
	testSPSecretName      = "sp-creds"
	testSPSecretClientKey = "clientSecret"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("corev1.AddToScheme() error = %v", err)
	}
	return s
}

func newKube(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return clientfake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(objs...).Build()
}

func serviceEndpointCR(externalName string, mutate func(*v1alpha1.ServiceEndpointAzureRM)) *v1alpha1.ServiceEndpointAzureRM {
	cr := &v1alpha1.ServiceEndpointAzureRM{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: defaultNamespace},
		Spec: v1alpha1.ServiceEndpointAzureRMSpec{
			ForProvider: v1alpha1.ServiceEndpointAzureRMParameters{
				Name:                  "example-connection",
				ProjectID:             "8f571f72-7e56-4ec5-9485-6be503ca9763",
				AzureSubscriptionID:   "ae824372-5fb2-4160-80b2-298148d464aa",
				AzureSubscriptionName: "example-subscription",
				AzureTenantID:         testTenantID,
				Credentials: v1alpha1.ServiceEndpointAzureRMCredentials{
					WorkloadIdentityFederation: &v1alpha1.WorkloadIdentityFederationCredentials{ClientID: "52c11e3b-0883-4899-bf6f-7028608f480f"},
				},
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

func notFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func endpointWith(id uuid.UUID, ready bool, scheme string, data map[string]string, params map[string]string) *adoserviceendpoint.ServiceEndpoint {
	name := "example-connection"
	typ := "AzureRM"
	return &adoserviceendpoint.ServiceEndpoint{
		Id:   &id,
		Name: &name,
		Type: &typ,
		Authorization: &adoserviceendpoint.EndpointAuthorization{
			Scheme:     &scheme,
			Parameters: &params,
		},
		Data:    &data,
		IsReady: &ready,
	}
}

func TestObserve(t *testing.T) {
	id := uuid.New()
	ready := true
	notReady := false
	base := serviceEndpointCR("", nil)
	baseData := desiredData(base.Spec.ForProvider, "52c11e3b-0883-4899-bf6f-7028608f480f", testTenantID)

	type fields struct {
		client ServiceEndpointClient
		kube   client.Client
	}
	type args struct {
		cr *v1alpha1.ServiceEndpointAzureRM
	}
	type want struct {
		o         managed.ExternalObservation
		err       error
		condition xpv2.ConditionReason
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"NoExternalName": {
			reason: "Observe should report nothing when the endpoint ID has not been set yet.",
			fields: fields{client: &fake.ServiceEndpointClient{}},
			args:   args{cr: serviceEndpointCR("", nil)},
			want:   want{o: managed.ExternalObservation{}},
		},
		"NotFound": {
			reason: "Observe should report no external resource when Azure DevOps returns 404.",
			fields: fields{client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				return nil, notFoundErr()
			}}},
			args: args{cr: serviceEndpointCR(id.String(), nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: false}},
		},
		"UpToDate": {
			reason: "Observe should report up to date when mutable non-secret fields match.",
			fields: fields{client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				return endpointWith(id, ready, serviceEndpointAuthorizationWIF, cloneMap(baseData), map[string]string{
					authParamTenantID:           testTenantID,
					authParamServicePrincipalID: "52c11e3b-0883-4899-bf6f-7028608f480f",
				}), nil
			}}},
			args: args{cr: serviceEndpointCR(id.String(), nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.ReasonAvailable},
		},
		"NotUpToDate": {
			reason: "Observe should report not up to date when the subscription name differs.",
			fields: fields{client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				data := cloneMap(baseData)
				data[dataKeySubscriptionName] = "other-subscription"
				return endpointWith(id, ready, serviceEndpointAuthorizationWIF, data, map[string]string{
					authParamTenantID:           testTenantID,
					authParamServicePrincipalID: "52c11e3b-0883-4899-bf6f-7028608f480f",
				}), nil
			}}},
			args: args{cr: serviceEndpointCR(id.String(), nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.ReasonAvailable},
		},
		"NotReady": {
			reason: "Observe should surface Creating while the endpoint exists but is not ready.",
			fields: fields{client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				return endpointWith(id, notReady, serviceEndpointAuthorizationWIF, cloneMap(baseData), map[string]string{
					authParamTenantID:           testTenantID,
					authParamServicePrincipalID: "52c11e3b-0883-4899-bf6f-7028608f480f",
				}), nil
			}}},
			args: args{cr: serviceEndpointCR(id.String(), nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.ReasonCreating},
		},
		"ClientSecretRotated": {
			reason: "Observe should report not up to date when the referenced Secret's value no longer matches the hash captured at last Create/Update, even though Azure DevOps' response is unchanged (it never returns the secret back).",
			fields: fields{
				client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
					data := cloneMap(baseData)
					return endpointWith(id, ready, serviceEndpointAuthorizationServicePrinc, data, map[string]string{
						authParamTenantID:           testTenantID,
						authParamServicePrincipalID: "925ca56d-16ff-4987-af31-cf17d90ba8f3",
					}), nil
				}},
				kube: newKube(t, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: testSPSecretName, Namespace: defaultNamespace},
					Data:       map[string][]byte{testSPSecretClientKey: []byte("rotated-value")},
				}),
			},
			args: args{cr: serviceEndpointCR(id.String(), func(cr *v1alpha1.ServiceEndpointAzureRM) {
				cr.Spec.ForProvider.Credentials = v1alpha1.ServiceEndpointAzureRMCredentials{
					ServicePrincipal: &v1alpha1.ServicePrincipalCredentials{
						ClientID: "925ca56d-16ff-4987-af31-cf17d90ba8f3",
						ClientSecretRef: xpv2.SecretKeySelector{
							SecretReference: xpv2.SecretReference{Name: testSPSecretName, Namespace: defaultNamespace},
							Key:             testSPSecretClientKey,
						},
					},
				}
				setClientSecretHashAnnotation(cr, "original-value")
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.ReasonAvailable},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			kube := tc.fields.kube
			if kube == nil {
				kube = newKube(t)
			}
			e := external{kube: kube, serviceEndpoint: tc.fields.client}
			got, err := e.Observe(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if gotReason := tc.args.cr.Status.GetCondition(xpv2.TypeReady).Reason; gotReason != tc.want.condition {
				t.Errorf("\n%s\ne.Observe(...): ready reason = %q, want %q\n", tc.reason, gotReason, tc.want.condition)
			}
		})
	}
}

func TestCreateServicePrincipal(t *testing.T) {
	secretValue := "super-secret-value"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSPSecretName, Namespace: defaultNamespace},
		Data:       map[string][]byte{testSPSecretClientKey: []byte(secretValue)},
	}

	var got adoserviceendpoint.CreateServiceEndpointArgs
	endpointID := uuid.New()
	e := external{
		kube: newKube(t, secret),
		serviceEndpoint: &fake.ServiceEndpointClient{CreateServiceEndpointFn: func(_ context.Context, args adoserviceendpoint.CreateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
			got = args
			return &adoserviceendpoint.ServiceEndpoint{
				Id:            &endpointID,
				IsReady:       boolPtr(true),
				Authorization: &adoserviceendpoint.EndpointAuthorization{Scheme: strPtr(serviceEndpointAuthorizationServicePrinc)},
			}, nil
		}},
	}

	cr := serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointAzureRM) {
		cr.Spec.ForProvider.ResourceGroup = "rg-apps"
		cr.Spec.ForProvider.Credentials = v1alpha1.ServiceEndpointAzureRMCredentials{
			ServicePrincipal: &v1alpha1.ServicePrincipalCredentials{
				ClientID: "925ca56d-16ff-4987-af31-cf17d90ba8f3",
				ClientSecretRef: xpv2.SecretKeySelector{
					SecretReference: xpv2.SecretReference{Name: testSPSecretName, Namespace: defaultNamespace},
					Key:             testSPSecretClientKey,
				},
			},
		}
	})

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("e.Create(...): unexpected error: %v", err)
	}

	if got.Endpoint == nil || got.Endpoint.Authorization == nil || got.Endpoint.Authorization.Parameters == nil {
		t.Fatalf("e.Create(...): expected authorization parameters, got %#v", got.Endpoint)
	}
	params := *got.Endpoint.Authorization.Parameters
	if params[authParamServicePrincipalKey] != secretValue {
		t.Fatalf("e.Create(...): serviceprincipalkey = %q, want %q", params[authParamServicePrincipalKey], secretValue)
	}
	if gotScheme := *got.Endpoint.Authorization.Scheme; gotScheme != serviceEndpointAuthorizationServicePrinc {
		t.Fatalf("e.Create(...): scheme = %q, want %q", gotScheme, serviceEndpointAuthorizationServicePrinc)
	}
	if gotType := *got.Endpoint.Type; gotType != serviceEndpointTypeAzureRM {
		t.Fatalf("e.Create(...): type = %q, want %q", gotType, serviceEndpointTypeAzureRM)
	}
	if data := *got.Endpoint.Data; data[dataKeyResourceGroupName] != "rg-apps" {
		t.Fatalf("e.Create(...): resourceGroupName = %q, want %q", data[dataKeyResourceGroupName], "rg-apps")
	}
	if gotName := meta.GetExternalName(cr); gotName != endpointID.String() {
		t.Fatalf("e.Create(...): external name = %q, want %q", gotName, endpointID.String())
	}
	if got := cr.GetAnnotations()[annotationClientSecretHash]; got != hashClientSecret(secretValue) {
		t.Fatalf("e.Create(...): client secret hash annotation = %q, want hash of %q", got, secretValue)
	}
}

func TestCreateWorkloadIdentityFederation(t *testing.T) {
	var got adoserviceendpoint.CreateServiceEndpointArgs
	e := external{
		kube: newKube(t),
		serviceEndpoint: &fake.ServiceEndpointClient{CreateServiceEndpointFn: func(_ context.Context, args adoserviceendpoint.CreateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
			got = args
			return &adoserviceendpoint.ServiceEndpoint{Id: uuidPtr(uuid.New()), IsReady: boolPtr(true), Authorization: &adoserviceendpoint.EndpointAuthorization{Scheme: strPtr(serviceEndpointAuthorizationWIF)}}, nil
		}},
	}

	if _, err := e.Create(context.Background(), serviceEndpointCR("", nil)); err != nil {
		t.Fatalf("e.Create(...): unexpected error: %v", err)
	}

	params := *got.Endpoint.Authorization.Parameters
	if _, ok := params[authParamServicePrincipalKey]; ok {
		t.Fatalf("e.Create(...): workload identity auth unexpectedly included %q", authParamServicePrincipalKey)
	}
	if gotScheme := *got.Endpoint.Authorization.Scheme; gotScheme != serviceEndpointAuthorizationWIF {
		t.Fatalf("e.Create(...): scheme = %q, want %q", gotScheme, serviceEndpointAuthorizationWIF)
	}
}

func TestCreateRejectsInvalidCredentials(t *testing.T) {
	e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{}}
	cr := serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointAzureRM) {
		cr.Spec.ForProvider.Credentials = v1alpha1.ServiceEndpointAzureRMCredentials{}
	})

	_, err := e.Create(context.Background(), cr)
	if err == nil || !strings.Contains(err.Error(), errInvalidCredentials) {
		t.Fatalf("e.Create(...): error = %v, want error containing %q", err, errInvalidCredentials)
	}
}

func TestUpdate(t *testing.T) {
	id := uuid.New()

	t.Run("MissingExternalName", func(t *testing.T) {
		e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{UpdateServiceEndpointFn: func(_ context.Context, _ adoserviceendpoint.UpdateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
			t.Fatal("UpdateServiceEndpoint should not be called when the endpoint id is missing")
			return nil, nil
		}}}

		if _, err := e.Update(context.Background(), serviceEndpointCR("", nil)); err == nil {
			t.Fatal("e.Update(...): expected error when external name is missing, got nil")
		}
	})

	t.Run("Success", func(t *testing.T) {
		var got adoserviceendpoint.UpdateServiceEndpointArgs
		e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{UpdateServiceEndpointFn: func(_ context.Context, args adoserviceendpoint.UpdateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
			got = args
			return &adoserviceendpoint.ServiceEndpoint{Id: &id, IsReady: boolPtr(true), Authorization: &adoserviceendpoint.EndpointAuthorization{Scheme: strPtr(serviceEndpointAuthorizationWIF)}}, nil
		}}}

		cr := serviceEndpointCR(id.String(), func(cr *v1alpha1.ServiceEndpointAzureRM) {
			cr.Spec.ForProvider.AzureSubscriptionName = "updated-subscription"
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("e.Update(...): unexpected error: %v", err)
		}

		if got.EndpointId == nil || *got.EndpointId != id {
			t.Fatalf("e.Update(...): endpoint id = %v, want %v", got.EndpointId, id)
		}
		if got.Endpoint == nil || got.Endpoint.Data == nil || (*got.Endpoint.Data)[dataKeySubscriptionName] != "updated-subscription" {
			t.Fatalf("e.Update(...): unexpected endpoint payload %#v", got.Endpoint)
		}
	})
}

func TestDelete(t *testing.T) {
	id := uuid.New()

	t.Run("NoExternalName", func(t *testing.T) {
		e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{DeleteServiceEndpointFn: func(_ context.Context, _ adoserviceendpoint.DeleteServiceEndpointArgs) error {
			t.Fatal("DeleteServiceEndpoint should not be called when the endpoint id is missing")
			return nil
		}}}

		if _, err := e.Delete(context.Background(), serviceEndpointCR("", nil)); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		var got adoserviceendpoint.DeleteServiceEndpointArgs
		e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{DeleteServiceEndpointFn: func(_ context.Context, args adoserviceendpoint.DeleteServiceEndpointArgs) error {
			got = args
			return nil
		}}}

		cr := serviceEndpointCR(id.String(), nil)
		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}

		if got.EndpointId == nil || *got.EndpointId != id {
			t.Fatalf("e.Delete(...): endpoint id = %v, want %v", got.EndpointId, id)
		}
		if got.ProjectIds == nil || len(*got.ProjectIds) != 1 || (*got.ProjectIds)[0] != cr.Spec.ForProvider.ProjectID {
			t.Fatalf("e.Delete(...): projectIds = %#v, want [%q]", got.ProjectIds, cr.Spec.ForProvider.ProjectID)
		}
	})

	t.Run("NotFoundIgnored", func(t *testing.T) {
		e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{DeleteServiceEndpointFn: func(_ context.Context, _ adoserviceendpoint.DeleteServiceEndpointArgs) error {
			return notFoundErr()
		}}}

		if _, err := e.Delete(context.Background(), serviceEndpointCR(id.String(), nil)); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
	})
}

func TestSecretRedaction(t *testing.T) {
	secretValue := "very-secret"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSPSecretName, Namespace: defaultNamespace},
		Data:       map[string][]byte{testSPSecretClientKey: []byte(secretValue)},
	}

	e := external{
		kube: newKube(t, secret),
		serviceEndpoint: &fake.ServiceEndpointClient{CreateServiceEndpointFn: func(_ context.Context, _ adoserviceendpoint.CreateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
			return nil, fmt.Errorf("create failed")
		}},
	}

	cr := serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointAzureRM) {
		cr.Spec.ForProvider.Credentials = v1alpha1.ServiceEndpointAzureRMCredentials{
			ServicePrincipal: &v1alpha1.ServicePrincipalCredentials{
				ClientID: "f1d1b03f-3cae-42ea-bb39-386f7ddc4076",
				ClientSecretRef: xpv2.SecretKeySelector{
					SecretReference: xpv2.SecretReference{Name: testSPSecretName, Namespace: defaultNamespace},
					Key:             testSPSecretClientKey,
				},
			},
		}
	})

	_, err := e.Create(context.Background(), cr)
	if err == nil {
		t.Fatal("e.Create(...): expected error, got nil")
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Fatalf("e.Create(...): error leaked secret %q: %v", secretValue, err)
	}

	endpointID := uuid.New()
	obs := observationFromServiceEndpoint(&adoserviceendpoint.ServiceEndpoint{
		Id:      &endpointID,
		IsReady: boolPtr(true),
		Authorization: &adoserviceendpoint.EndpointAuthorization{
			Scheme: strPtr(serviceEndpointAuthorizationServicePrinc),
			Parameters: &map[string]string{
				authParamServicePrincipalKey: secretValue,
			},
		},
	})
	if strings.Contains(fmt.Sprintf("%+v", obs), secretValue) {
		t.Fatalf("observation leaked secret %q: %+v", secretValue, obs)
	}
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func strPtr(s string) *string         { return &s }
func boolPtr(b bool) *bool            { return &b }
func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }
