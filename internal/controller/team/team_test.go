// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package team

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	runtimeTest "github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/team/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/team/fake"
)

const (
	defaultProjectID = "11111111-1111-1111-1111-111111111111"
	teamDescription  = "Example team"
)

func teamWith(externalName string, mutate func(cr *v1alpha1.Team)) *v1alpha1.Team {
	cr := &v1alpha1.Team{}
	cr.Spec.ForProvider.ProjectID = defaultProjectID
	cr.Spec.ForProvider.Name = "example-team"
	if externalName != "" {
		meta.SetExternalName(cr, externalName)
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

func teamNotFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func webAPITeam(id uuid.UUID, mutate func(team *core.WebApiTeam)) *core.WebApiTeam {
	projectID := uuid.MustParse(defaultProjectID)
	team := &core.WebApiTeam{
		Id:          &id,
		Name:        stringPtr("example-team"),
		Description: stringPtr(teamDescription),
		ProjectId:   &projectID,
		Url:         stringPtr("https://dev.azure.com/example/_apis/projects/project/teams/team"),
	}
	if mutate != nil {
		mutate(team)
	}
	return team
}

func TestObserve(t *testing.T) {
	teamID := uuid.New()

	type fields struct {
		teams TeamClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.Team
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
			reason: "Observe should report no external resource when the team has never been created.",
			fields: fields{teams: &fake.TeamClient{}},
			args:   args{cr: teamWith("", nil)},
			want:   want{o: managed.ExternalObservation{}},
		},
		"NotFound": {
			reason: "Observe should report no external resource when Azure DevOps returns 404.",
			fields: fields{teams: &fake.TeamClient{
				GetTeamFn: func(_ context.Context, _ core.GetTeamArgs) (*core.WebApiTeam, error) {
					return nil, teamNotFoundErr()
				},
			}},
			args: args{cr: teamWith(teamID.String(), nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: false}},
		},
		"UpToDate": {
			reason: "Observe should report the team is up to date when mutable and immutable fields match desired state.",
			fields: fields{teams: &fake.TeamClient{
				GetTeamFn: func(_ context.Context, _ core.GetTeamArgs) (*core.WebApiTeam, error) {
					return webAPITeam(teamID, nil), nil
				},
			}},
			args: args{cr: teamWith(teamID.String(), func(cr *v1alpha1.Team) {
				cr.Spec.ForProvider.Description = teamDescription
			})},
			want: want{
				o:         managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				condition: xpv2.TypeReady,
				reason:    xpv2.ReasonAvailable,
			},
		},
		"NeedsUpdate": {
			reason: "Observe should report the team is not up to date when the desired description differs.",
			fields: fields{teams: &fake.TeamClient{
				GetTeamFn: func(_ context.Context, _ core.GetTeamArgs) (*core.WebApiTeam, error) {
					return webAPITeam(teamID, nil), nil
				},
			}},
			args: args{cr: teamWith(teamID.String(), func(cr *v1alpha1.Team) {
				cr.Spec.ForProvider.Description = "Updated team description"
			})},
			want: want{
				o:         managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false},
				condition: xpv2.TypeReady,
				reason:    xpv2.ReasonAvailable,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.ctx == nil {
				tc.args.ctx = context.Background()
			}
			e := external{teams: tc.fields.teams}
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
	teamID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		var createArgs core.CreateTeamArgs
		e := external{teams: &fake.TeamClient{
			CreateTeamFn: func(_ context.Context, args core.CreateTeamArgs) (*core.WebApiTeam, error) {
				createArgs = args
				return webAPITeam(teamID, nil), nil
			},
		}}

		cr := teamWith("", func(cr *v1alpha1.Team) {
			cr.Spec.ForProvider.Description = teamDescription
		})

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("e.Create(...): unexpected error: %v", err)
		}
		if got := meta.GetExternalName(cr); got != teamID.String() {
			t.Fatalf("external name = %q, want %q", got, teamID.String())
		}
		if createArgs.ProjectId == nil || *createArgs.ProjectId != defaultProjectID {
			t.Fatalf("CreateTeam project = %v, want %q", createArgs.ProjectId, defaultProjectID)
		}
		if createArgs.Team == nil || createArgs.Team.Name == nil || *createArgs.Team.Name != "example-team" {
			t.Fatalf("CreateTeam called with unexpected payload: %+v", createArgs.Team)
		}
		if createArgs.Team.Description == nil || *createArgs.Team.Description != teamDescription {
			t.Fatalf("CreateTeam called with unexpected description payload: %+v", createArgs.Team)
		}
	})

	t.Run("CreateError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{teams: &fake.TeamClient{
			CreateTeamFn: func(_ context.Context, _ core.CreateTeamArgs) (*core.WebApiTeam, error) {
				return nil, wantErr
			},
		}}

		_, err := e.Create(context.Background(), teamWith("", nil))
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Create(...): -want wrapped error, +got:\n%s", diff)
		}
	})
}

func TestUpdate(t *testing.T) {
	teamID := uuid.New()

	t.Run("RenameSuccess", func(t *testing.T) {
		var gotUpdate core.UpdateTeamArgs
		e := external{teams: &fake.TeamClient{
			GetTeamFn: func(_ context.Context, _ core.GetTeamArgs) (*core.WebApiTeam, error) {
				return webAPITeam(teamID, nil), nil
			},
			UpdateTeamFn: func(_ context.Context, args core.UpdateTeamArgs) (*core.WebApiTeam, error) {
				gotUpdate = args
				return webAPITeam(teamID, func(team *core.WebApiTeam) {
					team.Name = stringPtr("renamed-team")
				}), nil
			},
		}}

		cr := teamWith(teamID.String(), func(cr *v1alpha1.Team) {
			cr.Spec.ForProvider.Name = "renamed-team"
			cr.Spec.ForProvider.Description = teamDescription
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("e.Update(...): unexpected error: %v", err)
		}
		if gotUpdate.TeamData == nil || gotUpdate.TeamData.Name == nil || *gotUpdate.TeamData.Name != "renamed-team" {
			t.Fatalf("UpdateTeam patch = %+v, want renamed-team", gotUpdate.TeamData)
		}
	})

	t.Run("NoChanges", func(t *testing.T) {
		e := external{teams: &fake.TeamClient{
			GetTeamFn: func(_ context.Context, _ core.GetTeamArgs) (*core.WebApiTeam, error) {
				return webAPITeam(teamID, nil), nil
			},
			UpdateTeamFn: func(_ context.Context, _ core.UpdateTeamArgs) (*core.WebApiTeam, error) {
				t.Fatal("UpdateTeam should not be called when nothing changed")
				return nil, nil
			},
		}}

		cr := teamWith(teamID.String(), func(cr *v1alpha1.Team) {
			cr.Spec.ForProvider.Description = teamDescription
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("e.Update(...): unexpected error: %v", err)
		}
	})

	t.Run("ImmutableProjectChange", func(t *testing.T) {
		e := external{teams: &fake.TeamClient{
			GetTeamFn: func(_ context.Context, _ core.GetTeamArgs) (*core.WebApiTeam, error) {
				return webAPITeam(teamID, nil), nil
			},
			UpdateTeamFn: func(_ context.Context, _ core.UpdateTeamArgs) (*core.WebApiTeam, error) {
				t.Fatal("UpdateTeam should not be called when immutable fields change")
				return nil, nil
			},
		}}

		cr := teamWith(teamID.String(), func(cr *v1alpha1.Team) {
			cr.Spec.ForProvider.ProjectID = "33333333-3333-3333-3333-333333333333"
		})

		if _, err := e.Update(context.Background(), cr); err == nil {
			t.Fatal("e.Update(...): expected error for immutable project change, got nil")
		}
	})

	t.Run("MissingTeamID", func(t *testing.T) {
		e := external{teams: &fake.TeamClient{
			UpdateTeamFn: func(_ context.Context, _ core.UpdateTeamArgs) (*core.WebApiTeam, error) {
				t.Fatal("UpdateTeam should not be called when there is no team id")
				return nil, nil
			},
		}}

		if _, err := e.Update(context.Background(), teamWith("", nil)); err == nil {
			t.Fatal("e.Update(...): expected error when there is no team id, got nil")
		}
	})
}

func TestDelete(t *testing.T) {
	teamID := uuid.New()

	t.Run("NoObservedID", func(t *testing.T) {
		e := external{teams: &fake.TeamClient{
			DeleteTeamFn: func(_ context.Context, _ core.DeleteTeamArgs) error {
				t.Fatal("DeleteTeam should not be called when there is no team id")
				return nil
			},
		}}

		if _, err := e.Delete(context.Background(), teamWith("", nil)); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		var gotDelete core.DeleteTeamArgs
		e := external{teams: &fake.TeamClient{
			DeleteTeamFn: func(_ context.Context, args core.DeleteTeamArgs) error {
				gotDelete = args
				return nil
			},
		}}

		if _, err := e.Delete(context.Background(), teamWith(teamID.String(), nil)); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
		if gotDelete.ProjectId == nil || *gotDelete.ProjectId != defaultProjectID {
			t.Fatalf("DeleteTeam project = %v, want %q", gotDelete.ProjectId, defaultProjectID)
		}
		if gotDelete.TeamId == nil || *gotDelete.TeamId != teamID.String() {
			t.Fatalf("DeleteTeam team id = %v, want %q", gotDelete.TeamId, teamID.String())
		}
	})

	t.Run("IgnoreNotFound", func(t *testing.T) {
		e := external{teams: &fake.TeamClient{
			DeleteTeamFn: func(_ context.Context, _ core.DeleteTeamArgs) error {
				return teamNotFoundErr()
			},
		}}

		if _, err := e.Delete(context.Background(), teamWith(teamID.String(), nil)); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
	})
}
