// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package serviceendpointkubernetes

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	runtimeTest "github.com/crossplane/crossplane-runtime/v2/pkg/test"
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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/serviceendpointkubernetes/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/serviceendpointkubernetes/fake"
)

const defaultNamespace = "default"

const (
	testKubeconfigSecretName = "kubeconfig-creds"
	testKubeconfigSecretKey  = "kubeconfig"
	testTokenSecretName      = "service-account-creds"
	testTokenSecretKey       = "token"
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

func kubeconfigSecretRef() *xpv2.SecretKeySelector {
	return &xpv2.SecretKeySelector{
		SecretReference: xpv2.SecretReference{Name: testKubeconfigSecretName, Namespace: defaultNamespace},
		Key:             testKubeconfigSecretKey,
	}
}

func serviceAccountTokenSecretRef() *xpv2.SecretKeySelector {
	return &xpv2.SecretKeySelector{
		SecretReference: xpv2.SecretReference{Name: testTokenSecretName, Namespace: defaultNamespace},
		Key:             testTokenSecretKey,
	}
}

func serviceEndpointCR(externalName string, mutate func(*v1alpha1.ServiceEndpointKubernetes)) *v1alpha1.ServiceEndpointKubernetes {
	cr := &v1alpha1.ServiceEndpointKubernetes{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: defaultNamespace},
		Spec: v1alpha1.ServiceEndpointKubernetesSpec{
			ForProvider: v1alpha1.ServiceEndpointKubernetesParameters{
				Name:                         "example-kubernetes-connection",
				ProjectID:                    "8f571f72-7e56-4ec5-9485-6be503ca9763",
				ClusterServer:                "https://cluster.example.com",
				AuthorizationType:            authorizationTypeServiceAccount,
				ServiceAccountTokenSecretRef: serviceAccountTokenSecretRef(),
				ClusterCACertificate:         "ca-data",
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

func endpointWith(id uuid.UUID, ready bool, scheme, authType string, mutate func(*adoserviceendpoint.ServiceEndpoint)) *adoserviceendpoint.ServiceEndpoint {
	name := "example-kubernetes-connection"
	typ := serviceEndpointTypeKubernetes
	url := "https://cluster.example.com"
	params := map[string]string{}
	data := map[string]string{dataKeyAuthorizationType: authType}
	if authType == authorizationTypeServiceAccount {
		data[dataKeyAcceptUntrustedCerts] = "false"
		params[authParamServiceAccountCert] = "ca-data"
	}
	endpoint := &adoserviceendpoint.ServiceEndpoint{
		Id:   &id,
		Name: &name,
		Type: &typ,
		Url:  &url,
		Authorization: &adoserviceendpoint.EndpointAuthorization{
			Scheme:     &scheme,
			Parameters: &params,
		},
		Data:    &data,
		IsReady: &ready,
	}
	if mutate != nil {
		mutate(endpoint)
	}
	return endpoint
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
		cr *v1alpha1.ServiceEndpointKubernetes
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
				return endpointWith(id, ready, authSchemeToken, authorizationTypeServiceAccount, nil), nil
			}}, kube: newKube(t, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: testTokenSecretName, Namespace: defaultNamespace},
				Data:       map[string][]byte{testTokenSecretKey: []byte("super-secret-value")},
			})},
			args: args{cr: serviceEndpointCR(id.String(), func(cr *v1alpha1.ServiceEndpointKubernetes) {
				setAuthSecretHashAnnotation(cr, "super-secret-value")
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.ReasonAvailable},
		},
		"NotUpToDate": {
			reason: "Observe should report not up to date when the authorization type differs.",
			fields: fields{client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				return endpointWith(id, ready, authSchemeKubernetes, authorizationTypeKubeconfig, nil), nil
			}}},
			args: args{cr: serviceEndpointCR(id.String(), nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.ReasonAvailable},
		},
		"NotReady": {
			reason: "Observe should surface Creating while the endpoint exists but is not ready.",
			fields: fields{client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				return endpointWith(id, notReady, authSchemeToken, authorizationTypeServiceAccount, nil), nil
			}}, kube: newKube(t, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: testTokenSecretName, Namespace: defaultNamespace},
				Data:       map[string][]byte{testTokenSecretKey: []byte("super-secret-value")},
			})},
			args: args{cr: serviceEndpointCR(id.String(), func(cr *v1alpha1.ServiceEndpointKubernetes) {
				setAuthSecretHashAnnotation(cr, "super-secret-value")
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.ReasonCreating},
		},
		"SecretRotated": {
			reason: "Observe should report not up to date when the referenced Secret's value no longer matches the hash captured at last Create/Update.",
			fields: fields{
				client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
					return endpointWith(id, ready, authSchemeToken, authorizationTypeServiceAccount, nil), nil
				}},
				kube: newKube(t, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: testTokenSecretName, Namespace: defaultNamespace},
					Data:       map[string][]byte{testTokenSecretKey: []byte("rotated-value")},
				}),
			},
			args: args{cr: serviceEndpointCR(id.String(), func(cr *v1alpha1.ServiceEndpointKubernetes) {
				setAuthSecretHashAnnotation(cr, "original-value")
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.ReasonAvailable},
		},
		"DeletingSkipsSecretDriftCheck": {
			reason: "Observe should skip secret drift lookups during deletion so finalization can continue even if the referenced Secret is already gone.",
			fields: fields{client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				return endpointWith(id, ready, authSchemeToken, authorizationTypeServiceAccount, nil), nil
			}}},
			args: args{cr: serviceEndpointCR(id.String(), func(cr *v1alpha1.ServiceEndpointKubernetes) {
				now := metav1.Now()
				cr.SetDeletionTimestamp(&now)
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.ReasonAvailable},
		},
		"GetError": {
			reason: "Observe should wrap service endpoint API errors.",
			fields: fields{client: &fake.ServiceEndpointClient{GetServiceEndpointDetailsFn: func(_ context.Context, _ adoserviceendpoint.GetServiceEndpointDetailsArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				return nil, fmt.Errorf("boom")
			}}},
			args: args{cr: serviceEndpointCR(id.String(), nil)},
			want: want{err: fmt.Errorf("boom")},
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
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			gotErr := err
			if tc.want.err != nil {
				gotErr = stderrors.Unwrap(err)
			}
			if diff := cmp.Diff(tc.want.err, gotErr, runtimeTest.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want wrapped error, +got wrapped error:\n%s\n", tc.reason, diff)
			}
			if gotReason := tc.args.cr.Status.GetCondition(xpv2.TypeReady).Reason; gotReason != tc.want.condition {
				t.Errorf("\n%s\ne.Observe(...): ready reason = %q, want %q\n", tc.reason, gotReason, tc.want.condition)
			}
		})
	}
}

func TestCreateServiceAccount(t *testing.T) {
	secretValue := "super-secret-value"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testTokenSecretName, Namespace: defaultNamespace},
		Data:       map[string][]byte{testTokenSecretKey: []byte(secretValue)},
	}

	var got adoserviceendpoint.CreateServiceEndpointArgs
	endpointID := uuid.New()
	e := external{
		kube: newKube(t, secret),
		serviceEndpoint: &fake.ServiceEndpointClient{CreateServiceEndpointFn: func(_ context.Context, args adoserviceendpoint.CreateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
			got = args
			return &adoserviceendpoint.ServiceEndpoint{Id: &endpointID, IsReady: boolPtr(true)}, nil
		}},
	}

	cr := serviceEndpointCR("", nil)

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("e.Create(...): unexpected error: %v", err)
	}

	if got.Endpoint == nil || got.Endpoint.Authorization == nil || got.Endpoint.Authorization.Parameters == nil {
		t.Fatalf("e.Create(...): expected authorization parameters, got %#v", got.Endpoint)
	}
	params := *got.Endpoint.Authorization.Parameters
	if params[authParamAPIToken] != secretValue {
		t.Fatalf("e.Create(...): apiToken = %q, want %q", params[authParamAPIToken], secretValue)
	}
	if params[authParamServiceAccountCert] != "ca-data" {
		t.Fatalf("e.Create(...): serviceAccountCertificate = %q, want %q", params[authParamServiceAccountCert], "ca-data")
	}
	if gotScheme := *got.Endpoint.Authorization.Scheme; gotScheme != authSchemeToken {
		t.Fatalf("e.Create(...): scheme = %q, want %q", gotScheme, authSchemeToken)
	}
	if gotType := *got.Endpoint.Type; gotType != serviceEndpointTypeKubernetes {
		t.Fatalf("e.Create(...): type = %q, want %q", gotType, serviceEndpointTypeKubernetes)
	}
	if gotURL := *got.Endpoint.Url; gotURL != cr.Spec.ForProvider.ClusterServer {
		t.Fatalf("e.Create(...): url = %q, want %q", gotURL, cr.Spec.ForProvider.ClusterServer)
	}
	if gotData := (*got.Endpoint.Data)[dataKeyAuthorizationType]; gotData != authorizationTypeServiceAccount {
		t.Fatalf("e.Create(...): authorizationType = %q, want %q", gotData, authorizationTypeServiceAccount)
	}
	if gotData := (*got.Endpoint.Data)[dataKeyAcceptUntrustedCerts]; gotData != "false" {
		t.Fatalf("e.Create(...): acceptUntrustedCerts = %q, want false", gotData)
	}
	if gotName := meta.GetExternalName(cr); gotName != endpointID.String() {
		t.Fatalf("e.Create(...): external name = %q, want %q", gotName, endpointID.String())
	}
	if got := cr.GetAnnotations()[annotationAuthSecretHash]; got != hashSecret(secretValue) {
		t.Fatalf("e.Create(...): auth secret hash annotation = %q, want hash of %q", got, secretValue)
	}
}

func TestCreateServiceAccountRequiresCACert(t *testing.T) {
	e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{}}
	cr := serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointKubernetes) {
		cr.Spec.ForProvider.ClusterCACertificate = ""
	})

	_, err := e.Create(context.Background(), cr)
	if err == nil || !strings.Contains(err.Error(), errMissingClusterCACertificate) {
		t.Fatalf("e.Create(...): error = %v, want error containing %q", err, errMissingClusterCACertificate)
	}
}

func TestCreateKubeconfig(t *testing.T) {
	secretValue := "apiVersion: v1\nclusters: []\n"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testKubeconfigSecretName, Namespace: defaultNamespace},
		Data:       map[string][]byte{testKubeconfigSecretKey: []byte(secretValue)},
	}

	var got adoserviceendpoint.CreateServiceEndpointArgs
	e := external{
		kube: newKube(t, secret),
		serviceEndpoint: &fake.ServiceEndpointClient{CreateServiceEndpointFn: func(_ context.Context, args adoserviceendpoint.CreateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
			got = args
			return &adoserviceendpoint.ServiceEndpoint{Id: uuidPtr(uuid.New()), IsReady: boolPtr(true)}, nil
		}},
	}

	cr := serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointKubernetes) {
		cr.Spec.ForProvider.AuthorizationType = authorizationTypeKubeconfig
		cr.Spec.ForProvider.ServiceAccountTokenSecretRef = nil
		cr.Spec.ForProvider.KubeconfigSecretRef = kubeconfigSecretRef()
		cr.Spec.ForProvider.ClusterCACertificate = "ignored-for-kubeconfig"
	})

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("e.Create(...): unexpected error: %v", err)
	}
	if got.Endpoint == nil || got.Endpoint.Authorization == nil || got.Endpoint.Authorization.Parameters == nil {
		t.Fatalf("e.Create(...): expected authorization parameters, got %#v", got.Endpoint)
	}
	params := *got.Endpoint.Authorization.Parameters
	if params[authParamKubeconfig] != secretValue {
		t.Fatalf("e.Create(...): kubeconfig = %q, want %q", params[authParamKubeconfig], secretValue)
	}
	if gotScheme := *got.Endpoint.Authorization.Scheme; gotScheme != authSchemeKubernetes {
		t.Fatalf("e.Create(...): scheme = %q, want %q", gotScheme, authSchemeKubernetes)
	}
	if gotData := (*got.Endpoint.Data)[dataKeyAuthorizationType]; gotData != authorizationTypeKubeconfig {
		t.Fatalf("e.Create(...): authorizationType = %q, want %q", gotData, authorizationTypeKubeconfig)
	}
	if got := cr.GetAnnotations()[annotationAuthSecretHash]; got != hashSecret(secretValue) {
		t.Fatalf("e.Create(...): auth secret hash annotation = %q, want hash of %q", got, secretValue)
	}
}

func TestCreateValidation(t *testing.T) {
	e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{}}

	cases := map[string]struct {
		cr      *v1alpha1.ServiceEndpointKubernetes
		wantErr string
	}{
		"MissingServiceAccountTokenSecretRef": {
			cr: serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointKubernetes) {
				cr.Spec.ForProvider.ServiceAccountTokenSecretRef = nil
			}),
			wantErr: errMissingServiceAccountTokenSecret,
		},
		"MissingClusterCACertificate": {
			cr: serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointKubernetes) {
				cr.Spec.ForProvider.ClusterCACertificate = ""
			}),
			wantErr: errMissingClusterCACertificate,
		},
		"MissingKubeconfigSecretRef": {
			cr: serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointKubernetes) {
				cr.Spec.ForProvider.AuthorizationType = authorizationTypeKubeconfig
				cr.Spec.ForProvider.ServiceAccountTokenSecretRef = nil
			}),
			wantErr: errMissingKubeconfigSecretRef,
		},
		"AzureSubscriptionUnsupported": {
			cr: serviceEndpointCR("", func(cr *v1alpha1.ServiceEndpointKubernetes) {
				cr.Spec.ForProvider.AuthorizationType = authorizationTypeAzureSubscription
				cr.Spec.ForProvider.ServiceAccountTokenSecretRef = nil
			}),
			wantErr: errAzureSubscriptionUnsupported,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := e.Create(context.Background(), tc.cr)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("e.Create(...): error = %v, want error containing %q", err, tc.wantErr)
			}
		})
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
		secretValue := "apiVersion: v1\nclusters: []\n"
		e := external{
			kube: newKube(t, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: testKubeconfigSecretName, Namespace: defaultNamespace},
				Data:       map[string][]byte{testKubeconfigSecretKey: []byte(secretValue)},
			}),
			serviceEndpoint: &fake.ServiceEndpointClient{UpdateServiceEndpointFn: func(_ context.Context, args adoserviceendpoint.UpdateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				got = args
				return &adoserviceendpoint.ServiceEndpoint{Id: &id, IsReady: boolPtr(true)}, nil
			}},
		}

		cr := serviceEndpointCR(id.String(), func(cr *v1alpha1.ServiceEndpointKubernetes) {
			cr.Spec.ForProvider.AuthorizationType = authorizationTypeKubeconfig
			cr.Spec.ForProvider.ServiceAccountTokenSecretRef = nil
			cr.Spec.ForProvider.KubeconfigSecretRef = kubeconfigSecretRef()
			cr.Spec.ForProvider.ClusterCACertificate = ""
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("e.Update(...): unexpected error: %v", err)
		}

		if got.EndpointId == nil || *got.EndpointId != id {
			t.Fatalf("e.Update(...): endpoint id = %v, want %v", got.EndpointId, id)
		}
		if got.Endpoint == nil || got.Endpoint.Authorization == nil || got.Endpoint.Authorization.Parameters == nil || (*got.Endpoint.Authorization.Parameters)[authParamKubeconfig] != secretValue {
			t.Fatalf("e.Update(...): unexpected endpoint payload %#v", got.Endpoint)
		}
	})

	t.Run("UpdateError", func(t *testing.T) {
		secretValue := "updated-token"
		e := external{
			kube: newKube(t, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: testTokenSecretName, Namespace: defaultNamespace},
				Data:       map[string][]byte{testTokenSecretKey: []byte(secretValue)},
			}),
			serviceEndpoint: &fake.ServiceEndpointClient{UpdateServiceEndpointFn: func(_ context.Context, _ adoserviceendpoint.UpdateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
				return nil, fmt.Errorf("boom")
			}},
		}

		_, err := e.Update(context.Background(), serviceEndpointCR(id.String(), nil))
		if diff := cmp.Diff(fmt.Errorf("boom"), stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Update(...): -want error, +got error:\n%s", diff)
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

	t.Run("DeleteError", func(t *testing.T) {
		e := external{kube: newKube(t), serviceEndpoint: &fake.ServiceEndpointClient{DeleteServiceEndpointFn: func(_ context.Context, _ adoserviceendpoint.DeleteServiceEndpointArgs) error {
			return fmt.Errorf("boom")
		}}}

		_, err := e.Delete(context.Background(), serviceEndpointCR(id.String(), nil))
		if diff := cmp.Diff(fmt.Errorf("boom"), stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Delete(...): -want error, +got error:\n%s", diff)
		}
	})
}

func TestSecretRedaction(t *testing.T) {
	secretValue := "very-secret"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testTokenSecretName, Namespace: defaultNamespace},
		Data:       map[string][]byte{testTokenSecretKey: []byte(secretValue)},
	}
	createErr := fmt.Errorf("create failed")

	e := external{
		kube: newKube(t, secret),
		serviceEndpoint: &fake.ServiceEndpointClient{CreateServiceEndpointFn: func(_ context.Context, _ adoserviceendpoint.CreateServiceEndpointArgs) (*adoserviceendpoint.ServiceEndpoint, error) {
			return nil, createErr
		}},
	}

	cr := serviceEndpointCR("", nil)

	_, err := e.Create(context.Background(), cr)
	if err == nil {
		t.Fatal("e.Create(...): expected error, got nil")
	}
	if diff := cmp.Diff(createErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
		t.Fatalf("e.Create(...): -want error, +got error:\n%s", diff)
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Fatalf("e.Create(...): error leaked secret %q: %v", secretValue, err)
	}
	if strings.Contains(fmt.Sprintf("%+v", cr.Status.AtProvider), secretValue) {
		t.Fatalf("status leaked secret %q: %+v", secretValue, cr.Status.AtProvider)
	}
}

func boolPtr(b bool) *bool            { return &b }
func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }
