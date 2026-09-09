// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package groupmembership

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/graph"

	xperrors "github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/groupmembership/v1alpha1"
	fakegraph "github.com/dunkin0486/provider-azuredevops/internal/controller/groupmembership/fake"
)

const (
	testMemberDescriptor = "aadgp.Uy0xLTktMTU1MTM3NDI0My0xMjM0NTY3ODkwLTEyMzQ1Njc4OS0xMjM0NTY3ODkwLTEyMzQ1Njc4OTAtMS0xMjM0NTY3ODktMQ"
	testGroupDescriptor  = "vssgp.Uy0xLTktMTU1MTM3NDI0My0xMjM0NTY3ODkwLTEyMzQ1Njc4OS0xMjM0NTY3ODkwLTEyMzQ1Njc4OTAtMS0yMzQ1Njc4OTAtMQ"
)

func groupMembershipCR(mutate func(*v1alpha1.GroupMembership)) *v1alpha1.GroupMembership {
	cr := &v1alpha1.GroupMembership{
		Spec: v1alpha1.GroupMembershipSpec{
			ForProvider: v1alpha1.GroupMembershipParameters{
				MemberDescriptor: testMemberDescriptor,
				GroupDescriptor:  testGroupDescriptor,
			},
		},
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

func membershipNotFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func TestObserve(t *testing.T) {
	type fields struct {
		client GroupMembershipClient
	}
	type args struct {
		cr *v1alpha1.GroupMembership
	}
	type want struct {
		observation managed.ExternalObservation
		err         error
		condition   xpv2.ConditionType
		reason      xpv2.ConditionReason
		active      bool
		external    string
	}

	cases := map[string]struct {
		fields fields
		args   args
		want   want
	}{
		"NotFound": {
			fields: fields{client: &fakegraph.GroupMembershipClient{
				GetMembershipFn: func(_ context.Context, _ graph.GetMembershipArgs) (*graph.GraphMembership, error) {
					return nil, membershipNotFoundErr()
				},
			}},
			args: args{cr: groupMembershipCR(nil)},
			want: want{observation: managed.ExternalObservation{ResourceExists: false}},
		},
		"UpToDateActive": {
			fields: fields{client: &fakegraph.GroupMembershipClient{
				GetMembershipFn: func(_ context.Context, args graph.GetMembershipArgs) (*graph.GraphMembership, error) {
					if args.SubjectDescriptor == nil || *args.SubjectDescriptor != testMemberDescriptor {
						t.Fatalf("GetMembership subject = %v, want %q", args.SubjectDescriptor, testMemberDescriptor)
					}
					if args.ContainerDescriptor == nil || *args.ContainerDescriptor != testGroupDescriptor {
						t.Fatalf("GetMembership container = %v, want %q", args.ContainerDescriptor, testGroupDescriptor)
					}
					return &graph.GraphMembership{
						MemberDescriptor:    stringPtr(testMemberDescriptor),
						ContainerDescriptor: stringPtr(testGroupDescriptor),
					}, nil
				},
				GetMembershipStateFn: func(_ context.Context, args graph.GetMembershipStateArgs) (*graph.GraphMembershipState, error) {
					if args.SubjectDescriptor == nil || *args.SubjectDescriptor != testMemberDescriptor {
						t.Fatalf("GetMembershipState subject = %v, want %q", args.SubjectDescriptor, testMemberDescriptor)
					}
					return &graph.GraphMembershipState{Active: boolPtr(true)}, nil
				},
			}},
			args: args{cr: groupMembershipCR(nil)},
			want: want{
				observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				condition:   xpv2.TypeReady,
				reason:      xpv2.ReasonAvailable,
				active:      true,
				external:    externalNameFor(v1alpha1.GroupMembershipParameters{MemberDescriptor: testMemberDescriptor, GroupDescriptor: testGroupDescriptor}),
			},
		},
		"UpToDateInactive": {
			fields: fields{client: &fakegraph.GroupMembershipClient{
				GetMembershipFn: func(_ context.Context, _ graph.GetMembershipArgs) (*graph.GraphMembership, error) {
					return &graph.GraphMembership{}, nil
				},
				GetMembershipStateFn: func(_ context.Context, _ graph.GetMembershipStateArgs) (*graph.GraphMembershipState, error) {
					return &graph.GraphMembershipState{Active: boolPtr(false)}, nil
				},
			}},
			args: args{cr: groupMembershipCR(nil)},
			want: want{
				observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				condition:   xpv2.TypeReady,
				reason:      xpv2.ReasonAvailable,
				active:      false,
				external:    externalNameFor(v1alpha1.GroupMembershipParameters{MemberDescriptor: testMemberDescriptor, GroupDescriptor: testGroupDescriptor}),
			},
		},
		"DescriptorChangeRejected": {
			fields: fields{client: &fakegraph.GroupMembershipClient{
				GetMembershipFn: func(_ context.Context, _ graph.GetMembershipArgs) (*graph.GraphMembership, error) {
					t.Fatal("GetMembership should not be called when immutable descriptors change")
					return nil, nil
				},
			}},
			args: args{cr: groupMembershipCR(func(cr *v1alpha1.GroupMembership) {
				meta.SetExternalName(cr, externalNameFor(v1alpha1.GroupMembershipParameters{
					MemberDescriptor: testMemberDescriptor,
					GroupDescriptor:  testGroupDescriptor,
				}))
				cr.Spec.ForProvider.MemberDescriptor = "aadgp.changed"
			})},
			want: want{err: xperrors.Wrap(errors.New(errImmutableIdentity), errGetMembership)},
		},
		"MissingGroupDescriptorRejected": {
			fields: fields{client: &fakegraph.GroupMembershipClient{
				GetMembershipFn: func(_ context.Context, _ graph.GetMembershipArgs) (*graph.GraphMembership, error) {
					t.Fatal("GetMembership should not be called when groupDescriptor is unresolved")
					return nil, nil
				},
			}},
			args: args{cr: groupMembershipCR(func(cr *v1alpha1.GroupMembership) {
				cr.Spec.ForProvider.GroupDescriptor = ""
			})},
			want: want{err: xperrors.Wrap(errors.New(errMissingGroupDescriptor), errGetMembership)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{memberships: tc.fields.client}
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
			if tc.args.cr.Status.AtProvider.Active != tc.want.active {
				t.Fatalf("Observe(...): active = %t, want %t", tc.args.cr.Status.AtProvider.Active, tc.want.active)
			}
			if tc.want.external != "" && meta.GetExternalName(tc.args.cr) != tc.want.external {
				t.Fatalf("Observe(...): external name = %q, want %q", meta.GetExternalName(tc.args.cr), tc.want.external)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	cr := groupMembershipCR(func(cr *v1alpha1.GroupMembership) {
		cr.Spec.ForProvider.Mode = "overwrite"
	})

	e := external{memberships: &fakegraph.GroupMembershipClient{
		AddMembershipFn: func(_ context.Context, args graph.AddMembershipArgs) (*graph.GraphMembership, error) {
			if args.SubjectDescriptor == nil || *args.SubjectDescriptor != testMemberDescriptor {
				t.Fatalf("AddMembership subject = %v, want %q", args.SubjectDescriptor, testMemberDescriptor)
			}
			if args.ContainerDescriptor == nil || *args.ContainerDescriptor != testGroupDescriptor {
				t.Fatalf("AddMembership container = %v, want %q", args.ContainerDescriptor, testGroupDescriptor)
			}
			return &graph.GraphMembership{
				MemberDescriptor:    stringPtr(testMemberDescriptor),
				ContainerDescriptor: stringPtr(testGroupDescriptor),
			}, nil
		},
	}}

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create(...): unexpected error: %v", err)
	}
	if got := meta.GetExternalName(cr); got != externalNameFor(cr.Spec.ForProvider) {
		t.Fatalf("Create(...): external name = %q, want %q", got, externalNameFor(cr.Spec.ForProvider))
	}
	if !cr.Status.AtProvider.Active {
		t.Fatal("Create(...): active = false, want true")
	}
}

func TestCreateMissingGroupDescriptor(t *testing.T) {
	cr := groupMembershipCR(func(cr *v1alpha1.GroupMembership) {
		cr.Spec.ForProvider.GroupDescriptor = ""
	})

	e := external{memberships: &fakegraph.GroupMembershipClient{
		AddMembershipFn: func(_ context.Context, _ graph.AddMembershipArgs) (*graph.GraphMembership, error) {
			t.Fatal("AddMembership should not be called when groupDescriptor is unresolved")
			return nil, nil
		},
	}}

	_, err := e.Create(context.Background(), cr)
	if diff := cmp.Diff(errors.New(errMissingGroupDescriptor), err, test.EquateErrors()); diff != "" {
		t.Fatalf("Create(...): -want error, +got error:\n%s", diff)
	}
}

func TestUpdate(t *testing.T) {
	type args struct {
		cr *v1alpha1.GroupMembership
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"NoOp": {
			args: args{cr: groupMembershipCR(nil)},
		},
		"DescriptorChangeRejected": {
			args: args{cr: groupMembershipCR(func(cr *v1alpha1.GroupMembership) {
				meta.SetExternalName(cr, externalNameFor(v1alpha1.GroupMembershipParameters{
					MemberDescriptor: testMemberDescriptor,
					GroupDescriptor:  testGroupDescriptor,
				}))
				cr.Spec.ForProvider.GroupDescriptor = "vssgp.changed"
			})},
			want: want{err: errors.New(errImmutableIdentity)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{memberships: &fakegraph.GroupMembershipClient{}}
			_, err := e.Update(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Fatalf("Update(...): -want error, +got error:\n%s", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type fields struct {
		client GroupMembershipClient
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		fields fields
		want   want
	}{
		"Success": {
			fields: fields{client: &fakegraph.GroupMembershipClient{
				RemoveMembershipFn: func(_ context.Context, args graph.RemoveMembershipArgs) error {
					if args.SubjectDescriptor == nil || *args.SubjectDescriptor != testMemberDescriptor {
						t.Fatalf("RemoveMembership subject = %v, want %q", args.SubjectDescriptor, testMemberDescriptor)
					}
					if args.ContainerDescriptor == nil || *args.ContainerDescriptor != testGroupDescriptor {
						t.Fatalf("RemoveMembership container = %v, want %q", args.ContainerDescriptor, testGroupDescriptor)
					}
					return nil
				},
			}},
		},
		"NotFoundIgnored": {
			fields: fields{client: &fakegraph.GroupMembershipClient{
				RemoveMembershipFn: func(_ context.Context, _ graph.RemoveMembershipArgs) error {
					return membershipNotFoundErr()
				},
			}},
		},
		"UsesStoredIdentityOnDelete": {
			fields: fields{client: &fakegraph.GroupMembershipClient{
				RemoveMembershipFn: func(_ context.Context, args graph.RemoveMembershipArgs) error {
					if args.SubjectDescriptor == nil || *args.SubjectDescriptor != testMemberDescriptor {
						t.Fatalf("RemoveMembership subject = %v, want %q", args.SubjectDescriptor, testMemberDescriptor)
					}
					if args.ContainerDescriptor == nil || *args.ContainerDescriptor != testGroupDescriptor {
						t.Fatalf("RemoveMembership container = %v, want %q", args.ContainerDescriptor, testGroupDescriptor)
					}
					return nil
				},
			}},
			want: want{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{memberships: tc.fields.client}
			cr := groupMembershipCR(nil)
			if name == "UsesStoredIdentityOnDelete" {
				meta.SetExternalName(cr, externalNameFor(v1alpha1.GroupMembershipParameters{
					MemberDescriptor: testMemberDescriptor,
					GroupDescriptor:  testGroupDescriptor,
				}))
				cr.Spec.ForProvider.MemberDescriptor = "aadgp.changed"
				cr.Spec.ForProvider.GroupDescriptor = "vssgp.changed"
			}
			_, err := e.Delete(context.Background(), cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Fatalf("Delete(...): -want error, +got error:\n%s", diff)
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }
