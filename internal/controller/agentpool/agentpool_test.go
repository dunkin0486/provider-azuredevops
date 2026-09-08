// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package agentpool

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

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/agentpool/v1alpha1"
	"github.com/dunkin0486/provider-azuredevops/internal/controller/agentpool/fake"
)

const defaultAgentPoolName = "example-agent-pool"

func agentPoolWith(externalName string, mutate func(cr *v1alpha1.AgentPool)) *v1alpha1.AgentPool {
	cr := &v1alpha1.AgentPool{}
	cr.Spec.ForProvider.Name = defaultAgentPoolName
	cr.Spec.ForProvider.IsHosted = boolPtr(false)
	if externalName != "" {
		meta.SetExternalName(cr, externalName)
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

func agentPoolNotFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func taskAgentPool(id int, mutate func(pool *taskagent.TaskAgentPool)) *taskagent.TaskAgentPool {
	pool := &taskagent.TaskAgentPool{
		Id:            intPtr(id),
		Name:          stringPtr(defaultAgentPoolName),
		IsHosted:      boolPtr(false),
		AutoProvision: boolPtr(true),
		AutoUpdate:    boolPtr(false),
		Size:          intPtr(2),
	}
	if mutate != nil {
		mutate(pool)
	}
	return pool
}

func TestObserve(t *testing.T) {
	type fields struct {
		agentPools AgentPoolClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.AgentPool
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
			reason: "Observe should report no external resource when the agent pool has never been created.",
			fields: fields{agentPools: &fake.AgentPoolClient{}},
			args:   args{cr: agentPoolWith("", nil)},
			want:   want{o: managed.ExternalObservation{}},
		},
		"NotFound": {
			reason: "Observe should report no external resource when Azure DevOps returns 404.",
			fields: fields{agentPools: &fake.AgentPoolClient{GetAgentPoolFn: func(_ context.Context, _ taskagent.GetAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
				return nil, agentPoolNotFoundErr()
			}}},
			args: args{cr: agentPoolWith("37", nil)},
			want: want{o: managed.ExternalObservation{ResourceExists: false}},
		},
		"UpToDate": {
			reason: "Observe should report the agent pool is up to date when managed fields match desired state.",
			fields: fields{agentPools: &fake.AgentPoolClient{GetAgentPoolFn: func(_ context.Context, _ taskagent.GetAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
				return taskAgentPool(37, nil), nil
			}}},
			args: args{cr: agentPoolWith("37", func(cr *v1alpha1.AgentPool) {
				cr.Spec.ForProvider.AutoProvision = boolPtr(true)
				cr.Spec.ForProvider.AutoUpdate = boolPtr(false)
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
		"NeedsUpdate": {
			reason: "Observe should report the agent pool is not up to date when the desired mutable fields differ.",
			fields: fields{agentPools: &fake.AgentPoolClient{GetAgentPoolFn: func(_ context.Context, _ taskagent.GetAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
				return taskAgentPool(37, nil), nil
			}}},
			args: args{cr: agentPoolWith("37", func(cr *v1alpha1.AgentPool) {
				cr.Spec.ForProvider.AutoUpdate = boolPtr(true)
			})},
			want: want{o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, condition: xpv2.TypeReady, reason: xpv2.ReasonAvailable},
		},
		"GetError": {
			reason: "Observe should wrap non-404 API errors.",
			fields: fields{agentPools: &fake.AgentPoolClient{GetAgentPoolFn: func(_ context.Context, _ taskagent.GetAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
				return nil, stderrors.New("boom")
			}}},
			args: args{cr: agentPoolWith("37", nil)},
			want: want{err: stderrors.New("boom")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.ctx == nil {
				tc.args.ctx = context.Background()
			}
			e := external{agentPools: tc.fields.agentPools}
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
		agentPools AgentPoolClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.AgentPool
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
		check  func(t *testing.T, cr *v1alpha1.AgentPool)
	}{
		"Success": {
			reason: "Create should send the desired payload and record the created pool id as the external name.",
			fields: fields{agentPools: &fake.AgentPoolClient{AddAgentPoolFn: func(_ context.Context, args taskagent.AddAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
				if args.Pool == nil || args.Pool.Name == nil || *args.Pool.Name != defaultAgentPoolName {
					t.Fatalf("AddAgentPool called with unexpected payload: %+v", args.Pool)
				}
				if args.Pool.IsHosted == nil || *args.Pool.IsHosted {
					t.Fatalf("AddAgentPool IsHosted = %+v, want false", args.Pool.IsHosted)
				}
				if args.Pool.AutoProvision == nil || !*args.Pool.AutoProvision {
					t.Fatalf("AddAgentPool AutoProvision = %+v, want true", args.Pool.AutoProvision)
				}
				if args.Pool.AutoUpdate == nil || *args.Pool.AutoUpdate {
					t.Fatalf("AddAgentPool AutoUpdate = %+v, want false", args.Pool.AutoUpdate)
				}
				return taskAgentPool(37, nil), nil
			}}},
			args: args{cr: agentPoolWith("", func(cr *v1alpha1.AgentPool) {
				cr.Spec.ForProvider.AutoProvision = boolPtr(true)
				cr.Spec.ForProvider.AutoUpdate = boolPtr(false)
			})},
			check: func(t *testing.T, cr *v1alpha1.AgentPool) {
				if got := meta.GetExternalName(cr); got != "37" {
					t.Fatalf("external name = %q, want %q", got, "37")
				}
				if got := cr.Status.AtProvider.Size; got != 2 {
					t.Fatalf("status size = %d, want 2", got)
				}
			},
		},
		"CreateError": {
			reason: "Create should wrap API errors.",
			fields: fields{agentPools: &fake.AgentPoolClient{AddAgentPoolFn: func(_ context.Context, _ taskagent.AddAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
				return nil, stderrors.New("boom")
			}}},
			args: args{cr: agentPoolWith("", nil)},
			want: want{err: stderrors.New("boom")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.ctx == nil {
				tc.args.ctx = context.Background()
			}
			e := external{agentPools: tc.fields.agentPools}
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
	type fields struct {
		agentPools AgentPoolClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.AgentPool
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
		"UpdateSuccess": {
			reason: "Update should patch only changed mutable fields.",
			fields: fields{agentPools: &fake.AgentPoolClient{
				GetAgentPoolFn: func(_ context.Context, _ taskagent.GetAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
					return taskAgentPool(37, nil), nil
				},
				UpdateAgentPoolFn: func(_ context.Context, args taskagent.UpdateAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
					if args.PoolId == nil || *args.PoolId != 37 {
						t.Fatalf("UpdateAgentPool id = %v, want 37", args.PoolId)
					}
					if args.Pool == nil || args.Pool.Name == nil || *args.Pool.Name != "renamed-pool" {
						t.Fatalf("UpdateAgentPool name patch = %+v, want renamed-pool", args.Pool)
					}
					if args.Pool.AutoUpdate == nil || !*args.Pool.AutoUpdate {
						t.Fatalf("UpdateAgentPool autoUpdate patch = %+v, want true", args.Pool)
					}
					if args.Pool.IsHosted != nil {
						t.Fatalf("UpdateAgentPool isHosted patch = %+v, want omitted for unchanged field", args.Pool)
					}
					return taskAgentPool(37, func(pool *taskagent.TaskAgentPool) {
						pool.Name = stringPtr("renamed-pool")
						pool.AutoUpdate = boolPtr(true)
					}), nil
				},
			}},
			args: args{cr: agentPoolWith("37", func(cr *v1alpha1.AgentPool) {
				cr.Spec.ForProvider.Name = "renamed-pool"
				cr.Spec.ForProvider.AutoProvision = boolPtr(true)
				cr.Spec.ForProvider.AutoUpdate = boolPtr(true)
			})},
		},
		"NoChanges": {
			reason: "Update should not call Azure DevOps when the resource is already up to date.",
			fields: fields{agentPools: &fake.AgentPoolClient{
				GetAgentPoolFn: func(_ context.Context, _ taskagent.GetAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
					return taskAgentPool(37, nil), nil
				},
				UpdateAgentPoolFn: func(_ context.Context, _ taskagent.UpdateAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
					t.Fatal("UpdateAgentPool should not be called when nothing changed")
					return nil, nil
				},
			}},
			args: args{cr: agentPoolWith("37", func(cr *v1alpha1.AgentPool) {
				cr.Spec.ForProvider.AutoProvision = boolPtr(true)
				cr.Spec.ForProvider.AutoUpdate = boolPtr(false)
			})},
		},
		"MissingAgentPoolID": {
			reason: "Update should fail when the external name cannot be resolved to an agent pool id.",
			fields: fields{agentPools: &fake.AgentPoolClient{}},
			args:   args{cr: agentPoolWith("", nil)},
			want:   want{err: stderrors.New(errMissingAgentPoolID)},
		},
		"GetError": {
			reason: "Update should wrap read errors from Azure DevOps.",
			fields: fields{agentPools: &fake.AgentPoolClient{
				GetAgentPoolFn: func(_ context.Context, _ taskagent.GetAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
					return nil, stderrors.New("boom")
				},
			}},
			args: args{cr: agentPoolWith("37", nil)},
			want: want{err: stderrors.New("boom")},
		},
		"UpdateError": {
			reason: "Update should wrap patch errors from Azure DevOps.",
			fields: fields{agentPools: &fake.AgentPoolClient{
				GetAgentPoolFn: func(_ context.Context, _ taskagent.GetAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
					return taskAgentPool(37, nil), nil
				},
				UpdateAgentPoolFn: func(_ context.Context, _ taskagent.UpdateAgentPoolArgs) (*taskagent.TaskAgentPool, error) {
					return nil, stderrors.New("boom")
				},
			}},
			args: args{cr: agentPoolWith("37", func(cr *v1alpha1.AgentPool) {
				cr.Spec.ForProvider.Name = "renamed-pool"
			})},
			want: want{err: stderrors.New("boom")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.ctx == nil {
				tc.args.ctx = context.Background()
			}
			e := external{agentPools: tc.fields.agentPools}
			_, err := e.Update(tc.args.ctx, tc.args.cr)
			gotErr := err
			if tc.want.err != nil {
				if unwrapped := stderrors.Unwrap(err); unwrapped != nil {
					gotErr = unwrapped
				}
			}
			if diff := cmp.Diff(tc.want.err, gotErr, runtimeTest.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type fields struct {
		agentPools AgentPoolClient
	}
	type args struct {
		ctx context.Context
		cr  *v1alpha1.AgentPool
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
			reason: "Delete should not call Azure DevOps when there is no agent pool id.",
			fields: fields{agentPools: &fake.AgentPoolClient{DeleteAgentPoolFn: func(_ context.Context, _ taskagent.DeleteAgentPoolArgs) error {
				t.Fatal("DeleteAgentPool should not be called when there is no agent pool id")
				return nil
			}}},
			args: args{cr: agentPoolWith("", nil)},
		},
		"Success": {
			reason: "Delete should pass the observed agent pool id to Azure DevOps.",
			fields: fields{agentPools: &fake.AgentPoolClient{DeleteAgentPoolFn: func(_ context.Context, args taskagent.DeleteAgentPoolArgs) error {
				if args.PoolId == nil || *args.PoolId != 37 {
					t.Fatalf("DeleteAgentPool id = %v, want 37", args.PoolId)
				}
				return nil
			}}},
			args: args{cr: agentPoolWith("37", nil)},
		},
		"AlreadyGone": {
			reason: "Delete should ignore not-found responses.",
			fields: fields{agentPools: &fake.AgentPoolClient{DeleteAgentPoolFn: func(_ context.Context, _ taskagent.DeleteAgentPoolArgs) error {
				return agentPoolNotFoundErr()
			}}},
			args: args{cr: agentPoolWith("37", nil)},
		},
		"DeleteError": {
			reason: "Delete should wrap non-404 errors.",
			fields: fields{agentPools: &fake.AgentPoolClient{DeleteAgentPoolFn: func(_ context.Context, _ taskagent.DeleteAgentPoolArgs) error {
				return stderrors.New("boom")
			}}},
			args: args{cr: agentPoolWith("37", nil)},
			want: want{err: stderrors.New("boom")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.ctx == nil {
				tc.args.ctx = context.Background()
			}
			e := external{agentPools: tc.fields.agentPools}
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
