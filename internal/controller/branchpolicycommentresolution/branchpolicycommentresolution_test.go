// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package branchpolicycommentresolution

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/policy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	runtimeTest "github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/branchpolicycommentresolution/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/branchpolicycommentresolution/fake"
)

const (
	testProjectID        = "11111111-1111-1111-1111-111111111111"
	testRepositoryID     = "22222222-2222-2222-2222-222222222222"
	testBranch           = "refs/heads/main"
	settingsScopeKey     = settingScope
	scopeRepositoryIDKey = "repositoryId"
	scopeRefNameKey      = "refName"
	scopeMatchKindKey    = "matchKind"
)

func branchPolicyCommentResolutionWith(externalName string, mutate func(*v1alpha1.BranchPolicyCommentResolution)) *v1alpha1.BranchPolicyCommentResolution {
	cr := &v1alpha1.BranchPolicyCommentResolution{}
	cr.Spec.ForProvider.ProjectID = testProjectID
	cr.Spec.ForProvider.RepositoryID = testRepositoryID
	cr.Spec.ForProvider.Branch = testBranch
	cr.Spec.ForProvider.Enabled = true
	cr.Spec.ForProvider.Blocking = true
	if externalName != "" {
		meta.SetExternalName(cr, externalName)
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

func policyNotFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func commentResolutionPolicy(id int, mutate func(*policy.PolicyConfiguration)) *policy.PolicyConfiguration {
	policyTypeID := commentResolutionPolicyTypeID
	enabled := true
	blocking := true
	enterpriseManaged := true
	revision := 3
	cfg := &policy.PolicyConfiguration{
		Id:                  intPtr(id),
		IsEnabled:           &enabled,
		IsBlocking:          &blocking,
		IsEnterpriseManaged: &enterpriseManaged,
		Revision:            &revision,
		Type: &policy.PolicyTypeRef{
			Id:          &policyTypeID,
			DisplayName: stringPtr(commentResolutionPolicyTypeName),
		},
		Settings: map[string]interface{}{
			settingsScopeKey: []map[string]interface{}{{
				scopeRepositoryIDKey: testRepositoryID,
				scopeRefNameKey:      testBranch,
				scopeMatchKindKey:    matchKindExact,
			}},
		},
	}
	if mutate != nil {
		mutate(cfg)
	}
	return cfg
}

func TestObserve(t *testing.T) {
	type fields struct {
		policies BranchPolicyCommentResolutionClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.BranchPolicyCommentResolution
	}
	type want struct {
		observation managed.ExternalObservation
		err         error
		condition   xpv2.ConditionType
		reason      xpv2.ConditionReason
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"NoExternalName": {
			reason: "Observe should report no external resource before creation sets an external name.",
			fields: fields{policies: &fake.BranchPolicyCommentResolutionClient{}},
			args:   args{cr: branchPolicyCommentResolutionWith("", nil)},
			want:   want{observation: managed.ExternalObservation{}},
		},
		"NotFoundAfterCreate": {
			reason: "Observe should report no resource exists when Azure DevOps returns 404 for a controller-created resource with a stored project annotation.",
			fields: fields{policies: &fake.BranchPolicyCommentResolutionClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return nil, policyNotFoundErr()
				},
			}},
			args: args{cr: branchPolicyCommentResolutionWith("18", func(cr *v1alpha1.BranchPolicyCommentResolution) {
				cr.SetAnnotations(map[string]string{annotationProjectID: testProjectID})
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: false}},
		},
		"NotFoundDuringImport": {
			reason: "Observe should fail import verification when an imported external name cannot be found before the original project identity has been established.",
			fields: fields{policies: &fake.BranchPolicyCommentResolutionClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return nil, policyNotFoundErr()
				},
			}},
			args: args{cr: branchPolicyCommentResolutionWith("18", nil)},
			want: want{err: stderrors.New(errImportProjectIDVerification)},
		},
		"UpToDate": {
			reason: "Observe should report the resource is up to date when the remote policy matches the desired spec fields.",
			fields: fields{policies: &fake.BranchPolicyCommentResolutionClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return commentResolutionPolicy(18, nil), nil
				},
			}},
			args: args{cr: branchPolicyCommentResolutionWith("18", nil)},
			want: want{
				observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				condition:   xpv2.TypeReady,
				reason:      xpv2.ReasonAvailable,
			},
		},
		"NeedsUpdate": {
			reason: "Observe should report drift when the desired branch scope differs from the remote policy.",
			fields: fields{policies: &fake.BranchPolicyCommentResolutionClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return commentResolutionPolicy(18, nil), nil
				},
			}},
			args: args{cr: branchPolicyCommentResolutionWith("18", func(cr *v1alpha1.BranchPolicyCommentResolution) {
				cr.Spec.ForProvider.Branch = "refs/heads/release/*"
			})},
			want: want{
				observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false},
				condition:   xpv2.TypeReady,
				reason:      xpv2.ReasonAvailable,
			},
		},
		"DeletedPolicy": {
			reason: "Observe should treat soft-deleted Azure DevOps policies as absent so Crossplane can recreate them.",
			fields: fields{policies: &fake.BranchPolicyCommentResolutionClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return commentResolutionPolicy(18, func(cfg *policy.PolicyConfiguration) {
						deleted := true
						cfg.IsDeleted = &deleted
					}), nil
				},
			}},
			args: args{cr: branchPolicyCommentResolutionWith("18", func(cr *v1alpha1.BranchPolicyCommentResolution) {
				cr.SetAnnotations(map[string]string{annotationProjectID: testProjectID})
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: false}},
		},
		"ImmutableProjectChange": {
			reason: "Observe should fail when the immutable projectId differs from the recorded creation-time project.",
			fields: fields{policies: &fake.BranchPolicyCommentResolutionClient{}},
			args: args{cr: branchPolicyCommentResolutionWith("18", func(cr *v1alpha1.BranchPolicyCommentResolution) {
				cr.Spec.ForProvider.ProjectID = "33333333-3333-3333-3333-333333333333"
				cr.SetAnnotations(map[string]string{annotationProjectID: testProjectID})
			})},
			want: want{err: stderrors.New(errImmutableProjectID)},
		},
		"MalformedSettings": {
			reason: "Observe should return a decode error when the imported policy settings payload cannot be interpreted.",
			fields: fields{policies: &fake.BranchPolicyCommentResolutionClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return commentResolutionPolicy(18, func(cfg *policy.PolicyConfiguration) {
						cfg.Settings = map[string]interface{}{settingsScopeKey: "invalid"}
					}), nil
				},
			}},
			args: args{cr: branchPolicyCommentResolutionWith("18", nil)},
			want: want{err: stderrors.New(errDecodePolicyConfiguration)},
		},
		"WrongPolicyType": {
			reason: "Observe should fail when the imported external resource is not the comment requirements policy type.",
			fields: fields{policies: &fake.BranchPolicyCommentResolutionClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					cfg := commentResolutionPolicy(18, func(cfg *policy.PolicyConfiguration) {
						otherID := uuidMustParse(t, "00000000-0000-0000-0000-000000000001")
						cfg.Type.Id = &otherID
					})
					return cfg, nil
				},
			}},
			args: args{cr: branchPolicyCommentResolutionWith("18", nil)},
			want: want{err: stderrors.New(errUnexpectedPolicyType)},
		},
		"UnsupportedImportedSettings": {
			reason: "Observe should reject imported policies with extra settings that this controller does not manage.",
			fields: fields{policies: &fake.BranchPolicyCommentResolutionClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return commentResolutionPolicy(18, func(cfg *policy.PolicyConfiguration) {
						cfg.Settings = map[string]interface{}{
							settingsScopeKey: []map[string]interface{}{{
								scopeRepositoryIDKey: testRepositoryID,
								scopeRefNameKey:      testBranch,
								scopeMatchKindKey:    matchKindExact,
							}},
							"unexpected": true,
						}
					}), nil
				},
			}},
			args: args{cr: branchPolicyCommentResolutionWith("18", nil)},
			want: want{err: stderrors.New(errUnsupportedPolicySettings)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.ctx == nil {
				tc.args.ctx = context.Background()
			}
			e := external{policies: tc.fields.policies}
			got, err := e.Observe(tc.args.ctx, tc.args.cr)
			if name == "MalformedSettings" {
				if err == nil || !strings.Contains(err.Error(), errDecodePolicyConfiguration) {
					t.Fatalf("\n%s\ne.Observe(...): expected error containing %q, got %v\n", tc.reason, errDecodePolicyConfiguration, err)
				}
			} else if diff := cmp.Diff(tc.want.err, err, runtimeTest.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.observation, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if tc.want.condition != "" {
				gotCondition := tc.args.cr.Status.GetCondition(tc.want.condition)
				if gotCondition.Reason != tc.want.reason {
					t.Errorf("\n%s\ne.Observe(...): condition %s reason = %q, want %q\n", tc.reason, tc.want.condition, gotCondition.Reason, tc.want.reason)
				}
			}
		})
	}
}

func TestCreate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		var gotCreate policy.CreatePolicyConfigurationArgs
		e := external{policies: &fake.BranchPolicyCommentResolutionClient{
			CreatePolicyConfigurationFn: func(_ context.Context, args policy.CreatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				gotCreate = args
				return commentResolutionPolicy(18, nil), nil
			},
		}}

		cr := branchPolicyCommentResolutionWith("", nil)

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("e.Create(...): unexpected error: %v", err)
		}
		if got := meta.GetExternalName(cr); got != "18" {
			t.Fatalf("external name = %q, want %q", got, "18")
		}
		if got := cr.GetAnnotations()[annotationProjectID]; got != testProjectID {
			t.Fatalf("project annotation = %q, want %q", got, testProjectID)
		}
		if gotCreate.Project == nil || *gotCreate.Project != testProjectID {
			t.Fatalf("CreatePolicyConfiguration project = %v, want %q", gotCreate.Project, testProjectID)
		}
		if gotCreate.Configuration == nil || gotCreate.Configuration.Type == nil || gotCreate.Configuration.Type.Id == nil || *gotCreate.Configuration.Type.Id != commentResolutionPolicyTypeID {
			t.Fatalf("CreatePolicyConfiguration called with unexpected type: %+v", gotCreate.Configuration)
		}

		settings, err := settingsFromConfiguration(gotCreate.Configuration.Settings)
		if err != nil {
			t.Fatalf("settingsFromConfiguration(...): unexpected error: %v", err)
		}
		if len(settings.Scope) != 1 || settings.Scope[0].RepositoryID != testRepositoryID || settings.Scope[0].RefName != testBranch || settings.Scope[0].MatchKind != matchKindExact {
			t.Fatalf("CreatePolicyConfiguration called with unexpected settings: %+v", settings)
		}
	})

	t.Run("MissingID", func(t *testing.T) {
		e := external{policies: &fake.BranchPolicyCommentResolutionClient{
			CreatePolicyConfigurationFn: func(_ context.Context, _ policy.CreatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				return &policy.PolicyConfiguration{}, nil
			},
		}}

		_, err := e.Create(context.Background(), branchPolicyCommentResolutionWith("", nil))
		if diff := cmp.Diff(stderrors.New(errCreatePolicyConfigurationNoID), err, runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Create(...): -want, +got:\n%s", diff)
		}
	})

	t.Run("CreateError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{policies: &fake.BranchPolicyCommentResolutionClient{
			CreatePolicyConfigurationFn: func(_ context.Context, _ policy.CreatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				return nil, wantErr
			},
		}}

		_, err := e.Create(context.Background(), branchPolicyCommentResolutionWith("", nil))
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Create(...): -want wrapped error, +got:\n%s", diff)
		}
	})
}

func TestUpdate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		var gotUpdate policy.UpdatePolicyConfigurationArgs
		e := external{policies: &fake.BranchPolicyCommentResolutionClient{
			GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				return commentResolutionPolicy(18, nil), nil
			},
			UpdatePolicyConfigurationFn: func(_ context.Context, args policy.UpdatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				gotUpdate = args
				return commentResolutionPolicy(18, func(cfg *policy.PolicyConfiguration) {
					falseValue := false
					cfg.IsEnabled = &falseValue
					cfg.Settings = map[string]interface{}{
						settingsScopeKey: []map[string]interface{}{{
							scopeRepositoryIDKey: testRepositoryID,
							scopeRefNameKey:      "refs/heads/release/",
							scopeMatchKindKey:    matchKindPrefix,
						}},
					}
				}), nil
			},
		}}

		cr := branchPolicyCommentResolutionWith("18", func(cr *v1alpha1.BranchPolicyCommentResolution) {
			cr.Spec.ForProvider.Enabled = false
			cr.Spec.ForProvider.Branch = "refs/heads/release/*"
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("e.Update(...): unexpected error: %v", err)
		}
		if gotUpdate.Project == nil || *gotUpdate.Project != testProjectID {
			t.Fatalf("UpdatePolicyConfiguration project = %v, want %q", gotUpdate.Project, testProjectID)
		}
		if gotUpdate.ConfigurationId == nil || *gotUpdate.ConfigurationId != 18 {
			t.Fatalf("UpdatePolicyConfiguration id = %v, want 18", gotUpdate.ConfigurationId)
		}

		settings, err := settingsFromConfiguration(gotUpdate.Configuration.Settings)
		if err != nil {
			t.Fatalf("settingsFromConfiguration(...): unexpected error: %v", err)
		}
		if len(settings.Scope) != 1 || settings.Scope[0].RefName != "refs/heads/release/" || settings.Scope[0].MatchKind != matchKindPrefix {
			t.Fatalf("UpdatePolicyConfiguration called with unexpected settings: %+v", settings)
		}
		if gotUpdate.Configuration.IsEnabled == nil || *gotUpdate.Configuration.IsEnabled {
			t.Fatalf("UpdatePolicyConfiguration IsEnabled = %v, want false", gotUpdate.Configuration.IsEnabled)
		}
	})

	t.Run("UpdateError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{policies: &fake.BranchPolicyCommentResolutionClient{
			GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				return commentResolutionPolicy(18, nil), nil
			},
			UpdatePolicyConfigurationFn: func(_ context.Context, _ policy.UpdatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				return nil, wantErr
			},
		}}

		_, err := e.Update(context.Background(), branchPolicyCommentResolutionWith("18", nil))
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Update(...): -want wrapped error, +got:\n%s", diff)
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		var gotDelete policy.DeletePolicyConfigurationArgs
		e := external{policies: &fake.BranchPolicyCommentResolutionClient{
			DeletePolicyConfigurationFn: func(_ context.Context, args policy.DeletePolicyConfigurationArgs) error {
				gotDelete = args
				return nil
			},
		}}
		cr := branchPolicyCommentResolutionWith("18", nil)
		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
		if gotDelete.ConfigurationId == nil || *gotDelete.ConfigurationId != 18 {
			t.Fatalf("DeletePolicyConfiguration id = %v, want 18", gotDelete.ConfigurationId)
		}
		if gotDelete.Project == nil || *gotDelete.Project != testProjectID {
			t.Fatalf("DeletePolicyConfiguration project = %v, want %q", gotDelete.Project, testProjectID)
		}
	})

	t.Run("NotFoundIgnored", func(t *testing.T) {
		e := external{policies: &fake.BranchPolicyCommentResolutionClient{
			DeletePolicyConfigurationFn: func(_ context.Context, _ policy.DeletePolicyConfigurationArgs) error {
				return policyNotFoundErr()
			},
		}}
		cr := branchPolicyCommentResolutionWith("18", nil)
		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
	})

	t.Run("DeleteError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{policies: &fake.BranchPolicyCommentResolutionClient{
			DeletePolicyConfigurationFn: func(_ context.Context, _ policy.DeletePolicyConfigurationArgs) error {
				return wantErr
			},
		}}
		cr := branchPolicyCommentResolutionWith("18", nil)
		_, err := e.Delete(context.Background(), cr)
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Delete(...): -want wrapped error, +got:\n%s", diff)
		}
	})
}

func uuidMustParse(t *testing.T, s string) uuid.UUID {
	t.Helper()

	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("uuid.Parse(%q): %v", s, err)
	}
	return id
}

func TestDeleteUsesStoredProjectIDDuringDeletion(t *testing.T) {
	var gotDelete policy.DeletePolicyConfigurationArgs
	e := external{policies: &fake.BranchPolicyCommentResolutionClient{
		DeletePolicyConfigurationFn: func(_ context.Context, args policy.DeletePolicyConfigurationArgs) error {
			gotDelete = args
			return nil
		},
	}}
	cr := branchPolicyCommentResolutionWith("18", func(cr *v1alpha1.BranchPolicyCommentResolution) {
		cr.Spec.ForProvider.ProjectID = "33333333-3333-3333-3333-333333333333"
		annotations := cr.GetAnnotations()
		annotations[annotationProjectID] = testProjectID
		cr.SetAnnotations(annotations)
		now := metav1.Now()
		cr.DeletionTimestamp = &now
	})
	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("e.Delete(...): unexpected error: %v", err)
	}
	if gotDelete.Project == nil || *gotDelete.Project != testProjectID {
		t.Fatalf("DeletePolicyConfiguration project = %v, want stored %q", gotDelete.Project, testProjectID)
	}
}
