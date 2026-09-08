// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package agentqueue

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	runtimeTest "github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/agentqueue/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/agentqueue/fake"
)

const (
	defaultProjectID = "11111111-1111-1111-1111-111111111111"
	defaultPoolID    = "37"
)

func agentQueueWith(externalName string, mutate func(cr *v1alpha1.AgentQueue)) *v1alpha1.AgentQueue {
	cr := &v1alpha1.AgentQueue{}
	cr.Spec.ForProvider.ProjectID = defaultProjectID
	cr.Spec.ForProvider.AgentPoolID = defaultPoolID
	if externalName != "" {
		meta.SetExternalName(cr, externalName)
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

func agentQueueNotFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func taskAgentQueue(id int, mutate func(queue *taskagent.TaskAgentQueue)) *taskagent.TaskAgentQueue {
	queue := &taskagent.TaskAgentQueue{
		Id:   intPtr(id),
		Name: stringPtr("example-pool"),
		Pool: &taskagent.TaskAgentPoolReference{Id: intPtr(37)},
	}
	if mutate != nil {
		mutate(queue)
	}
	return queue
}

func TestObserve(t *testing.T) {
	type fields struct {
		queues AgentQueueClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.AgentQueue
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
			reason: "Observe should report no external resource when the queue has never been created.",
			fields: fields{queues: &fake.AgentQueueClient{}},
			args:   args{cr: agentQueueWith("", nil)},
			want:   want{o: managed.ExternalObservation{}},
		},
		"NotFound": {
			reason: "Observe should report no external resource when Azure DevOps returns 404.",
			fields: fields{queues: &fake.AgentQueueClient{GetAgentQueueFn: func(_ context.Context, _ taskagent.GetAgentQueueArgs) (*taskagent.TaskAgentQueue, error) {
				return nil, agentQueueNotFoundErr()
			}}},
			args: args{cr: agentQueueWith("55", nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: false}},
		},
		"UpToDate": {
			reason: "Observe should report the queue is up to date when its pool linkage matches desired state.",
			fields: fields{queues: &fake.AgentQueueClient{GetAgentQueueFn: func(_ context.Context, _ taskagent.GetAgentQueueArgs) (*taskagent.TaskAgentQueue, error) {
				return taskAgentQueue(55, nil), nil
			}}},
			args: args{cr: agentQueueWith("55", nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
		"NeedsUpdate": {
			reason: "Observe should report the queue is not up to date when the pool linkage differs.",
			fields: fields{queues: &fake.AgentQueueClient{GetAgentQueueFn: func(_ context.Context, _ taskagent.GetAgentQueueArgs) (*taskagent.TaskAgentQueue, error) {
				return taskAgentQueue(55, func(q *taskagent.TaskAgentQueue) {
					q.Pool = &taskagent.TaskAgentPoolReference{Id: intPtr(99)}
				}), nil
			}}},
			args: args{cr: agentQueueWith("55", nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
		"MissingProjectID": {
			reason: "Observe should fail when spec.forProvider.projectId is empty.",
			fields: fields{queues: &fake.AgentQueueClient{}},
			args: args{cr: agentQueueWith("55", func(cr *v1alpha1.AgentQueue) {
				cr.Spec.ForProvider.ProjectID = ""
			})},
			want: want{err: stderrors.New(errMissingProjectID)},
		},
		"GetError": {
			reason: "Observe should wrap non-404 API errors.",
			fields: fields{queues: &fake.AgentQueueClient{GetAgentQueueFn: func(_ context.Context, _ taskagent.GetAgentQueueArgs) (*taskagent.TaskAgentQueue, error) {
				return nil, stderrors.New("boom")
			}}},
			args: args{cr: agentQueueWith("55", nil)},
			want: want{err: stderrors.New("boom")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.ctx == nil {
				tc.args.ctx = context.Background()
			}
			e := external{queues: tc.fields.queues}
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
	type fields struct {
		queues AgentQueueClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.AgentQueue
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
		check  func(t *testing.T, cr *v1alpha1.AgentQueue)
	}{
		"Success": {
			reason: "Create should send the desired payload and record the created queue id as the external name.",
			fields: fields{queues: &fake.AgentQueueClient{AddAgentQueueFn: func(_ context.Context, args taskagent.AddAgentQueueArgs) (*taskagent.TaskAgentQueue, error) {
				if args.Project == nil || *args.Project != defaultProjectID {
					t.Fatalf("AddAgentQueue project = %+v, want %q", args.Project, defaultProjectID)
				}
				if args.Queue == nil || args.Queue.Pool == nil || args.Queue.Pool.Id == nil || *args.Queue.Pool.Id != 37 {
					t.Fatalf("AddAgentQueue pool = %+v, want id 37", args.Queue)
				}
				return taskAgentQueue(55, nil), nil
			}}},
			args: args{cr: agentQueueWith("", nil)},
			check: func(t *testing.T, cr *v1alpha1.AgentQueue) {
				if got := meta.GetExternalName(cr); got != "55" {
					t.Fatalf("external name = %q, want %q", got, "55")
				}
				if got := cr.Status.AtProvider.PoolID; got != "37" {
					t.Fatalf("status poolId = %q, want %q", got, "37")
				}
			},
		},
		"MissingProjectID": {
			reason: "Create should fail when spec.forProvider.projectId is empty.",
			fields: fields{queues: &fake.AgentQueueClient{}},
			args: args{cr: agentQueueWith("", func(cr *v1alpha1.AgentQueue) {
				cr.Spec.ForProvider.ProjectID = ""
			})},
			want: want{err: stderrors.New(errMissingProjectID)},
		},
		"MissingPoolID": {
			reason: "Create should fail when spec.forProvider.agentPoolId is empty.",
			fields: fields{queues: &fake.AgentQueueClient{}},
			args: args{cr: agentQueueWith("", func(cr *v1alpha1.AgentQueue) {
				cr.Spec.ForProvider.AgentPoolID = ""
			})},
			want: want{err: stderrors.New(errMissingPoolID)},
		},
		"CreateError": {
			reason: "Create should wrap API errors.",
			fields: fields{queues: &fake.AgentQueueClient{AddAgentQueueFn: func(_ context.Context, _ taskagent.AddAgentQueueArgs) (*taskagent.TaskAgentQueue, error) {
				return nil, stderrors.New("boom")
			}}},
			args: args{cr: agentQueueWith("", nil)},
			want: want{err: stderrors.New("boom")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.ctx == nil {
				tc.args.ctx = context.Background()
			}
			e := external{queues: tc.fields.queues}
			_, err := e.Create(tc.args.ctx, tc.args.cr)
			gotErr := err
			if tc.want.err != nil {
				if unwrapped := stderrors.Unwrap(err); unwrapped != nil {
					gotErr = unwrapped
				}
			}
			if diff := cmp.Diff(tc.want.err, gotErr, runtimeTest.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if tc.check != nil {
				tc.check(t, tc.args.cr)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	e := external{queues: &fake.AgentQueueClient{}}

	if _, err := e.Update(context.Background(), agentQueueWith("55", nil)); err != nil {
		t.Fatalf("e.Update(...) = %v, want nil", err)
	}

	_, err := e.Update(context.Background(), agentQueueWith("", nil))
	if diff := cmp.Diff(stderrors.New(errMissingPoolID), err, runtimeTest.EquateErrors()); diff != "" {
		t.Errorf("e.Update(...): -want error, +got error:\n%s\n", diff)
	}
}

func TestDelete(t *testing.T) {
	type fields struct {
		queues AgentQueueClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.AgentQueue
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"NoObservedID": {
			reason: "Delete should not call Azure DevOps when there is no queue id.",
			fields: fields{queues: &fake.AgentQueueClient{DeleteAgentQueueFn: func(_ context.Context, _ taskagent.DeleteAgentQueueArgs) error {
				t.Fatal("DeleteAgentQueue should not be called when there is no queue id")
				return nil
			}}},
			args: args{cr: agentQueueWith("", nil)},
		},
		"Success": {
			reason: "Delete should pass the observed queue id and project to Azure DevOps.",
			fields: fields{queues: &fake.AgentQueueClient{DeleteAgentQueueFn: func(_ context.Context, args taskagent.DeleteAgentQueueArgs) error {
				if args.QueueId == nil || *args.QueueId != 55 {
					t.Fatalf("DeleteAgentQueue id = %v, want 55", args.QueueId)
				}
				if args.Project == nil || *args.Project != defaultProjectID {
					t.Fatalf("DeleteAgentQueue project = %v, want %q", args.Project, defaultProjectID)
				}
				return nil
			}}},
			args: args{cr: agentQueueWith("55", nil)},
		},
		"AlreadyGone": {
			reason: "Delete should ignore not-found responses.",
			fields: fields{queues: &fake.AgentQueueClient{DeleteAgentQueueFn: func(_ context.Context, _ taskagent.DeleteAgentQueueArgs) error {
				return agentQueueNotFoundErr()
			}}},
			args: args{cr: agentQueueWith("55", nil)},
		},
		"DeleteError": {
			reason: "Delete should wrap non-404 errors.",
			fields: fields{queues: &fake.AgentQueueClient{DeleteAgentQueueFn: func(_ context.Context, _ taskagent.DeleteAgentQueueArgs) error {
				return stderrors.New("boom")
			}}},
			args: args{cr: agentQueueWith("55", nil)},
			want: want{err: stderrors.New("boom")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.ctx == nil {
				tc.args.ctx = context.Background()
			}
			e := external{queues: tc.fields.queues}
			_, err := e.Delete(tc.args.ctx, tc.args.cr)
			gotErr := err
			if tc.want.err != nil {
				if unwrapped := stderrors.Unwrap(err); unwrapped != nil {
					gotErr = unwrapped
				}
			}
			if diff := cmp.Diff(tc.want.err, gotErr, runtimeTest.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestAgentQueueIsUpToDate(t *testing.T) {
	cases := map[string]struct {
		desired v1alpha1.AgentQueueParameters
		current *taskagent.TaskAgentQueue
		want    bool
	}{
		"NilCurrent": {
			desired: v1alpha1.AgentQueueParameters{AgentPoolID: "37"},
			current: nil,
			want:    false,
		},
		"NilPool": {
			desired: v1alpha1.AgentQueueParameters{AgentPoolID: "37"},
			current: &taskagent.TaskAgentQueue{Id: intPtr(55)},
			want:    false,
		},
		"Match": {
			desired: v1alpha1.AgentQueueParameters{AgentPoolID: "37"},
			current: taskAgentQueue(55, nil),
			want:    true,
		},
		"Mismatch": {
			desired: v1alpha1.AgentQueueParameters{AgentPoolID: "99"},
			current: taskAgentQueue(55, nil),
			want:    false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isUpToDate(tc.desired, tc.current); got != tc.want {
				t.Fatalf("isUpToDate(...) = %v, want %v", got, tc.want)
			}
		})
	}
}
