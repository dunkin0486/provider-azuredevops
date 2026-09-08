// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/webapi"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	runtimeTest "github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/environment/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/environment/fake"
)

const (
	defaultProjectID       = "11111111-1111-1111-1111-111111111111"
	defaultEnvironmentName = "example-environment"
	environmentDescription = "Example environment"
)

func environmentWith(externalName string, mutate func(cr *v1alpha1.Environment)) *v1alpha1.Environment {
	cr := &v1alpha1.Environment{}
	cr.Spec.ForProvider.ProjectID = defaultProjectID
	cr.Spec.ForProvider.Name = defaultEnvironmentName
	if externalName != "" {
		meta.SetExternalName(cr, externalName)
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

func environmentNotFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func environmentInstance(id int, mutate func(env *taskagent.EnvironmentInstance)) *taskagent.EnvironmentInstance {
	projectID := uuid.MustParse(defaultProjectID)
	createdAt := azuredevops.Time{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	env := &taskagent.EnvironmentInstance{
		Id:          intPtr(id),
		Name:        stringPtr(defaultEnvironmentName),
		Description: stringPtr(environmentDescription),
		CreatedBy:   &webapi.IdentityRef{DisplayName: stringPtr("Example User"), UniqueName: stringPtr("example@example.com"), Id: stringPtr("user-id")},
		CreatedOn:   &createdAt,
		Project:     &taskagent.ProjectReference{Id: &projectID, Name: stringPtr("Example Project")},
	}
	if mutate != nil {
		mutate(env)
	}
	return env
}

func TestObserve(t *testing.T) {
	environmentID := 37

	type fields struct {
		environments EnvironmentClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.Environment
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
			reason: "Observe should report no external resource when the environment has never been created.",
			fields: fields{environments: &fake.EnvironmentClient{}},
			args:   args{cr: environmentWith("", nil)},
			want:   want{o: managed.ExternalObservation{}},
		},
		"NotFound": {
			reason: "Observe should report no external resource when Azure DevOps returns 404.",
			fields: fields{environments: &fake.EnvironmentClient{GetEnvironmentByIdFn: func(_ context.Context, _ taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
				return nil, environmentNotFoundErr()
			}}},
			args: args{cr: environmentWith("37", func(cr *v1alpha1.Environment) {
				setProjectIDAnnotation(cr, defaultProjectID)
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: false}},
		},
		"ImportedProjectNotVerified": {
			reason: "Observe should refuse to treat an imported environment as missing until its projectId has been verified.",
			fields: fields{environments: &fake.EnvironmentClient{GetEnvironmentByIdFn: func(_ context.Context, _ taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
				return nil, environmentNotFoundErr()
			}}},
			args: args{cr: environmentWith("37", nil)},
			want: want{err: stderrors.New(errImportProjectIDVerification)},
		},
		"NotFoundAfterProjectRetarget": {
			reason: "Observe should surface immutable project drift instead of reporting the environment missing when the resolved project changes after creation.",
			fields: fields{environments: &fake.EnvironmentClient{GetEnvironmentByIdFn: func(_ context.Context, args taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
				if args.Project == nil || *args.Project != defaultProjectID {
					t.Fatalf("GetEnvironmentById project = %v, want %q", args.Project, defaultProjectID)
				}
				return nil, environmentNotFoundErr()
			}}},
			args: args{cr: environmentWith("37", func(cr *v1alpha1.Environment) {
				cr.Spec.ForProvider.ProjectID = "33333333-3333-3333-3333-333333333333"
				setProjectIDAnnotation(cr, defaultProjectID)
			})},
			want: want{err: stderrors.New(errImmutableProject + ": desired \"33333333-3333-3333-3333-333333333333\", observed \"" + defaultProjectID + "\"")},
		},
		"UpToDate": {
			reason: "Observe should report the environment is up to date when mutable fields match desired state.",
			fields: fields{environments: &fake.EnvironmentClient{GetEnvironmentByIdFn: func(_ context.Context, _ taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
				return environmentInstance(environmentID, nil), nil
			}}},
			args: args{cr: environmentWith("37", func(cr *v1alpha1.Environment) {
				cr.Spec.ForProvider.Description = environmentDescription
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
		"NeedsUpdate": {
			reason: "Observe should report the environment is not up to date when the desired description differs.",
			fields: fields{environments: &fake.EnvironmentClient{GetEnvironmentByIdFn: func(_ context.Context, _ taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
				return environmentInstance(environmentID, nil), nil
			}}},
			args: args{cr: environmentWith("37", func(cr *v1alpha1.Environment) {
				cr.Spec.ForProvider.Description = "Updated description"
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
		"ImmutableProjectChange": {
			reason: "Observe should keep reading from the original project when projectId drifts, so the immutable-field change is surfaced instead of treating the environment as missing.",
			fields: fields{environments: &fake.EnvironmentClient{GetEnvironmentByIdFn: func(_ context.Context, args taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
				if args.Project == nil || *args.Project != defaultProjectID {
					t.Fatalf("GetEnvironmentById project = %v, want %q", args.Project, defaultProjectID)
				}
				return environmentInstance(environmentID, nil), nil
			}}},
			args: args{cr: environmentWith("37", func(cr *v1alpha1.Environment) {
				cr.Spec.ForProvider.ProjectID = "33333333-3333-3333-3333-333333333333"
				setProjectIDAnnotation(cr, defaultProjectID)
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
		"GetError": {
			reason: "Observe should wrap non-404 API errors.",
			fields: fields{environments: &fake.EnvironmentClient{GetEnvironmentByIdFn: func(_ context.Context, _ taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
				return nil, stderrors.New("boom")
			}}},
			args: args{cr: environmentWith("37", nil)},
			want: want{err: stderrors.New("boom")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.ctx == nil {
				tc.args.ctx = context.Background()
			}
			e := external{environments: tc.fields.environments}
			got, err := e.Observe(tc.args.ctx, tc.args.cr)
			gotErr := err
			if tc.want.err != nil {
				if unwrapped := stderrors.Unwrap(err); unwrapped != nil {
					gotErr = unwrapped
				}
			}
			if diff := cmp.Diff(tc.want.err, gotErr, runtimeTest.EquateErrors()); diff != "" {
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
	t.Run("Success", func(t *testing.T) {
		var createArgs taskagent.AddEnvironmentArgs
		e := external{environments: &fake.EnvironmentClient{AddEnvironmentFn: func(_ context.Context, args taskagent.AddEnvironmentArgs) (*taskagent.EnvironmentInstance, error) {
			createArgs = args
			return environmentInstance(37, nil), nil
		}}}

		cr := environmentWith("", func(cr *v1alpha1.Environment) {
			cr.Spec.ForProvider.Description = environmentDescription
		})

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("e.Create(...): unexpected error: %v", err)
		}
		if got := meta.GetExternalName(cr); got != "37" {
			t.Fatalf("external name = %q, want %q", got, "37")
		}
		if createArgs.Project == nil || *createArgs.Project != defaultProjectID {
			t.Fatalf("AddEnvironment project = %v, want %q", createArgs.Project, defaultProjectID)
		}
		if createArgs.EnvironmentCreateParameter == nil || createArgs.EnvironmentCreateParameter.Name == nil || *createArgs.EnvironmentCreateParameter.Name != defaultEnvironmentName {
			t.Fatalf("AddEnvironment called with unexpected payload: %+v", createArgs.EnvironmentCreateParameter)
		}
		if createArgs.EnvironmentCreateParameter.Description == nil || *createArgs.EnvironmentCreateParameter.Description != environmentDescription {
			t.Fatalf("AddEnvironment called with unexpected description payload: %+v", createArgs.EnvironmentCreateParameter)
		}
		if got := cr.GetAnnotations()[annotationProjectID]; got != defaultProjectID {
			t.Fatalf("project annotation = %q, want %q", got, defaultProjectID)
		}
	})

	t.Run("CreateError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{environments: &fake.EnvironmentClient{AddEnvironmentFn: func(_ context.Context, _ taskagent.AddEnvironmentArgs) (*taskagent.EnvironmentInstance, error) {
			return nil, wantErr
		}}}

		_, err := e.Create(context.Background(), environmentWith("", nil))
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Create(...): -want wrapped error, +got:\n%s", diff)
		}
	})
}

func TestUpdate(t *testing.T) {
	t.Run("RenameAndDescriptionUpdateSuccess", func(t *testing.T) {
		var gotUpdate taskagent.UpdateEnvironmentArgs
		e := external{environments: &fake.EnvironmentClient{
			GetEnvironmentByIdFn: func(_ context.Context, _ taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
				return environmentInstance(37, nil), nil
			},
			UpdateEnvironmentFn: func(_ context.Context, args taskagent.UpdateEnvironmentArgs) (*taskagent.EnvironmentInstance, error) {
				gotUpdate = args
				return environmentInstance(37, func(env *taskagent.EnvironmentInstance) {
					env.Name = stringPtr("renamed-environment")
					env.Description = stringPtr("Updated description")
				}), nil
			},
		}}

		cr := environmentWith("37", func(cr *v1alpha1.Environment) {
			cr.Spec.ForProvider.Name = "renamed-environment"
			cr.Spec.ForProvider.Description = "Updated description"
			setProjectIDAnnotation(cr, defaultProjectID)
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("e.Update(...): unexpected error: %v", err)
		}
		if gotUpdate.EnvironmentId == nil || *gotUpdate.EnvironmentId != 37 {
			t.Fatalf("UpdateEnvironment id = %v, want 37", gotUpdate.EnvironmentId)
		}
		if gotUpdate.EnvironmentUpdateParameter == nil || gotUpdate.EnvironmentUpdateParameter.Name == nil || *gotUpdate.EnvironmentUpdateParameter.Name != "renamed-environment" {
			t.Fatalf("UpdateEnvironment patch = %+v, want renamed-environment", gotUpdate.EnvironmentUpdateParameter)
		}
		if gotUpdate.EnvironmentUpdateParameter.Description == nil || *gotUpdate.EnvironmentUpdateParameter.Description != "Updated description" {
			t.Fatalf("UpdateEnvironment patch = %+v, want updated description", gotUpdate.EnvironmentUpdateParameter)
		}
	})

	t.Run("NoChanges", func(t *testing.T) {
		e := external{environments: &fake.EnvironmentClient{
			GetEnvironmentByIdFn: func(_ context.Context, _ taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
				return environmentInstance(37, nil), nil
			},
			UpdateEnvironmentFn: func(_ context.Context, _ taskagent.UpdateEnvironmentArgs) (*taskagent.EnvironmentInstance, error) {
				t.Fatal("UpdateEnvironment should not be called when nothing changed")
				return nil, nil
			},
		}}

		cr := environmentWith("37", func(cr *v1alpha1.Environment) {
			cr.Spec.ForProvider.Description = environmentDescription
			setProjectIDAnnotation(cr, defaultProjectID)
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("e.Update(...): unexpected error: %v", err)
		}
	})

	t.Run("ImmutableProjectChange", func(t *testing.T) {
		e := external{environments: &fake.EnvironmentClient{
			GetEnvironmentByIdFn: func(_ context.Context, args taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
				if args.Project == nil || *args.Project != defaultProjectID {
					t.Fatalf("GetEnvironmentById project = %v, want %q", args.Project, defaultProjectID)
				}
				return environmentInstance(37, nil), nil
			},
			UpdateEnvironmentFn: func(_ context.Context, _ taskagent.UpdateEnvironmentArgs) (*taskagent.EnvironmentInstance, error) {
				t.Fatal("UpdateEnvironment should not be called when immutable fields change")
				return nil, nil
			},
		}}

		cr := environmentWith("37", func(cr *v1alpha1.Environment) {
			cr.Spec.ForProvider.ProjectID = "33333333-3333-3333-3333-333333333333"
			setProjectIDAnnotation(cr, defaultProjectID)
		})

		if _, err := e.Update(context.Background(), cr); err == nil {
			t.Fatal("e.Update(...): expected error for immutable project change, got nil")
		}
	})

	t.Run("MissingEnvironmentID", func(t *testing.T) {
		e := external{environments: &fake.EnvironmentClient{
			UpdateEnvironmentFn: func(_ context.Context, _ taskagent.UpdateEnvironmentArgs) (*taskagent.EnvironmentInstance, error) {
				t.Fatal("UpdateEnvironment should not be called when there is no environment id")
				return nil, nil
			},
		}}

		if _, err := e.Update(context.Background(), environmentWith("", nil)); err == nil {
			t.Fatal("e.Update(...): expected error when there is no environment id, got nil")
		}
	})

	t.Run("ImportedProjectNotVerified", func(t *testing.T) {
		e := external{environments: &fake.EnvironmentClient{
			GetEnvironmentByIdFn: func(_ context.Context, _ taskagent.GetEnvironmentByIdArgs) (*taskagent.EnvironmentInstance, error) {
				t.Fatal("GetEnvironmentById should not be called before import project verification")
				return nil, nil
			},
		}}

		if _, err := e.Update(context.Background(), environmentWith("37", nil)); err == nil {
			t.Fatal("e.Update(...): expected import verification error, got nil")
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("NoObservedID", func(t *testing.T) {
		e := external{environments: &fake.EnvironmentClient{DeleteEnvironmentFn: func(_ context.Context, _ taskagent.DeleteEnvironmentArgs) error {
			t.Fatal("DeleteEnvironment should not be called when there is no environment id")
			return nil
		}}}

		if _, err := e.Delete(context.Background(), environmentWith("", nil)); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		var gotDelete taskagent.DeleteEnvironmentArgs
		e := external{environments: &fake.EnvironmentClient{DeleteEnvironmentFn: func(_ context.Context, args taskagent.DeleteEnvironmentArgs) error {
			gotDelete = args
			return nil
		}}}

		if _, err := e.Delete(context.Background(), environmentWith("37", func(cr *v1alpha1.Environment) {
			setProjectIDAnnotation(cr, defaultProjectID)
		})); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
		if gotDelete.Project == nil || *gotDelete.Project != defaultProjectID {
			t.Fatalf("DeleteEnvironment project = %v, want %q", gotDelete.Project, defaultProjectID)
		}
		if gotDelete.EnvironmentId == nil || *gotDelete.EnvironmentId != 37 {
			t.Fatalf("DeleteEnvironment id = %v, want 37", gotDelete.EnvironmentId)
		}
	})

	t.Run("ImmutableProjectChange", func(t *testing.T) {
		e := external{environments: &fake.EnvironmentClient{DeleteEnvironmentFn: func(_ context.Context, args taskagent.DeleteEnvironmentArgs) error {
			t.Fatalf("DeleteEnvironment should not be called on immutable project drift: %+v", args)
			return nil
		}}}

		cr := environmentWith("37", func(cr *v1alpha1.Environment) {
			cr.Spec.ForProvider.ProjectID = "33333333-3333-3333-3333-333333333333"
			setProjectIDAnnotation(cr, defaultProjectID)
		})

		if _, err := e.Delete(context.Background(), cr); err == nil {
			t.Fatal("e.Delete(...): expected immutable project error, got nil")
		}
	})

	t.Run("AlreadyGone", func(t *testing.T) {
		e := external{environments: &fake.EnvironmentClient{DeleteEnvironmentFn: func(_ context.Context, _ taskagent.DeleteEnvironmentArgs) error {
			return environmentNotFoundErr()
		}}}

		if _, err := e.Delete(context.Background(), environmentWith("37", func(cr *v1alpha1.Environment) {
			setProjectIDAnnotation(cr, defaultProjectID)
		})); err != nil {
			t.Fatalf("e.Delete(...): expected nil error when environment is already gone, got %v", err)
		}
	})

	t.Run("DeleteError", func(t *testing.T) {
		wantErr := stderrors.New("boom")
		e := external{environments: &fake.EnvironmentClient{DeleteEnvironmentFn: func(_ context.Context, _ taskagent.DeleteEnvironmentArgs) error {
			return wantErr
		}}}

		_, err := e.Delete(context.Background(), environmentWith("37", func(cr *v1alpha1.Environment) {
			setProjectIDAnnotation(cr, defaultProjectID)
		}))
		if diff := cmp.Diff(wantErr, stderrors.Unwrap(err), runtimeTest.EquateErrors()); diff != "" {
			t.Fatalf("e.Delete(...): -want wrapped error, +got:\n%s", diff)
		}
	})

	t.Run("ImportedProjectNotVerified", func(t *testing.T) {
		e := external{environments: &fake.EnvironmentClient{DeleteEnvironmentFn: func(_ context.Context, _ taskagent.DeleteEnvironmentArgs) error {
			t.Fatal("DeleteEnvironment should not be called before import project verification")
			return nil
		}}}

		if _, err := e.Delete(context.Background(), environmentWith("37", nil)); err == nil {
			t.Fatal("e.Delete(...): expected import verification error, got nil")
		}
	})
}
