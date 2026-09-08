// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/operations"

	"k8s.io/apimachinery/pkg/util/wait"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/project/fake"
)

// Unlike many Kubernetes projects Crossplane does not use third party testing
// libraries, per the common Go test review comments. Crossplane encourages the
// use of table driven unit tests. The tests of the crossplane-runtime project
// are representative of the testing style Crossplane encourages.
//
// https://github.com/golang/go/wiki/TestComments
// https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md#contributing-code

func projectWith(name string, mutate func(cr *v1alpha1.Project)) *v1alpha1.Project {
	cr := &v1alpha1.Project{}
	if name != "" {
		meta.SetExternalName(cr, name)
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

const (
	testProjectName      = "my-project"
	testVisibilityPublic = "public"
)

func notFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

var errBoom = errors.New("boom")

func TestObserve(t *testing.T) {
	id := uuid.New()
	wellFormed := core.ProjectStateValues.WellFormed
	createPending := core.ProjectStateValues.CreatePending
	deleting := core.ProjectStateValues.Deleting
	visibility := core.ProjectVisibilityValues.Private

	type fields struct {
		project ProjectClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.Project
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
			fields: fields{project: &fake.ProjectClient{}},
			args:   args{cr: projectWith("", nil)},
			want:   want{o: managed.ExternalObservation{}},
		},
		"NotFound": {
			reason: "Observe should report no resource exists when the Azure DevOps API returns 404.",
			fields: fields{project: &fake.ProjectClient{
				GetProjectFn: func(_ context.Context, _ core.GetProjectArgs) (*core.TeamProject, error) {
					return nil, notFoundErr()
				},
			}},
			args: args{cr: projectWith(testProjectName, nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: false}},
		},
		"UpToDate": {
			reason: "Observe should report the resource is up to date when its mutable fields match the desired state.",
			fields: fields{project: &fake.ProjectClient{
				GetProjectFn: func(_ context.Context, _ core.GetProjectArgs) (*core.TeamProject, error) {
					return &core.TeamProject{
						Id:         &id,
						Name:       strPtr(testProjectName),
						State:      &wellFormed,
						Visibility: &visibility,
					}, nil
				},
			}},
			args: args{cr: projectWith(testProjectName, func(cr *v1alpha1.Project) {
				cr.Spec.ForProvider.Visibility = "private"
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}},
		},
		"NotUpToDate": {
			reason: "Observe should report the resource is not up to date when a mutable field differs from the desired state.",
			fields: fields{project: &fake.ProjectClient{
				GetProjectFn: func(_ context.Context, _ core.GetProjectArgs) (*core.TeamProject, error) {
					return &core.TeamProject{
						Id:         &id,
						Name:       strPtr(testProjectName),
						State:      &wellFormed,
						Visibility: &visibility,
					}, nil
				},
			}},
			args: args{cr: projectWith(testProjectName, func(cr *v1alpha1.Project) {
				cr.Spec.ForProvider.Visibility = testVisibilityPublic
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}},
		},
		"StillCreating": {
			reason: "Observe should report the resource exists, but skip the up-to-date check, while Azure DevOps is still provisioning it.",
			fields: fields{project: &fake.ProjectClient{
				GetProjectFn: func(_ context.Context, _ core.GetProjectArgs) (*core.TeamProject, error) {
					return &core.TeamProject{
						Id:    &id,
						Name:  strPtr(testProjectName),
						State: &createPending,
					}, nil
				},
			}},
			args: args{cr: projectWith(testProjectName, nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.TypeReady, reason: xpv2.ReasonCreating},
		},
		"StillDeleting": {
			reason: "Observe should report the Deleting condition, not Creating, while Azure DevOps is tearing the project down.",
			fields: fields{project: &fake.ProjectClient{
				GetProjectFn: func(_ context.Context, _ core.GetProjectArgs) (*core.TeamProject, error) {
					return &core.TeamProject{
						Id:    &id,
						Name:  strPtr(testProjectName),
						State: &deleting,
					}, nil
				},
			}},
			args: args{cr: projectWith(testProjectName, nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.TypeReady, reason: xpv2.ReasonDeleting},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{project: tc.fields.project}
			got, err := e.Observe(tc.args.ctx, tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if tc.want.condition != "" {
				got := tc.args.cr.Status.GetCondition(tc.want.condition)
				if got.Reason != tc.want.reason {
					t.Errorf("\n%s\ne.Observe(...): condition %s reason = %q, want %q\n", tc.reason, tc.want.condition, got.Reason, tc.want.reason)
				}
			}
		})
	}
}

func TestCreate(t *testing.T) {
	opID := uuid.New()
	succeeded := operations.OperationStatusValues.Succeeded

	t.Run("Success", func(t *testing.T) {
		cr := projectWith("", func(cr *v1alpha1.Project) {
			cr.Spec.ForProvider.Name = testProjectName
		})

		e := external{
			project: &fake.ProjectClient{
				QueueCreateProjectFn: func(_ context.Context, args core.QueueCreateProjectArgs) (*operations.OperationReference, error) {
					if args.ProjectToCreate == nil || args.ProjectToCreate.Name == nil || *args.ProjectToCreate.Name != testProjectName {
						t.Fatalf("QueueCreateProject called with unexpected project: %+v", args.ProjectToCreate)
					}
					return &operations.OperationReference{Id: &opID}, nil
				},
			},
			operations: &fake.OperationsClient{
				GetOperationFn: func(_ context.Context, _ operations.GetOperationArgs) (*operations.Operation, error) {
					return &operations.Operation{Status: &succeeded}, nil
				},
			},
		}

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("e.Create(...): unexpected error: %v", err)
		}

		if got := meta.GetExternalName(cr); got != testProjectName {
			t.Errorf("e.Create(...): external name = %q, want %q", got, testProjectName)
		}
	})

	t.Run("FullySpecified", func(t *testing.T) {
		templateID := uuid.New()
		templateName := "Agile"

		cr := projectWith("", func(cr *v1alpha1.Project) {
			cr.Spec.ForProvider.Name = testProjectName
			cr.Spec.ForProvider.Description = "a description"
			cr.Spec.ForProvider.Visibility = testVisibilityPublic
			cr.Spec.ForProvider.VersionControl = "Git"
			cr.Spec.ForProvider.WorkItemTemplate = "agile"
		})

		var gotArgs core.QueueCreateProjectArgs
		e := external{
			project: &fake.ProjectClient{
				GetProcessesFn: func(_ context.Context, _ core.GetProcessesArgs) (*[]core.Process, error) {
					return &[]core.Process{{Id: &templateID, Name: &templateName}}, nil
				},
				QueueCreateProjectFn: func(_ context.Context, args core.QueueCreateProjectArgs) (*operations.OperationReference, error) {
					gotArgs = args
					return &operations.OperationReference{Id: &opID}, nil
				},
			},
			operations: &fake.OperationsClient{
				GetOperationFn: func(_ context.Context, _ operations.GetOperationArgs) (*operations.Operation, error) {
					return &operations.Operation{Status: &succeeded}, nil
				},
			},
		}

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("e.Create(...): unexpected error: %v", err)
		}

		tp := gotArgs.ProjectToCreate
		if tp == nil || tp.Description == nil || *tp.Description != "a description" {
			t.Errorf("e.Create(...): unexpected description in %+v", tp)
		}
		if tp == nil || tp.Visibility == nil || string(*tp.Visibility) != testVisibilityPublic {
			t.Errorf("e.Create(...): unexpected visibility in %+v", tp)
		}
		if tp == nil || tp.Capabilities == nil {
			t.Fatalf("e.Create(...): expected capabilities to be set, got %+v", tp)
		}
		caps := *tp.Capabilities
		if caps["versioncontrol"]["sourceControlType"] != "Git" {
			t.Errorf("e.Create(...): unexpected versioncontrol capability: %+v", caps["versioncontrol"])
		}
		if caps["processTemplate"]["templateTypeId"] != templateID.String() {
			t.Errorf("e.Create(...): unexpected processTemplate capability: %+v", caps["processTemplate"])
		}
	})

	t.Run("ResolveProcessTemplateError", func(t *testing.T) {
		cr := projectWith("", func(cr *v1alpha1.Project) {
			cr.Spec.ForProvider.Name = testProjectName
			cr.Spec.ForProvider.WorkItemTemplate = "unknown-template"
		})

		e := external{project: &fake.ProjectClient{
			GetProcessesFn: func(_ context.Context, _ core.GetProcessesArgs) (*[]core.Process, error) {
				return &[]core.Process{}, nil
			},
			QueueCreateProjectFn: func(_ context.Context, _ core.QueueCreateProjectArgs) (*operations.OperationReference, error) {
				t.Fatal("QueueCreateProject should not be called when the process template cannot be resolved")
				return nil, nil
			},
		}}

		if _, err := e.Create(context.Background(), cr); err == nil {
			t.Fatal("e.Create(...): expected error when the process template cannot be resolved, got nil")
		}
	})

	t.Run("QueueCreateError", func(t *testing.T) {
		cr := projectWith("", func(cr *v1alpha1.Project) {
			cr.Spec.ForProvider.Name = testProjectName
		})

		e := external{project: &fake.ProjectClient{
			QueueCreateProjectFn: func(_ context.Context, _ core.QueueCreateProjectArgs) (*operations.OperationReference, error) {
				return nil, errBoom
			},
		}}

		if _, err := e.Create(context.Background(), cr); err == nil {
			t.Fatal("e.Create(...): expected error when QueueCreateProject fails, got nil")
		}
	})

	t.Run("WaitForOperationError", func(t *testing.T) {
		withOperationPollBackoff(t, wait.Backoff{Steps: 1})

		cr := projectWith("", func(cr *v1alpha1.Project) {
			cr.Spec.ForProvider.Name = testProjectName
		})

		e := external{
			project: &fake.ProjectClient{
				QueueCreateProjectFn: func(_ context.Context, _ core.QueueCreateProjectArgs) (*operations.OperationReference, error) {
					return &operations.OperationReference{Id: &opID}, nil
				},
			},
			operations: &fake.OperationsClient{
				GetOperationFn: func(_ context.Context, _ operations.GetOperationArgs) (*operations.Operation, error) {
					return nil, errBoom
				},
			},
		}

		if _, err := e.Create(context.Background(), cr); err == nil {
			t.Fatal("e.Create(...): expected error when waitForOperation fails, got nil")
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("NoObservedID", func(t *testing.T) {
		e := external{project: &fake.ProjectClient{
			QueueDeleteProjectFn: func(_ context.Context, _ core.QueueDeleteProjectArgs) (*operations.OperationReference, error) {
				t.Fatal("QueueDeleteProject should not be called when no project id has been observed")
				return nil, nil
			},
		}}

		cr := projectWith(testProjectName, nil)

		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}
	})

	t.Run("InvalidObservedID", func(t *testing.T) {
		e := external{project: &fake.ProjectClient{
			QueueDeleteProjectFn: func(_ context.Context, _ core.QueueDeleteProjectArgs) (*operations.OperationReference, error) {
				t.Fatal("QueueDeleteProject should not be called when the observed id is not a valid UUID")
				return nil, nil
			},
		}}

		cr := projectWith(testProjectName, func(cr *v1alpha1.Project) {
			cr.Status.AtProvider.ID = "not-a-uuid"
		})

		if _, err := e.Delete(context.Background(), cr); err == nil {
			t.Fatal("e.Delete(...): expected error for an invalid observed project id, got nil")
		}
	})

	t.Run("Success", func(t *testing.T) {
		id := uuid.New()
		opID := uuid.New()
		succeeded := operations.OperationStatusValues.Succeeded

		var gotArgs core.QueueDeleteProjectArgs
		e := external{
			project: &fake.ProjectClient{
				QueueDeleteProjectFn: func(_ context.Context, args core.QueueDeleteProjectArgs) (*operations.OperationReference, error) {
					gotArgs = args
					return &operations.OperationReference{Id: &opID}, nil
				},
			},
			operations: &fake.OperationsClient{
				GetOperationFn: func(_ context.Context, _ operations.GetOperationArgs) (*operations.Operation, error) {
					return &operations.Operation{Status: &succeeded}, nil
				},
			},
		}

		cr := projectWith(testProjectName, func(cr *v1alpha1.Project) {
			cr.Status.AtProvider.ID = id.String()
		})

		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("e.Delete(...): unexpected error: %v", err)
		}

		if gotArgs.ProjectId == nil || *gotArgs.ProjectId != id {
			t.Errorf("e.Delete(...): QueueDeleteProject called with project id = %v, want %v", gotArgs.ProjectId, id)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		id := uuid.New()
		e := external{project: &fake.ProjectClient{
			QueueDeleteProjectFn: func(_ context.Context, _ core.QueueDeleteProjectArgs) (*operations.OperationReference, error) {
				return nil, notFoundErr()
			},
		}}

		cr := projectWith(testProjectName, func(cr *v1alpha1.Project) {
			cr.Status.AtProvider.ID = id.String()
		})

		if _, err := e.Delete(context.Background(), cr); err != nil {
			t.Fatalf("e.Delete(...): unexpected error for a not-found project: %v", err)
		}
	})

	t.Run("QueueDeleteError", func(t *testing.T) {
		id := uuid.New()
		e := external{project: &fake.ProjectClient{
			QueueDeleteProjectFn: func(_ context.Context, _ core.QueueDeleteProjectArgs) (*operations.OperationReference, error) {
				return nil, errBoom
			},
		}}

		cr := projectWith(testProjectName, func(cr *v1alpha1.Project) {
			cr.Status.AtProvider.ID = id.String()
		})

		if _, err := e.Delete(context.Background(), cr); err == nil {
			t.Fatal("e.Delete(...): expected error when QueueDeleteProject fails, got nil")
		}
	})

	t.Run("WaitForOperationError", func(t *testing.T) {
		withOperationPollBackoff(t, wait.Backoff{Steps: 1})

		id := uuid.New()
		opID := uuid.New()
		e := external{
			project: &fake.ProjectClient{
				QueueDeleteProjectFn: func(_ context.Context, _ core.QueueDeleteProjectArgs) (*operations.OperationReference, error) {
					return &operations.OperationReference{Id: &opID}, nil
				},
			},
			operations: &fake.OperationsClient{
				GetOperationFn: func(_ context.Context, _ operations.GetOperationArgs) (*operations.Operation, error) {
					return nil, errBoom
				},
			},
		}

		cr := projectWith(testProjectName, func(cr *v1alpha1.Project) {
			cr.Status.AtProvider.ID = id.String()
		})

		if _, err := e.Delete(context.Background(), cr); err == nil {
			t.Fatal("e.Delete(...): expected error when waitForOperation fails, got nil")
		}
	})
}

func TestResolveProcessTemplateID(t *testing.T) {
	t.Run("Found", func(t *testing.T) {
		id := uuid.New()
		name := "Agile"
		e := external{project: &fake.ProjectClient{
			GetProcessesFn: func(_ context.Context, _ core.GetProcessesArgs) (*[]core.Process, error) {
				return &[]core.Process{{Id: &id, Name: &name}}, nil
			},
		}}

		got, err := e.resolveProcessTemplateID(context.Background(), "agile")
		if err != nil {
			t.Fatalf("e.resolveProcessTemplateID(...): unexpected error: %v", err)
		}
		if got != id.String() {
			t.Errorf("e.resolveProcessTemplateID(...): got %q, want %q", got, id.String())
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		id := uuid.New()
		name := "Agile"
		e := external{project: &fake.ProjectClient{
			GetProcessesFn: func(_ context.Context, _ core.GetProcessesArgs) (*[]core.Process, error) {
				return &[]core.Process{{Id: &id, Name: &name}}, nil
			},
		}}

		if _, err := e.resolveProcessTemplateID(context.Background(), "Scrum"); err == nil {
			t.Fatal("e.resolveProcessTemplateID(...): expected error for an unknown template name, got nil")
		}
	})

	t.Run("Error", func(t *testing.T) {
		e := external{project: &fake.ProjectClient{
			GetProcessesFn: func(_ context.Context, _ core.GetProcessesArgs) (*[]core.Process, error) {
				return nil, errBoom
			},
		}}

		if _, err := e.resolveProcessTemplateID(context.Background(), "Agile"); err == nil {
			t.Fatal("e.resolveProcessTemplateID(...): expected error when GetProcesses fails, got nil")
		}
	})
}

func TestUpdate(t *testing.T) {
	id := uuid.New()
	opID := uuid.New()
	succeeded := operations.OperationStatusValues.Succeeded

	t.Run("NoObservedID", func(t *testing.T) {
		e := external{project: &fake.ProjectClient{
			UpdateProjectFn: func(_ context.Context, _ core.UpdateProjectArgs) (*operations.OperationReference, error) {
				t.Fatal("UpdateProject should not be called when no project id has been observed")
				return nil, nil
			},
		}}

		cr := projectWith(testProjectName, nil)

		if _, err := e.Update(context.Background(), cr); err == nil {
			t.Fatal("e.Update(...): expected error when no project id has been observed, got nil")
		}
	})

	t.Run("Success", func(t *testing.T) {
		var gotArgs core.UpdateProjectArgs
		e := external{
			project: &fake.ProjectClient{
				UpdateProjectFn: func(_ context.Context, args core.UpdateProjectArgs) (*operations.OperationReference, error) {
					gotArgs = args
					return &operations.OperationReference{Id: &opID}, nil
				},
			},
			operations: &fake.OperationsClient{
				GetOperationFn: func(_ context.Context, _ operations.GetOperationArgs) (*operations.Operation, error) {
					return &operations.Operation{Status: &succeeded}, nil
				},
			},
		}

		cr := projectWith(testProjectName, func(cr *v1alpha1.Project) {
			cr.Spec.ForProvider.Visibility = testVisibilityPublic
			cr.Status.AtProvider.ID = id.String()
		})

		if _, err := e.Update(context.Background(), cr); err != nil {
			t.Fatalf("e.Update(...): unexpected error: %v", err)
		}

		if gotArgs.ProjectId == nil || *gotArgs.ProjectId != id {
			t.Errorf("e.Update(...): UpdateProject called with project id = %v, want %v", gotArgs.ProjectId, id)
		}
		if gotArgs.ProjectUpdate == nil || gotArgs.ProjectUpdate.Visibility == nil || string(*gotArgs.ProjectUpdate.Visibility) != testVisibilityPublic {
			t.Errorf("e.Update(...): UpdateProject called with unexpected visibility: %+v", gotArgs.ProjectUpdate)
		}
	})
}

// withOperationPollBackoff temporarily overrides operationPollBackoff for
// the duration of a test, restoring it afterward. Tests that exercise
// waitForOperation's failure/timeout paths use a much smaller backoff so
// they don't have to wait out the real (multi-second) production schedule.
func withOperationPollBackoff(t *testing.T, b wait.Backoff) {
	t.Helper()
	orig := operationPollBackoff
	operationPollBackoff = b
	t.Cleanup(func() { operationPollBackoff = orig })
}

func TestWaitForOperation(t *testing.T) {
	opID := uuid.New()
	failed := operations.OperationStatusValues.Failed
	inProgress := operations.OperationStatusValues.InProgress

	t.Run("NilOperation", func(t *testing.T) {
		e := external{}
		if err := e.waitForOperation(context.Background(), nil); err != nil {
			t.Fatalf("e.waitForOperation(nil): unexpected error: %v", err)
		}
	})

	t.Run("Failure", func(t *testing.T) {
		withOperationPollBackoff(t, wait.Backoff{Duration: time.Millisecond, Factor: 1, Steps: 3})

		msg := "something went wrong"
		e := external{operations: &fake.OperationsClient{
			GetOperationFn: func(_ context.Context, _ operations.GetOperationArgs) (*operations.Operation, error) {
				return &operations.Operation{Status: &failed, DetailedMessage: &msg}, nil
			},
		}}

		err := e.waitForOperation(context.Background(), &operations.OperationReference{Id: &opID})
		if err == nil {
			t.Fatal("e.waitForOperation(...): expected error for a failed operation, got nil")
		}
		if !strings.Contains(err.Error(), msg) {
			t.Errorf("e.waitForOperation(...): error = %q, want it to contain %q", err.Error(), msg)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		withOperationPollBackoff(t, wait.Backoff{Duration: time.Millisecond, Factor: 1, Steps: 3})

		e := external{operations: &fake.OperationsClient{
			GetOperationFn: func(_ context.Context, _ operations.GetOperationArgs) (*operations.Operation, error) {
				return &operations.Operation{Status: &inProgress}, nil
			},
		}}

		err := e.waitForOperation(context.Background(), &operations.OperationReference{Id: &opID})
		if err == nil {
			t.Fatal("e.waitForOperation(...): expected error when the operation never reaches a terminal state, got nil")
		}
	})
}

func strPtr(s string) *string { return &s }
