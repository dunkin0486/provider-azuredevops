// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package group

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/graph"

	xperrors "github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/group/v1alpha1"
	fakegraph "github.com/dunkin0486/provider-azuredevops/internal/controller/group/fake"
)

const testDescriptor = "vssgp.Uy0xLTktMTU1MTM3NDI0My0xMjM0NTY3ODkwLTEyMzQ1Njc4OS0xMjM0NTY3ODkwLTEyMzQ1Njc4OTAtMS0yMzQ1Njc4OTAtMQ"

var errBoom = xperrors.New("boom")

func groupCR(mutate func(*v1alpha1.Group)) *v1alpha1.Group {
	cr := &v1alpha1.Group{
		Spec: v1alpha1.GroupSpec{
			ForProvider: v1alpha1.GroupParameters{
				DisplayName: "Example Group",
				Description: "An example group",
			},
		},
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

func groupNotFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func TestObserve(t *testing.T) {
	type fields struct {
		client GroupClient
	}
	type args struct {
		cr *v1alpha1.Group
	}
	type want struct {
		observation managed.ExternalObservation
		err         error
		condition   xpv2.ConditionType
		reason      xpv2.ConditionReason
	}

	cases := map[string]struct {
		fields fields
		args   args
		want   want
	}{
		"NoExternalName": {
			fields: fields{client: &fakegraph.GroupClient{}},
			args:   args{cr: groupCR(nil)},
			want:   want{observation: managed.ExternalObservation{ResourceExists: false}},
		},
		"NotFound": {
			fields: fields{client: &fakegraph.GroupClient{
				GetGroupFn: func(_ context.Context, _ graph.GetGroupArgs) (*graph.GraphGroup, error) {
					return nil, groupNotFoundErr()
				},
			}},
			args: args{cr: groupCR(func(cr *v1alpha1.Group) {
				meta.SetExternalName(cr, testDescriptor)
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: false}},
		},
		"UpToDate": {
			fields: fields{client: &fakegraph.GroupClient{
				GetGroupFn: func(_ context.Context, args graph.GetGroupArgs) (*graph.GraphGroup, error) {
					if args.GroupDescriptor == nil || *args.GroupDescriptor != testDescriptor {
						t.Fatalf("GetGroup descriptor = %v, want %q", args.GroupDescriptor, testDescriptor)
					}
					return &graph.GraphGroup{
						Descriptor:  stringPtr(testDescriptor),
						DisplayName: stringPtr("Example Group"),
						Description: stringPtr("An example group"),
					}, nil
				},
			}},
			args: args{cr: groupCR(func(cr *v1alpha1.Group) {
				meta.SetExternalName(cr, testDescriptor)
			})},
			want: want{
				observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				condition:   xpv2.TypeReady,
				reason:      xpv2.ReasonAvailable,
			},
		},
		"NeedsUpdate": {
			fields: fields{client: &fakegraph.GroupClient{
				GetGroupFn: func(_ context.Context, _ graph.GetGroupArgs) (*graph.GraphGroup, error) {
					return &graph.GraphGroup{
						Descriptor:  stringPtr(testDescriptor),
						DisplayName: stringPtr("Old Name"),
						Description: stringPtr("An example group"),
					}, nil
				},
			}},
			args: args{cr: groupCR(func(cr *v1alpha1.Group) {
				meta.SetExternalName(cr, testDescriptor)
			})},
			want: want{
				observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false},
				condition:   xpv2.TypeReady,
				reason:      xpv2.ReasonAvailable,
			},
		},
		"GetGroupError": {
			fields: fields{client: &fakegraph.GroupClient{
				GetGroupFn: func(_ context.Context, _ graph.GetGroupArgs) (*graph.GraphGroup, error) {
					return nil, errBoom
				},
			}},
			args: args{cr: groupCR(func(cr *v1alpha1.Group) {
				meta.SetExternalName(cr, testDescriptor)
			})},
			want: want{err: xperrors.Wrap(errBoom, errGetGroup)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{groups: tc.fields.client}
			got, err := e.Observe(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Fatalf("Observe(...): -want error, +got error:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.observation, got); diff != "" {
				t.Fatalf("Observe(...): -want, +got:\n%s", diff)
			}
			if tc.want.condition != "" {
				gotCondition := tc.args.cr.Status.GetCondition(tc.want.condition)
				if gotCondition.Reason != tc.want.reason {
					t.Fatalf("Observe(...): condition %s reason = %q, want %q", tc.want.condition, gotCondition.Reason, tc.want.reason)
				}
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type fields struct {
		client GroupClient
	}
	type want struct {
		err      error
		external string
	}

	cases := map[string]struct {
		fields fields
		cr     *v1alpha1.Group
		want   want
	}{
		"Success": {
			fields: fields{client: &fakegraph.GroupClient{
				CreateGroupVstsFn: func(_ context.Context, args graph.CreateGroupVstsArgs) (*graph.GraphGroup, error) {
					if args.CreationContext == nil || args.CreationContext.DisplayName == nil || *args.CreationContext.DisplayName != "Example Group" {
						t.Fatalf("CreateGroupVsts display name = %v, want %q", args.CreationContext, "Example Group")
					}
					return &graph.GraphGroup{
						Descriptor:  stringPtr(testDescriptor),
						DisplayName: stringPtr("Example Group"),
						Description: stringPtr("An example group"),
					}, nil
				},
			}},
			cr:   groupCR(nil),
			want: want{external: testDescriptor},
		},
		"CreateError": {
			fields: fields{client: &fakegraph.GroupClient{
				CreateGroupVstsFn: func(_ context.Context, _ graph.CreateGroupVstsArgs) (*graph.GraphGroup, error) {
					return nil, errBoom
				},
			}},
			cr:   groupCR(nil),
			want: want{err: xperrors.Wrap(errBoom, errCreateGroup)},
		},
		"MissingDescriptorInResponse": {
			fields: fields{client: &fakegraph.GroupClient{
				CreateGroupVstsFn: func(_ context.Context, _ graph.CreateGroupVstsArgs) (*graph.GraphGroup, error) {
					return &graph.GraphGroup{}, nil
				},
			}},
			cr:   groupCR(nil),
			want: want{err: xperrors.New(errCreateGroup)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{groups: tc.fields.client}
			_, err := e.Create(context.Background(), tc.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Fatalf("Create(...): -want error, +got error:\n%s", diff)
			}
			if tc.want.external != "" && meta.GetExternalName(tc.cr) != tc.want.external {
				t.Fatalf("Create(...): external name = %q, want %q", meta.GetExternalName(tc.cr), tc.want.external)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type fields struct {
		client GroupClient
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		fields fields
		cr     *v1alpha1.Group
		want   want
	}{
		"Success": {
			fields: fields{client: &fakegraph.GroupClient{
				UpdateGroupFn: func(_ context.Context, args graph.UpdateGroupArgs) (*graph.GraphGroup, error) {
					if args.GroupDescriptor == nil || *args.GroupDescriptor != testDescriptor {
						t.Fatalf("UpdateGroup descriptor = %v, want %q", args.GroupDescriptor, testDescriptor)
					}
					if args.PatchDocument == nil || len(*args.PatchDocument) != 2 {
						t.Fatalf("UpdateGroup patch = %v, want 2 operations", args.PatchDocument)
					}
					return &graph.GraphGroup{Descriptor: stringPtr(testDescriptor)}, nil
				},
			}},
			cr: groupCR(func(cr *v1alpha1.Group) {
				meta.SetExternalName(cr, testDescriptor)
			}),
		},
		"MissingExternalName": {
			fields: fields{client: &fakegraph.GroupClient{}},
			cr:     groupCR(nil),
			want:   want{err: xperrors.New(errMissingExtern)},
		},
		"UpdateError": {
			fields: fields{client: &fakegraph.GroupClient{
				UpdateGroupFn: func(_ context.Context, _ graph.UpdateGroupArgs) (*graph.GraphGroup, error) {
					return nil, errBoom
				},
			}},
			cr: groupCR(func(cr *v1alpha1.Group) {
				meta.SetExternalName(cr, testDescriptor)
			}),
			want: want{err: xperrors.Wrap(errBoom, errUpdateGroup)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{groups: tc.fields.client}
			_, err := e.Update(context.Background(), tc.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Fatalf("Update(...): -want error, +got error:\n%s", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type fields struct {
		client GroupClient
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		fields fields
		cr     *v1alpha1.Group
		want   want
	}{
		"Success": {
			fields: fields{client: &fakegraph.GroupClient{
				DeleteGroupFn: func(_ context.Context, args graph.DeleteGroupArgs) error {
					if args.GroupDescriptor == nil || *args.GroupDescriptor != testDescriptor {
						t.Fatalf("DeleteGroup descriptor = %v, want %q", args.GroupDescriptor, testDescriptor)
					}
					return nil
				},
			}},
			cr: groupCR(func(cr *v1alpha1.Group) {
				meta.SetExternalName(cr, testDescriptor)
			}),
		},
		"NotFoundIgnored": {
			fields: fields{client: &fakegraph.GroupClient{
				DeleteGroupFn: func(_ context.Context, _ graph.DeleteGroupArgs) error {
					return groupNotFoundErr()
				},
			}},
			cr: groupCR(func(cr *v1alpha1.Group) {
				meta.SetExternalName(cr, testDescriptor)
			}),
		},
		"NoExternalName": {
			fields: fields{client: &fakegraph.GroupClient{
				DeleteGroupFn: func(_ context.Context, _ graph.DeleteGroupArgs) error {
					t.Fatal("DeleteGroup should not be called without an external name")
					return nil
				},
			}},
			cr: groupCR(nil),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{groups: tc.fields.client}
			_, err := e.Delete(context.Background(), tc.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Fatalf("Delete(...): -want error, +got error:\n%s", diff)
			}
		})
	}
}
