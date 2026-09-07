// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package serviceendpointgeneric

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointgeneric/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/serviceendpointgeneric/fake"
)

const defaultNamespace = "default"

const (
	testSecretName = "generic-creds"
	testSecretKey  = "password"
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

func serviceEndpointCR(externalName string, mutate func(*v1alpha1.ServiceEndpointGeneric)) *v1alpha1.ServiceEndpointGeneric {
	cr := &v1alpha1.ServiceEndpointGeneric{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: defaultNamespace},
		Spec: v1alpha1.ServiceEndpointGenericSpec{
			ForProvider: v1alpha1.ServiceEndpointGenericParameters{
				Name:                "example-connection",
				ProjectID:           "8f571f72-7e56-4ec5-9485-6be503ca9763",
				ServerURL:           "https://api.example.com",
				AuthorizationScheme: serviceEndpointAuthorizationNone,
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

func endpointWith(id uuid.UUID, ready bool, scheme, url string, params map[string]string) *adoserviceendpoint.ServiceEndpoint {
	name := "example-connection"
	typ := "Generic"
	return &adoserviceendpoint.ServiceEndpoint{
		Id:   &id,
		Name: &name,
		Type: &typ,
		Url:  &url,
		Authorization: &adoserviceendpoint.EndpointAuthorization{
			Scheme:     &scheme,
			Parameters: &params,
		},
		Data:    &map[string]string{},
		IsReady: &ready,
	}
}

func passwordSecretRef() *xpv2.SecretKeySelector {
	return &xpv2.SecretKeySelector{
		SecretReference: xpv2.SecretReference{Name: testSecretName, Namespace: defaultNamespace},
		Key:             testSecretKey,
	}
}

func TestObserve(t *testing.T) {
	id := uuid.New()
	ready := true
	notReady := false

	type fields struct {
		client ServiceEndpointClient
		kube   client.Client
	}
	type args struct {
		cr *v1alpha1.ServiceEndpointGeneric
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
				return endpointWith(id, ready, serviceEndpointAuthorizationNone, "https://api.example.com", map[string]string{}), nil
			}}},
			args: args{cr: serviceEndpointCR(id.String(), nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.ReasonAvailable},
		},
		"NotUpToDate": {
			reason: "Observe should report not up to date when the server URL differs.",
			fields: fields{client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				return endpointWith(id, ready, serviceEndpointAuthorizationNone, "https://other.example.com", map[string]string{}), nil
			}}},
			args: args{cr: serviceEndpointCR(id.String(), nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.ReasonAvailable},
		},
		"NotReady": {
			reason: "Observe should surface Creating while the endpoint exists but is not ready.",
			fields: fields{client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				return endpointWith(id, notReady, serviceEndpointAuthorizationNone, "https://api.example.com", map[string]string{}), nil
			}}},
			args: args{cr: serviceEndpointCR(id.String(), nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.ReasonCreating},
		},
		"PasswordRotated": {
			reason: "Observe should report not up to date when the referenced Secret's value no longer matches the hash captured at last Create/Update, even though Azure DevOps' response is unchanged (it never returns the secret back).",
			fields: fields{
				client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
					return endpointWith(id, ready, serviceEndpointAuthorizationToken, "https://api.example.com", map[string]string{}), nil
				}},
				kube: newKube(t, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: defaultNamespace},
					Data:       map[string][]byte{testSecretKey: []byte("rotated-value")},
				}),
			},
			args: args{cr: serviceEndpointCR(id.String(), func(cr *v1alpha1.ServiceEndpointGeneric) {
				cr.Spec.ForProvider.AuthorizationScheme = serviceEndpointAuthorizationToken
				cr.Spec.ForProvider.PasswordSecretRef = passwordSecretRef()
				setPasswordHashAnnotation(cr, "original-value")
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.ReasonAvailable},
		},
		"GetError": {
			reason: "Observe should wrap Azure DevOps API errors.",
			fields: fields{client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				return nil, fmt.Errorf("boom")
			}}},
			args: args{cr: serviceEndpointCR(id.String(), nil)},
			want: want{err: errors.Wrap(fmt.Errorf("boom"), errGetServiceEndpoint)},
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

func TestCreate(t *testing.T) {
	secretValue := "super-secret-value"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: defaultNamespace},
		Data:       map[string][]byte{testSecretKey: []byte(secretValue)},
	}

	t.Run("UsernamePassword", func(t *testing.T) {
		var got adoserviceendpoint.CreateServiceEndpointArgs
		endpointID := uuid.New()
		e := external{
			kube: newKube(t, secret),
			serviceEndpoint: &fake.ServiceEndpointClient{CreateServiceEndpointFn: func(_ context.Context, args adoserviceendpoint.CreateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				got = args
				return &adoserviceendpoint.ServiceEndpoint{
					Id:            &endpointID,
					IsReady:       boolPtr(true),
					Authorization: &adoserviceendpoint.EndpointAuthorization{Scheme: strPtr(serviceEndpointAuthorizationUsernamePassword)},
				}, nil
			}},
		}

		cr := serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointGeneric) {
			cr.Spec.ForProvider.AuthorizationScheme = serviceEndpointAuthorizationUsernamePassword
			cr.Spec.ForProvider.Username = "my-user"
			cr.Spec.ForProvider.PasswordSecretRef = passwordSecretRef()
		})

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("e.Create(...): unexpected error: %v", err)
		}

		if got.Endpoint == nil || got.Endpoint.Authorization == nil || got.Endpoint.Authorization.Parameters == nil {
			t.Fatalf("e.Create(...): expected authorization parameters, got %#v", got.Endpoint)
		}
		params := *got.Endpoint.Authorization.Parameters
		if params[authParamUsername] != "my-user" {
			t.Fatalf("e.Create(...): username = %q, want %q", params[authParamUsername], "my-user")
		}
		if params[authParamPassword] != secretValue {
			t.Fatalf("e.Create(...): password = %q, want %q", params[authParamPassword], secretValue)
		}
		if gotScheme := *got.Endpoint.Authorization.Scheme; gotScheme != serviceEndpointAuthorizationUsernamePassword {
			t.Fatalf("e.Create(...): scheme = %q, want %q", gotScheme, serviceEndpointAuthorizationUsernamePassword)
		}
		if gotType := *got.Endpoint.Type; gotType != serviceEndpointTypeGeneric {
			t.Fatalf("e.Create(...): type = %q, want %q", gotType, serviceEndpointTypeGeneric)
		}
		if gotURL := *got.Endpoint.Url; gotURL != "https://api.example.com" {
			t.Fatalf("e.Create(...): url = %q, want %q", gotURL, "https://api.example.com")
		}
		if gotName := meta.GetExternalName(cr); gotName != endpointID.String() {
			t.Fatalf("e.Create(...): external name = %q, want %q", gotName, endpointID.String())
		}
		if got := cr.GetAnnotations()[annotationPasswordHash]; got != hashPassword(secretValue) {
			t.Fatalf("e.Create(...): password hash annotation = %q, want hash of %q", got, secretValue)
		}
	})

	t.Run("Token", func(t *testing.T) {
		var got adoserviceendpoint.CreateServiceEndpointArgs
		e := external{
			kube: newKube(t, secret),
			serviceEndpoint: &fake.ServiceEndpointClient{CreateServiceEndpointFn: func(_ context.Context, args adoserviceendpoint.CreateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				got = args
				return &adoserviceendpoint.ServiceEndpoint{
					Id:            uuidPtr(uuid.New()),
					IsReady:       boolPtr(true),
					Authorization: &adoserviceendpoint.EndpointAuthorization{Scheme: strPtr(serviceEndpointAuthorizationToken)},
				}, nil
			}},
		}

		cr := serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointGeneric) {
			cr.Spec.ForProvider.AuthorizationScheme = serviceEndpointAuthorizationToken
			cr.Spec.ForProvider.PasswordSecretRef = passwordSecretRef()
		})

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("e.Create(...): unexpected error: %v", err)
		}

		params := *got.Endpoint.Authorization.Parameters
		if params[authParamAPIToken] != secretValue {
			t.Fatalf("e.Create(...): apitoken = %q, want %q", params[authParamAPIToken], secretValue)
		}
		if _, ok := params[authParamUsername]; ok {
			t.Fatalf("e.Create(...): token auth unexpectedly included %q", authParamUsername)
		}
		if got := cr.GetAnnotations()[annotationPasswordHash]; got != hashPassword(secretValue) {
			t.Fatalf("e.Create(...): password hash annotation = %q, want hash of %q", got, secretValue)
		}
	})

	t.Run("None", func(t *testing.T) {
		var got adoserviceendpoint.CreateServiceEndpointArgs
		e := external{
			kube: newKube(t),
			serviceEndpoint: &fake.ServiceEndpointClient{CreateServiceEndpointFn: func(_ context.Context, args adoserviceendpoint.CreateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				got = args
				return &adoserviceendpoint.ServiceEndpoint{
					Id:            uuidPtr(uuid.New()),
					IsReady:       boolPtr(true),
					Authorization: &adoserviceendpoint.EndpointAuthorization{Scheme: strPtr(serviceEndpointAuthorizationNone)},
				}, nil
			}},
		}

		if _, err := e.Create(context.Background(), serviceEndpointCR("", nil)); err != nil {
			t.Fatalf("e.Create(...): unexpected error: %v", err)
		}

		params := *got.Endpoint.Authorization.Parameters
		if len(params) != 0 {
			t.Fatalf("e.Create(...): None auth params = %#v, want empty map", params)
		}
		if gotScheme := *got.Endpoint.Authorization.Scheme; gotScheme != serviceEndpointAuthorizationNone {
			t.Fatalf("e.Create(...): scheme = %q, want %q", gotScheme, serviceEndpointAuthorizationNone)
		}
	})
}

func TestCreateRejectsInvalidParameters(t *testing.T) {
	e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{}}
	cr := serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointGeneric) {
		cr.Spec.ForProvider.AuthorizationScheme = serviceEndpointAuthorizationToken
	})

	_, err := e.Create(context.Background(), cr)
	if err == nil || !strings.Contains(err.Error(), errMissingPasswordSecretRef) {
		t.Fatalf("e.Create(...): error = %v, want error containing %q", err, errMissingPasswordSecretRef)
	}
}

func TestUpdate(t *testing.T) {
	id := uuid.New()
	secretValue := "updated-secret"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: defaultNamespace},
		Data:       map[string][]byte{testSecretKey: []byte(secretValue)},
	}

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
		e := external{kube: newKube(t, secret), serviceEndpoint: &fake.ServiceEndpointClient{UpdateServiceEndpointFn: func(_ context.Context, args adoserviceendpoint.UpdateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
			got = args
			return &adoserviceendpoint.ServiceEndpoint{
				Id:            &id,
				IsReady:       boolPtr(true),
				Authorization: &adoserviceendpoint.EndpointAuthorization{Scheme: strPtr(serviceEndpointAuthorizationToken)},
			}, nil
		}}}

		cr := serviceEndpointCR(id.String(), func(cr *v1alpha1.ServiceEndpointGeneric) {
			cr.Spec.ForProvider.AuthorizationScheme = serviceEndpointAuthorizationToken
			cr.Spec.ForProvider.PasswordSecretRef = passwordSecretRef()
			cr.Spec.ForProvider.ServerURL = "https://updated.example.com"
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("e.Update(...): unexpected error: %v", err)
		}

		if got.EndpointId == nil || *got.EndpointId != id {
			t.Fatalf("e.Update(...): endpoint id = %v, want %v", got.EndpointId, id)
		}
		if got.Endpoint == nil || got.Endpoint.Url == nil || *got.Endpoint.Url != "https://updated.example.com" {
			t.Fatalf("e.Update(...): unexpected endpoint payload %#v", got.Endpoint)
		}
		params := *got.Endpoint.Authorization.Parameters
		if params[authParamAPIToken] != secretValue {
			t.Fatalf("e.Update(...): apitoken = %q, want %q", params[authParamAPIToken], secretValue)
		}
		if got := cr.GetAnnotations()[annotationPasswordHash]; got != hashPassword(secretValue) {
			t.Fatalf("e.Update(...): password hash annotation = %q, want hash of %q", got, secretValue)
		}
	})

	t.Run("Error", func(t *testing.T) {
		e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{UpdateServiceEndpointFn: func(_ context.Context, _ adoserviceendpoint.UpdateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
			return nil, fmt.Errorf("boom")
		}}}

		cr := serviceEndpointCR(id.String(), nil)
		if _, err := e.Update(context.Background(), cr); err == nil || !strings.Contains(err.Error(), errUpdateServiceEndpoint) {
			t.Fatalf("e.Update(...): error = %v, want wrapped %q", err, errUpdateServiceEndpoint)
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

	t.Run("Error", func(t *testing.T) {
		e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{DeleteServiceEndpointFn: func(_ context.Context, _ adoserviceendpoint.DeleteServiceEndpointArgs) error {
			return fmt.Errorf("boom")
		}}}

		if _, err := e.Delete(context.Background(), serviceEndpointCR(id.String(), nil)); err == nil || !strings.Contains(err.Error(), errDeleteServiceEndpoint) {
			t.Fatalf("e.Delete(...): error = %v, want wrapped %q", err, errDeleteServiceEndpoint)
		}
	})
}

func TestSecretRedaction(t *testing.T) {
	secretValue := "very-secret"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: defaultNamespace},
		Data:       map[string][]byte{testSecretKey: []byte(secretValue)},
	}

	e := external{
		kube: newKube(t, secret),
		serviceEndpoint: &fake.ServiceEndpointClient{CreateServiceEndpointFn: func(_ context.Context, _ adoserviceendpoint.CreateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
			return nil, fmt.Errorf("create failed")
		}},
	}

	cr := serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointGeneric) {
		cr.Spec.ForProvider.AuthorizationScheme = serviceEndpointAuthorizationToken
		cr.Spec.ForProvider.PasswordSecretRef = passwordSecretRef()
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
			Scheme: strPtr(serviceEndpointAuthorizationToken),
			Parameters: &map[string]string{
				authParamAPIToken: secretValue,
			},
		},
	})
	if strings.Contains(fmt.Sprintf("%+v", obs), secretValue) {
		t.Fatalf("observation leaked secret %q: %+v", secretValue, obs)
	}
}

func strPtr(s string) *string         { return &s }
func boolPtr(b bool) *bool            { return &b }
func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }
