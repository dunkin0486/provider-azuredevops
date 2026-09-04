// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package gitrepository

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	adogit "github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	runtimeTest "github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/gitrepository/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/gitrepository/fake"
)

const (
	defaultProjectID = "11111111-1111-1111-1111-111111111111"
	defaultBranchRef = "refs/heads/main"
)

func gitRepositoryWith(externalName string, mutate func(cr *v1alpha1.GitRepository)) *v1alpha1.GitRepository {
	cr := &v1alpha1.GitRepository{}
	cr.Spec.ForProvider.ProjectID = defaultProjectID
	cr.Spec.ForProvider.Name = "example-repository"
	if externalName != "" {
		meta.SetExternalName(cr, externalName)
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

func gitNotFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func repository(id uuid.UUID, mutate func(repo *adogit.GitRepository)) *adogit.GitRepository {
	projectID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &adogit.GitRepository{
		Id:            &id,
		Name:          stringPtr("example-repository"),
		Project:       &core.TeamProjectReference{Id: &projectID},
		DefaultBranch: stringPtr(defaultBranchRef),
		IsDisabled:    boolPtr(false),
		RemoteUrl:     stringPtr("https://dev.azure.com/example/project/_git/example-repository"),
		SshUrl:        stringPtr("git@ssh.dev.azure.com:v3/example/project/example-repository"),
		WebUrl:        stringPtr("https://dev.azure.com/example/project/_git/example-repository"),
	}
	if mutate != nil {
		mutate(repo)
	}
	return repo
}

func TestObserve(t *testing.T) {
	repoID := uuid.New()

	type fields struct {
		git GitRepositoryClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.GitRepository
	}
	type want struct {
		o         managed.ExternalObservation
		err       error
		condition xpv2.ConditionType
		reason    xpv2.ConditionReason
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"NoExternalName": {
			reason: "Observe should report no resource exists when the external name has never been set.",
			fields: fields{git: &fake.GitRepositoryClient{}},
			args:   args{cr: gitRepositoryWith("", nil)},
			want:   want{o: managed.ExternalObservation{}},
		},
		"NotFound": {
			reason: "Observe should report no resource exists when Azure DevOps returns 404.",
			fields: fields{git: &fake.GitRepositoryClient{GetRepositoryFn: func(_ context.Context, _ adogit.GetRepositoryArgs) (*adogit.GitRepository, error) {
				return nil, gitNotFoundErr()
			}}},
			args: args{cr: gitRepositoryWith(repoID.String(), nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: false}},
		},
		"UpToDate": {
			reason: "Observe should report the resource is up to date when mutable and immutable fields match desired state.",
			fields: fields{git: &fake.GitRepositoryClient{GetRepositoryFn: func(_ context.Context, _ adogit.GetRepositoryArgs) (*adogit.GitRepository, error) {
				return repository(repoID, nil), nil
			}}},
			args: args{cr: gitRepositoryWith(repoID.String(), func(cr *v1alpha1.GitRepository) {
				cr.Spec.ForProvider.DefaultBranch = defaultBranchRef
				cr.Spec.ForProvider.Disabled = boolPtr(false)
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
		"NeedsUpdate": {
			reason: "Observe should report the resource is not up to date when the repo name changes.",
			fields: fields{git: &fake.GitRepositoryClient{GetRepositoryFn: func(_ context.Context, _ adogit.GetRepositoryArgs) (*adogit.GitRepository, error) {
				return repository(repoID, nil), nil
			}}},
			args: args{cr: gitRepositoryWith(repoID.String(), func(cr *v1alpha1.GitRepository) {
				cr.Spec.ForProvider.Name = "renamed-repository"
				cr.Spec.ForProvider.DefaultBranch = defaultBranchRef
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{git: tc.fields.git}
			got, err := e.Observe(tc.args.ctx, tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, runtimeTest.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
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
	projectID := defaultProjectID
	repoID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		var createArgs adogit.CreateRepositoryArgs
		var updateArgs adogit.UpdateRepositoryArgs
		e := external{git: &fake.GitRepositoryClient{
			CreateRepositoryFn: func(_ context.Context, args adogit.CreateRepositoryArgs) (*adogit.GitRepository, error) {
				createArgs = args
				return repository(repoID, func(repo *adogit.GitRepository) {
					repo.DefaultBranch = nil
					repo.IsDisabled = nil
				}), nil
			},
			UpdateRepositoryFn: func(_ context.Context, args adogit.UpdateRepositoryArgs) (*adogit.GitRepository, error) {
				updateArgs = args
				return repository(repoID, func(repo *adogit.GitRepository) {
					repo.Name = stringPtr("example-repository")
					repo.DefaultBranch = stringPtr("refs/heads/main")
					repo.IsDisabled = boolPtr(true)
				}), nil
			},
		}}

		cr := gitRepositoryWith("", func(cr *v1alpha1.GitRepository) {
			cr.Spec.ForProvider.ProjectID = projectID
			cr.Spec.ForProvider.DefaultBranch = defaultBranchRef
			cr.Spec.ForProvider.Disabled = boolPtr(true)
		})

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("e.Create(...): unexpected error: %v", err)
		}
		if got := meta.GetExternalName(cr); got != repoID.String() {
			t.Fatalf("external name = %q, want %q", got, repoID.String())
		}
		if createArgs.Project == nil || *createArgs.Project != projectID {
			t.Fatalf("CreateRepository project = %v, want %q", createArgs.Project, projectID)
		}
		if createArgs.GitRepositoryToCreate == nil || createArgs.GitRepositoryToCreate.Name == nil || *createArgs.GitRepositoryToCreate.Name != "example-repository" {
			t.Fatalf("CreateRepository called with unexpected payload: %+v", createArgs.GitRepositoryToCreate)
		}
		// The freshly-created repository has no commits (DefaultBranch is
		// nil), so Azure DevOps would reject setting defaultBranch on it.
		// The controller must skip it here and let a later reconcile set it
		// once the repository actually has a branch.
		if updateArgs.NewRepositoryInfo == nil || updateArgs.NewRepositoryInfo.DefaultBranch != nil {
			t.Fatalf("UpdateRepository unexpectedly set defaultBranch on an empty repository: %+v", updateArgs.NewRepositoryInfo)
		}
		if updateArgs.NewRepositoryInfo.IsDisabled == nil || !*updateArgs.NewRepositoryInfo.IsDisabled {
			t.Fatalf("UpdateRepository did not set disabled=true: %+v", updateArgs.NewRepositoryInfo)
		}
	})

	t.Run("SetsDefaultBranchWhenRepositoryAlreadyHasCommits", func(t *testing.T) {
		var updateArgs adogit.UpdateRepositoryArgs
		e := external{git: &fake.GitRepositoryClient{
			CreateRepositoryFn: func(_ context.Context, _ adogit.CreateRepositoryArgs) (*adogit.GitRepository, error) {
				// Simulate creating a repository from a parent (fork), which
				// already has commits and a default branch.
				return repository(repoID, func(repo *adogit.GitRepository) {
					repo.DefaultBranch = stringPtr("refs/heads/master")
					repo.IsDisabled = nil
				}), nil
			},
			UpdateRepositoryFn: func(_ context.Context, args adogit.UpdateRepositoryArgs) (*adogit.GitRepository, error) {
				updateArgs = args
				return repository(repoID, func(repo *adogit.GitRepository) {
					repo.DefaultBranch = stringPtr(defaultBranchRef)
				}), nil
			},
		}}

		cr := gitRepositoryWith("", func(cr *v1alpha1.GitRepository) {
			cr.Spec.ForProvider.ProjectID = projectID
			cr.Spec.ForProvider.DefaultBranch = defaultBranchRef
		})

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("e.Create(...): unexpected error: %v", err)
		}
		if updateArgs.NewRepositoryInfo == nil || updateArgs.NewRepositoryInfo.DefaultBranch == nil || *updateArgs.NewRepositoryInfo.DefaultBranch != defaultBranchRef {
			t.Fatalf("UpdateRepository called with unexpected patch: %+v", updateArgs.NewRepositoryInfo)
		}
	})

	t.Run("CreateError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{git: &fake.GitRepositoryClient{CreateRepositoryFn: func(_ context.Context, _ adogit.CreateRepositoryArgs) (*adogit.GitRepository, error) {
			return nil, wantErr
		}}}

		_, err := e.Create(context.Background(), gitRepositoryWith("", nil))
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Create(...): -want wrapped error, +got:\n%s", diff)
		}
	})
}

func TestUpdate(t *testing.T) {
	repoID := uuid.New()

	t.Run("RenameSuccess", func(t *testing.T) {
		var gotUpdate adogit.UpdateRepositoryArgs
		e := external{git: &fake.GitRepositoryClient{
			GetRepositoryFn: func(_ context.Context, _ adogit.GetRepositoryArgs) (*adogit.GitRepository, error) {
				return repository(repoID, nil), nil
			},
			UpdateRepositoryFn: func(_ context.Context, args adogit.UpdateRepositoryArgs) (*adogit.GitRepository, error) {
				gotUpdate = args
				return repository(repoID, func(repo *adogit.GitRepository) {
					repo.Name = stringPtr("renamed-repository")
				}), nil
			},
		}}

		cr := gitRepositoryWith(repoID.String(), func(cr *v1alpha1.GitRepository) {
			cr.Spec.ForProvider.Name = "renamed-repository"
			cr.Spec.ForProvider.DefaultBranch = defaultBranchRef
			cr.Spec.ForProvider.Disabled = boolPtr(false)
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("e.Update(...): unexpected error: %v", err)
		}
		if gotUpdate.NewRepositoryInfo == nil || gotUpdate.NewRepositoryInfo.Name == nil || *gotUpdate.NewRepositoryInfo.Name != "renamed-repository" {
			t.Fatalf("UpdateRepository patch = %+v, want renamed-repository", gotUpdate.NewRepositoryInfo)
		}
	})

	t.Run("SkipsDefaultBranchOnEmptyRepository", func(t *testing.T) {
		var gotUpdate adogit.UpdateRepositoryArgs
		called := false
		e := external{git: &fake.GitRepositoryClient{
			GetRepositoryFn: func(_ context.Context, _ adogit.GetRepositoryArgs) (*adogit.GitRepository, error) {
				return repository(repoID, func(repo *adogit.GitRepository) {
					repo.DefaultBranch = nil
				}), nil
			},
			UpdateRepositoryFn: func(_ context.Context, args adogit.UpdateRepositoryArgs) (*adogit.GitRepository, error) {
				called = true
				gotUpdate = args
				return repository(repoID, func(repo *adogit.GitRepository) {
					repo.DefaultBranch = nil
				}), nil
			},
		}}

		cr := gitRepositoryWith(repoID.String(), func(cr *v1alpha1.GitRepository) {
			cr.Spec.ForProvider.DefaultBranch = defaultBranchRef
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("e.Update(...): unexpected error: %v", err)
		}
		if called && gotUpdate.NewRepositoryInfo != nil && gotUpdate.NewRepositoryInfo.DefaultBranch != nil {
			t.Fatalf("UpdateRepository unexpectedly set defaultBranch on an empty repository: %+v", gotUpdate.NewRepositoryInfo)
		}
	})

	t.Run("ImmutableProjectChange", func(t *testing.T) {
		e := external{git: &fake.GitRepositoryClient{
			GetRepositoryFn: func(_ context.Context, _ adogit.GetRepositoryArgs) (*adogit.GitRepository, error) {
				return repository(repoID, nil), nil
			},
			UpdateRepositoryFn: func(_ context.Context, _ adogit.UpdateRepositoryArgs) (*adogit.GitRepository, error) {
				t.Fatal("UpdateRepository should not be called when immutable fields change")
				return nil, nil
			},
		}}

		cr := gitRepositoryWith(repoID.String(), func(cr *v1alpha1.GitRepository) {
			cr.Spec.ForProvider.ProjectID = "33333333-3333-3333-3333-333333333333"
		})

		if _, err := e.Update(context.Background(), cr); err == nil {
			t.Fatal("e.Update(...): expected error for immutable project change, got nil")
		}
	})

	t.Run("GetRepositoryError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{git: &fake.GitRepositoryClient{
			GetRepositoryFn: func(_ context.Context, _ adogit.GetRepositoryArgs) (*adogit.GitRepository, error) {
				return nil, wantErr
			},
		}}

		_, err := e.Update(context.Background(), gitRepositoryWith(repoID.String(), nil))
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Update(...): -want wrapped error, +got:\n%s", diff)
		}
	})
}

func TestDelete(t *testing.T) {
	repoID := uuid.New()

	t.Run("NoObservedID", func(t *testing.T) {
		e := external{git: &fake.GitRepositoryClient{DeleteRepositoryFn: func(_ context.Context, _ adogit.DeleteRepositoryArgs) error {
			t.Fatal("DeleteRepository should not be called when there is no repository id")
			return nil
		}}}
		cr := gitRepositoryWith("", nil)
		cr.Status.AtProvider.ID = ""
		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		var gotDelete adogit.DeleteRepositoryArgs
		e := external{git: &fake.GitRepositoryClient{DeleteRepositoryFn: func(_ context.Context, args adogit.DeleteRepositoryArgs) error {
			gotDelete = args
			return nil
		}}}
		cr := gitRepositoryWith(repoID.String(), nil)
		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
		if gotDelete.RepositoryId == nil || *gotDelete.RepositoryId != repoID {
			t.Fatalf("DeleteRepository repository id = %v, want %v", gotDelete.RepositoryId, repoID)
		}
	})

	t.Run("NotFoundIgnored", func(t *testing.T) {
		e := external{git: &fake.GitRepositoryClient{DeleteRepositoryFn: func(_ context.Context, _ adogit.DeleteRepositoryArgs) error {
			return gitNotFoundErr()
		}}}
		cr := gitRepositoryWith(repoID.String(), nil)
		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
	})

	t.Run("DeleteError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{git: &fake.GitRepositoryClient{DeleteRepositoryFn: func(_ context.Context, _ adogit.DeleteRepositoryArgs) error {
			return wantErr
		}}}
		cr := gitRepositoryWith(repoID.String(), nil)
		_, err := e.Delete(context.Background(), cr)
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Delete(...): -want wrapped error, +got:\n%s", diff)
		}
	})
}
