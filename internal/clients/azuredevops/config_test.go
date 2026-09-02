// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package azuredevops

import (
	"context"
	"testing"

	fakemr "github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dunkin0486/provider-azuredevops/apis/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("corev1.AddToScheme() error = %v", err)
	}
	if err := v1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("v1alpha1.SchemeBuilder.AddToScheme() error = %v", err)
	}
	return s
}

func newManaged(namespace, pcKind, pcName string) *fakemr.ModernManaged {
	mg := &fakemr.ModernManaged{}
	mg.SetName("cool-resource")
	mg.SetNamespace(namespace)
	mg.SetUID(types.UID("cool-resource-uid"))
	mg.SetProviderConfigReference(&xpv2.ProviderConfigReference{Kind: pcKind, Name: pcName})
	return mg
}

func TestGetConfig(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "default"},
		Data:       map[string][]byte{"credentials": []byte("fake-pat")},
	}

	pc := &v1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: v1alpha1.ProviderConfigSpec{
			OrganizationURL: "https://dev.azure.com/example",
			Credentials: v1alpha1.ProviderCredentials{
				Source: xpv2.CredentialsSourceSecret,
				CommonCredentialSelectors: xpv2.CommonCredentialSelectors{
					SecretRef: &xpv2.SecretKeySelector{
						SecretReference: xpv2.SecretReference{Name: "creds", Namespace: "default"},
						Key:             "credentials",
					},
				},
			},
		},
	}

	cpc := &v1alpha1.ClusterProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-example"},
		Spec:       pc.Spec,
	}

	kube := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(secret, pc, cpc).
		Build()

	cases := map[string]struct {
		mg      *fakemr.ModernManaged
		wantURL string
		wantErr bool
	}{
		"NamespacedProviderConfig": {
			mg:      newManaged("default", v1alpha1.ProviderConfigKind, "example"),
			wantURL: "https://dev.azure.com/example",
		},
		"ClusterProviderConfig": {
			mg:      newManaged("default", v1alpha1.ClusterProviderConfigKind, "cluster-example"),
			wantURL: "https://dev.azure.com/example",
		},
		"UnknownKind": {
			mg:      newManaged("default", "Bogus", "example"),
			wantErr: true,
		},
		"MissingProviderConfig": {
			mg:      newManaged("default", v1alpha1.ProviderConfigKind, "does-not-exist"),
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := GetConfig(context.Background(), kube, tc.mg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("GetConfig() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("GetConfig() error = %v, want nil", err)
			}
			if cfg.OrganizationURL != tc.wantURL {
				t.Errorf("cfg.OrganizationURL = %q, want %q", cfg.OrganizationURL, tc.wantURL)
			}
			if cfg.Token != "fake-pat" {
				t.Errorf("cfg.Token = %q, want %q", cfg.Token, "fake-pat")
			}
			if conn := cfg.Connection(); conn == nil {
				t.Error("cfg.Connection() = nil, want non-nil")
			}
		})
	}
}

func TestGetConfigNoProviderConfigRef(t *testing.T) {
	kube := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()

	mg := &fakemr.ModernManaged{}
	mg.SetName("cool-resource")
	mg.SetNamespace("default")

	if _, err := GetConfig(context.Background(), kube, mg); err == nil {
		t.Fatal("GetConfig() error = nil, want non-nil")
	}
}
