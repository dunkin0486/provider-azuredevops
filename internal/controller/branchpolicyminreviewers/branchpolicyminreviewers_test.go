// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package branchpolicyminreviewers

import (
	"context"
	stderrors "errors"
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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/branchpolicyminreviewers/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/branchpolicyminreviewers/fake"
)

const (
	testProjectID                   = "11111111-1111-1111-1111-111111111111"
	testRepositoryID                = "22222222-2222-2222-2222-222222222222"
	testBranch                      = "refs/heads/main"
	settingsAllowDownvotesKey       = settingAllowDownvotes
	settingsCreatorVoteCountsKey    = settingCreatorVoteCounts
	settingsMinimumApproverCountKey = settingMinimumApproverCount
	settingsResetOnSourcePushKey    = settingResetOnSourcePush
	settingsScopeKey                = settingScope
	scopeRepositoryIDKey            = "repositoryId"
	scopeRefNameKey                 = "refName"
	scopeMatchKindKey               = "matchKind"
)

func branchPolicyMinReviewersWith(externalName string, mutate func(*v1alpha1.BranchPolicyMinReviewers)) *v1alpha1.BranchPolicyMinReviewers {
	cr := &v1alpha1.BranchPolicyMinReviewers{}
	cr.Spec.ForProvider.ProjectID = testProjectID
	cr.Spec.ForProvider.RepositoryID = testRepositoryID
	cr.Spec.ForProvider.Branch = testBranch
	cr.Spec.ForProvider.MinimumApproverCount = 2
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

func minimumReviewerPolicy(id int, mutate func(*policy.PolicyConfiguration)) *policy.PolicyConfiguration {
	policyTypeID := minReviewerPolicyTypeID
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
			DisplayName: stringPtr(minReviewerPolicyTypeDisplayName),
		},
		Settings: map[string]interface{}{
			settingsMinimumApproverCountKey: 2,
			settingsCreatorVoteCountsKey:    true,
			settingsAllowDownvotesKey:       false,
			settingsResetOnSourcePushKey:    true,
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
		policies BranchPolicyMinReviewersClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.BranchPolicyMinReviewers
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
			fields: fields{policies: &fake.BranchPolicyMinReviewersClient{}},
			args:   args{cr: branchPolicyMinReviewersWith("", nil)},
			want:   want{observation: managed.ExternalObservation{}},
		},
		"NotFoundAfterCreate": {
			reason: "Observe should report no resource exists when Azure DevOps returns 404 for a controller-created resource with a stored project annotation.",
			fields: fields{policies: &fake.BranchPolicyMinReviewersClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return nil, policyNotFoundErr()
				},
			}},
			args: args{cr: branchPolicyMinReviewersWith("18", func(cr *v1alpha1.BranchPolicyMinReviewers) {
				cr.SetAnnotations(map[string]string{annotationProjectID: testProjectID})
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: false}},
		},
		"NotFoundDuringImport": {
			reason: "Observe should fail import verification when an imported external name cannot be found before the original project identity has been established.",
			fields: fields{policies: &fake.BranchPolicyMinReviewersClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return nil, policyNotFoundErr()
				},
			}},
			args: args{cr: branchPolicyMinReviewersWith("18", nil)},
			want: want{err: stderrors.New(errImportProjectIDVerification)},
		},
		"UpToDate": {
			reason: "Observe should report the resource is up to date when the remote policy matches the desired spec fields.",
			fields: fields{policies: &fake.BranchPolicyMinReviewersClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return minimumReviewerPolicy(18, nil), nil
				},
			}},
			args: args{cr: branchPolicyMinReviewersWith("18", func(cr *v1alpha1.BranchPolicyMinReviewers) {
				cr.Spec.ForProvider.CreatorVoteCounts = true
				cr.Spec.ForProvider.ResetOnSourcePush = true
			})},
			want: want{
				observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				condition:   xpv2.TypeReady,
				reason:      xpv2.ReasonAvailable,
			},
		},
		"NeedsUpdate": {
			reason: "Observe should report drift when mutable policy settings differ.",
			fields: fields{policies: &fake.BranchPolicyMinReviewersClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return minimumReviewerPolicy(18, nil), nil
				},
			}},
			args: args{cr: branchPolicyMinReviewersWith("18", func(cr *v1alpha1.BranchPolicyMinReviewers) {
				cr.Spec.ForProvider.CreatorVoteCounts = true
				cr.Spec.ForProvider.AllowCompletionWithRejects = true
				cr.Spec.ForProvider.ResetOnSourcePush = true
			})},
			want: want{
				observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false},
				condition:   xpv2.TypeReady,
				reason:      xpv2.ReasonAvailable,
			},
		},
		"WrongPolicyType": {
			reason: "Observe should fail when the imported external resource is not the minimum approval count policy type.",
			fields: fields{policies: &fake.BranchPolicyMinReviewersClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					cfg := minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
						otherID := uuidMustParse(t, "00000000-0000-0000-0000-000000000001")
						cfg.Type.Id = &otherID
					})
					return cfg, nil
				},
			}},
			args: args{cr: branchPolicyMinReviewersWith("18", nil)},
			want: want{err: stderrors.New(errUnexpectedPolicyType)},
		},
		"UnsupportedImportedScope": {
			reason: "Observe should fail rather than silently narrowing an imported policy with unsupported scope shape.",
			fields: fields{policies: &fake.BranchPolicyMinReviewersClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
						cfg.Settings = map[string]interface{}{
							settingsMinimumApproverCountKey: 2,
							settingsCreatorVoteCountsKey:    true,
							settingsAllowDownvotesKey:       false,
							settingsResetOnSourcePushKey:    true,
							settingsScopeKey: []map[string]interface{}{
								{scopeRepositoryIDKey: testRepositoryID, scopeRefNameKey: testBranch, scopeMatchKindKey: matchKindExact},
								{scopeRepositoryIDKey: testRepositoryID, scopeRefNameKey: "refs/heads/release/", scopeMatchKindKey: matchKindPrefix},
							},
						}
					}), nil
				},
			}},
			args: args{cr: branchPolicyMinReviewersWith("18", func(cr *v1alpha1.BranchPolicyMinReviewers) {
				cr.Spec.ForProvider.CreatorVoteCounts = true
				cr.Spec.ForProvider.ResetOnSourcePush = true
			})},
			want: want{err: stderrors.New(errUnsupportedPolicyScope)},
		},
		"SoftDeletedPolicy": {
			reason: "Observe should treat soft-deleted Azure DevOps policies as absent so Crossplane can recreate them.",
			fields: fields{policies: &fake.BranchPolicyMinReviewersClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
						deleted := true
						cfg.IsDeleted = &deleted
					}), nil
				},
			}},
			args: args{cr: branchPolicyMinReviewersWith("18", func(cr *v1alpha1.BranchPolicyMinReviewers) {
				cr.SetAnnotations(map[string]string{annotationProjectID: testProjectID})
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: false}},
		},
		"UnsupportedFlagRejected": {
			reason: "Observe should fail rather than silently weakening an unsupported imported policy.",
			fields: fields{policies: &fake.BranchPolicyMinReviewersClient{
				GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
					return minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
						cfg.Settings = map[string]interface{}{
							settingsMinimumApproverCountKey: 2,
							settingsCreatorVoteCountsKey:    true,
							settingsAllowDownvotesKey:       false,
							settingsResetOnSourcePushKey:    true,
							settingBlockLastPusherVote:      true,
							settingsScopeKey:                []map[string]interface{}{{scopeRepositoryIDKey: testRepositoryID, scopeRefNameKey: testBranch, scopeMatchKindKey: matchKindExact}},
						}
					}), nil
				},
			}},
			args: args{cr: branchPolicyMinReviewersWith("18", func(cr *v1alpha1.BranchPolicyMinReviewers) {
				cr.Spec.ForProvider.CreatorVoteCounts = true
				cr.Spec.ForProvider.ResetOnSourcePush = true
			})},
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
			if diff := cmp.Diff(tc.want.err, err, runtimeTest.EquateErrors()); diff != "" {
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
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			CreatePolicyConfigurationFn: func(_ context.Context, args policy.CreatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				gotCreate = args
				return minimumReviewerPolicy(18, nil), nil
			},
		}}

		cr := branchPolicyMinReviewersWith("", func(cr *v1alpha1.BranchPolicyMinReviewers) {
			cr.Spec.ForProvider.CreatorVoteCounts = true
			cr.Spec.ForProvider.ResetOnSourcePush = true
		})

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
		if gotCreate.Configuration == nil || gotCreate.Configuration.Type == nil || gotCreate.Configuration.Type.Id == nil || *gotCreate.Configuration.Type.Id != minReviewerPolicyTypeID {
			t.Fatalf("CreatePolicyConfiguration called with unexpected type: %+v", gotCreate.Configuration)
		}

		settings, err := settingsFromConfiguration(gotCreate.Configuration.Settings)
		if err != nil {
			t.Fatalf("settingsFromConfiguration(...): unexpected error: %v", err)
		}
		if settings.MinimumApproverCount != 2 || !settings.CreatorVoteCounts || !settings.ResetOnSourcePush || len(settings.Scope) != 1 || settings.Scope[0].RepositoryID != testRepositoryID || settings.Scope[0].RefName != testBranch || settings.Scope[0].MatchKind != matchKindExact {
			t.Fatalf("CreatePolicyConfiguration called with unexpected settings: %+v", settings)
		}
	})

	t.Run("WildcardBranchUsesPrefixMatch", func(t *testing.T) {
		var gotCreate policy.CreatePolicyConfigurationArgs
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			CreatePolicyConfigurationFn: func(_ context.Context, args policy.CreatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				gotCreate = args
				return minimumReviewerPolicy(18, nil), nil
			},
		}}

		cr := branchPolicyMinReviewersWith("", func(cr *v1alpha1.BranchPolicyMinReviewers) {
			cr.Spec.ForProvider.Branch = "refs/heads/release/*"
		})

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("e.Create(...): unexpected error: %v", err)
		}

		settings, err := settingsFromConfiguration(gotCreate.Configuration.Settings)
		if err != nil {
			t.Fatalf("settingsFromConfiguration(...): unexpected error: %v", err)
		}
		if len(settings.Scope) != 1 || settings.Scope[0].RefName != "refs/heads/release/" || settings.Scope[0].MatchKind != matchKindPrefix {
			t.Fatalf("unexpected wildcard scope: %+v", settings.Scope)
		}
	})

	t.Run("CreateError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			CreatePolicyConfigurationFn: func(_ context.Context, _ policy.CreatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				return nil, wantErr
			},
		}}

		_, err := e.Create(context.Background(), branchPolicyMinReviewersWith("", nil))
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Create(...): -want wrapped error, +got:\n%s", diff)
		}
	})
}

func TestUpdate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		var gotUpdate policy.UpdatePolicyConfigurationArgs
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				return minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
					cfg.Settings = map[string]interface{}{
						settingsMinimumApproverCountKey: 2,
						settingsCreatorVoteCountsKey:    false,
						settingsAllowDownvotesKey:       false,
						settingsResetOnSourcePushKey:    false,
						settingsScopeKey:                []map[string]interface{}{{scopeRepositoryIDKey: testRepositoryID, scopeRefNameKey: testBranch, scopeMatchKindKey: matchKindExact}},
					}
				}), nil
			},
			UpdatePolicyConfigurationFn: func(_ context.Context, args policy.UpdatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				gotUpdate = args
				return minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
					cfg.Settings = map[string]interface{}{
						settingsMinimumApproverCountKey: 3,
						settingsCreatorVoteCountsKey:    true,
						settingsAllowDownvotesKey:       true,
						settingsResetOnSourcePushKey:    true,
						settingsScopeKey: []map[string]interface{}{{
							scopeRepositoryIDKey: testRepositoryID,
							scopeRefNameKey:      testBranch,
							scopeMatchKindKey:    matchKindExact,
						}},
					}
				}), nil
			},
		}}

		cr := branchPolicyMinReviewersWith("18", func(cr *v1alpha1.BranchPolicyMinReviewers) {
			cr.Spec.ForProvider.MinimumApproverCount = 3
			cr.Spec.ForProvider.CreatorVoteCounts = true
			cr.Spec.ForProvider.AllowCompletionWithRejects = true
			cr.Spec.ForProvider.ResetOnSourcePush = true
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

		settingsMap, err := settingsMapFrom(gotUpdate.Configuration.Settings)
		if err != nil {
			t.Fatalf("settingsMapFrom(...): unexpected error: %v", err)
		}
		if _, found := settingsMap[settingBlockLastPusherVote]; found {
			t.Fatalf("UpdatePolicyConfiguration unexpectedly included unsupported settings: %+v", settingsMap)
		}

		settings, err := settingsFromConfiguration(gotUpdate.Configuration.Settings)
		if err != nil {
			t.Fatalf("settingsFromConfiguration(...): unexpected error: %v", err)
		}
		if settings.MinimumApproverCount != 3 || !settings.CreatorVoteCounts || !settings.AllowDownvotes || !settings.ResetOnSourcePush {
			t.Fatalf("UpdatePolicyConfiguration called with unexpected managed settings: %+v", settings)
		}
	})

	t.Run("GetPolicyError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				return nil, wantErr
			},
		}}

		_, err := e.Update(context.Background(), branchPolicyMinReviewersWith("18", nil))
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Update(...): -want wrapped error, +got:\n%s", diff)
		}
	})

	t.Run("UnsupportedFlagRejected", func(t *testing.T) {
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				return minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
					cfg.Settings = map[string]interface{}{
						settingsMinimumApproverCountKey: 2,
						settingsCreatorVoteCountsKey:    false,
						settingsAllowDownvotesKey:       false,
						settingsResetOnSourcePushKey:    false,
						settingBlockLastPusherVote:      true,
						settingsScopeKey:                []map[string]interface{}{{scopeRepositoryIDKey: testRepositoryID, scopeRefNameKey: testBranch, scopeMatchKindKey: matchKindExact}},
					}
				}), nil
			},
			UpdatePolicyConfigurationFn: func(_ context.Context, _ policy.UpdatePolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				t.Fatal("UpdatePolicyConfiguration should not be called for unsupported imported policy settings")
				return nil, nil
			},
		}}

		if _, err := e.Update(context.Background(), branchPolicyMinReviewersWith("18", nil)); err == nil {
			t.Fatal("e.Update(...): expected error for unsupported imported policy settings, got nil")
		}
	})

	t.Run("ImmutableProjectChange", func(t *testing.T) {
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			GetPolicyConfigurationFn: func(_ context.Context, _ policy.GetPolicyConfigurationArgs) (*policy.PolicyConfiguration, error) {
				t.Fatal("GetPolicyConfiguration should not be called when projectId changes")
				return nil, nil
			},
		}}

		cr := branchPolicyMinReviewersWith("18", func(cr *v1alpha1.BranchPolicyMinReviewers) {
			cr.Spec.ForProvider.ProjectID = "33333333-3333-3333-3333-333333333333"
			cr.SetAnnotations(map[string]string{annotationProjectID: testProjectID})
		})

		if _, err := e.Update(context.Background(), cr); err == nil {
			t.Fatal("e.Update(...): expected error for immutable project change, got nil")
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("NoObservedID", func(t *testing.T) {
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			DeletePolicyConfigurationFn: func(_ context.Context, _ policy.DeletePolicyConfigurationArgs) error {
				t.Fatal("DeletePolicyConfiguration should not be called when there is no configuration id")
				return nil
			},
		}}
		cr := branchPolicyMinReviewersWith("", nil)
		cr.Status.AtProvider.ID = ""
		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		var gotDelete policy.DeletePolicyConfigurationArgs
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			DeletePolicyConfigurationFn: func(_ context.Context, args policy.DeletePolicyConfigurationArgs) error {
				gotDelete = args
				return nil
			},
		}}
		cr := branchPolicyMinReviewersWith("18", nil)
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

	t.Run("UsesStoredProjectIDDuringDeletion", func(t *testing.T) {
		var gotDelete policy.DeletePolicyConfigurationArgs
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			DeletePolicyConfigurationFn: func(_ context.Context, args policy.DeletePolicyConfigurationArgs) error {
				gotDelete = args
				return nil
			},
		}}
		cr := branchPolicyMinReviewersWith("18", func(cr *v1alpha1.BranchPolicyMinReviewers) {
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
	})

	t.Run("NotFoundIgnored", func(t *testing.T) {
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			DeletePolicyConfigurationFn: func(_ context.Context, _ policy.DeletePolicyConfigurationArgs) error {
				return policyNotFoundErr()
			},
		}}
		cr := branchPolicyMinReviewersWith("18", nil)
		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
	})

	t.Run("DeleteError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{policies: &fake.BranchPolicyMinReviewersClient{
			DeletePolicyConfigurationFn: func(_ context.Context, _ policy.DeletePolicyConfigurationArgs) error {
				return wantErr
			},
		}}
		cr := branchPolicyMinReviewersWith("18", nil)
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

func TestGetProjectID(t *testing.T) {
	got, err := getProjectID(branchPolicyMinReviewersWith("", nil))
	if err != nil {
		t.Fatalf("getProjectID(...): unexpected error: %v", err)
	}
	if got != testProjectID {
		t.Fatalf("getProjectID(...) = %q, want %q", got, testProjectID)
	}

	if _, err := getProjectID(branchPolicyMinReviewersWith("", func(cr *v1alpha1.BranchPolicyMinReviewers) { cr.Spec.ForProvider.ProjectID = "" })); err == nil {
		t.Fatal("getProjectID(...): expected error when projectId is empty, got nil")
	}
}

func TestIsUpToDate(t *testing.T) {
	desired := branchPolicyMinReviewersWith("", func(cr *v1alpha1.BranchPolicyMinReviewers) {
		cr.Spec.ForProvider.CreatorVoteCounts = true
		cr.Spec.ForProvider.ResetOnSourcePush = true
	}).Spec.ForProvider

	cases := map[string]struct {
		current *policy.PolicyConfiguration
		want    bool
		wantErr bool
	}{
		"NilCurrent": {current: nil, want: false},
		"UpToDate":   {current: minimumReviewerPolicy(18, nil), want: true},
		"NotEnabled": {
			current: minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
				enabled := false
				cfg.IsEnabled = &enabled
			}),
			want: false,
		},
		"NotBlocking": {
			current: minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
				blocking := false
				cfg.IsBlocking = &blocking
			}),
			want: false,
		},
		"SettingsMismatch": {
			current: minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
				cfg.Settings.(map[string]interface{})[settingsMinimumApproverCountKey] = 5
			}),
			want: false,
		},
		"UnexpectedPolicyType": {
			current: minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
				otherID := uuid.New()
				cfg.Type.Id = &otherID
			}),
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := isUpToDate(desired, tc.current)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("isUpToDate(...): expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("isUpToDate(...): unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("isUpToDate(...) = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatchesTopLevelConfiguration(t *testing.T) {
	desired := branchPolicyMinReviewersWith("", nil).Spec.ForProvider

	cases := map[string]struct {
		current *policy.PolicyConfiguration
		want    bool
	}{
		"Matches": {current: minimumReviewerPolicy(18, nil), want: true},
		"NilIsEnabled": {
			current: minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) { cfg.IsEnabled = nil }),
			want:    false,
		},
		"EnabledMismatch": {
			current: minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
				enabled := false
				cfg.IsEnabled = &enabled
			}),
			want: false,
		},
		"NilIsBlocking": {
			current: minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) { cfg.IsBlocking = nil }),
			want:    false,
		},
		"BlockingMismatch": {
			current: minimumReviewerPolicy(18, func(cfg *policy.PolicyConfiguration) {
				blocking := false
				cfg.IsBlocking = &blocking
			}),
			want: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := matchesTopLevelConfiguration(desired, tc.current); got != tc.want {
				t.Fatalf("matchesTopLevelConfiguration(...) = %v, want %v", got, tc.want)
			}
		})
	}
}
