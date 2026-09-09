// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package userentitlement

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/licensing"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/memberentitlementmanagement"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/webapi"

	xperrors "github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/userentitlement/v1alpha1"
	fakemem "github.com/dunkin0486/provider-azuredevops/internal/controller/userentitlement/fake"
)

const testUserID = "11111111-1111-1111-1111-111111111111"
const testProjectID = "22222222-2222-2222-2222-222222222222"

var errBoom = xperrors.New("boom")

func userEntitlementCR(mutate func(*v1alpha1.UserEntitlement)) *v1alpha1.UserEntitlement {
	cr := &v1alpha1.UserEntitlement{
		Spec: v1alpha1.UserEntitlementSpec{
			ForProvider: v1alpha1.UserEntitlementParameters{
				PrincipalName: "user@example.com",
				AccessLevel:   "express",
			},
		},
	}
	if mutate != nil {
		mutate(cr)
	}
	return cr
}

func notFoundErr() error {
	code := 404
	return azuredevops.WrappedError{StatusCode: &code}
}

func TestObserve(t *testing.T) {
	type fields struct {
		client UserEntitlementClient
	}
	type args struct {
		cr *v1alpha1.UserEntitlement
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
			fields: fields{client: &fakemem.UserEntitlementClient{}},
			args:   args{cr: userEntitlementCR(nil)},
			want:   want{observation: managed.ExternalObservation{ResourceExists: false}},
		},
		"InvalidExternalName": {
			fields: fields{client: &fakemem.UserEntitlementClient{}},
			args: args{cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, "not-a-uuid")
			})},
			want: want{err: xperrors.Wrap(mustParseErr("not-a-uuid"), errParseExternalName)},
		},
		"NotFound": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				GetUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.GetUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlement, error) {
					return nil, notFoundErr()
				},
			}},
			args: args{cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, testUserID)
			})},
			want: want{observation: managed.ExternalObservation{ResourceExists: false}},
		},
		"UpToDate": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				GetUserEntitlementFn: func(_ context.Context, args memberentitlementmanagement.GetUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlement, error) {
					if args.UserId == nil || args.UserId.String() != testUserID {
						t.Fatalf("GetUserEntitlement userID = %v, want %q", args.UserId, testUserID)
					}
					return &memberentitlementmanagement.UserEntitlement{
						Id: uuidPtr(uuid.MustParse(testUserID)),
						AccessLevel: &licensing.AccessLevel{
							AccountLicenseType: accountLicenseTypePtr(licensing.AccountLicenseTypeValues.Express),
						},
					}, nil
				},
			}},
			args: args{cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, testUserID)
			})},
			want: want{
				observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				condition:   xpv2.TypeReady,
				reason:      xpv2.ReasonAvailable,
			},
		},
		"NeedsUpdate": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				GetUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.GetUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlement, error) {
					return &memberentitlementmanagement.UserEntitlement{
						Id: uuidPtr(uuid.MustParse(testUserID)),
						AccessLevel: &licensing.AccessLevel{
							AccountLicenseType: accountLicenseTypePtr(licensing.AccountLicenseTypeValues.Stakeholder),
						},
					}, nil
				},
			}},
			args: args{cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, testUserID)
			})},
			want: want{
				observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false},
				condition:   xpv2.TypeReady,
				reason:      xpv2.ReasonAvailable,
			},
		},
		"GetError": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				GetUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.GetUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlement, error) {
					return nil, errBoom
				},
			}},
			args: args{cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, testUserID)
			})},
			want: want{err: xperrors.Wrap(errBoom, errGetUserEntitlement)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{userEntitlements: tc.fields.client}
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
		client UserEntitlementClient
	}
	type want struct {
		err      error
		external string
	}

	cases := map[string]struct {
		fields fields
		cr     *v1alpha1.UserEntitlement
		want   want
	}{
		"Success": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				AddUserEntitlementFn: func(_ context.Context, args memberentitlementmanagement.AddUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPostResponse, error) {
					if args.UserEntitlement == nil || args.UserEntitlement.User == nil || args.UserEntitlement.User.PrincipalName == nil || *args.UserEntitlement.User.PrincipalName != "user@example.com" {
						t.Fatalf("AddUserEntitlement principal = %v, want %q", args.UserEntitlement, "user@example.com")
					}
					return &memberentitlementmanagement.UserEntitlementsPostResponse{
						IsSuccess: boolPtr(true),
						UserEntitlement: &memberentitlementmanagement.UserEntitlement{
							Id: uuidPtr(uuid.MustParse(testUserID)),
						},
					}, nil
				},
			}},
			cr:   userEntitlementCR(nil),
			want: want{external: testUserID},
		},
		"CreateError": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				AddUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.AddUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPostResponse, error) {
					return nil, errBoom
				},
			}},
			cr:   userEntitlementCR(nil),
			want: want{err: xperrors.Wrap(errBoom, errCreateUserEntitlement)},
		},
		"OperationFailed": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				AddUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.AddUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPostResponse, error) {
					return &memberentitlementmanagement.UserEntitlementsPostResponse{IsSuccess: boolPtr(false)}, nil
				},
			}},
			cr:   userEntitlementCR(nil),
			want: want{err: xperrors.New(errCreateUserEntitlementFailed)},
		},
		"WithProjectEntitlements": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				AddUserEntitlementFn: func(_ context.Context, args memberentitlementmanagement.AddUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPostResponse, error) {
					if args.UserEntitlement.ProjectEntitlements == nil || len(*args.UserEntitlement.ProjectEntitlements) != 1 {
						t.Fatalf("AddUserEntitlement projectEntitlements = %v, want 1 entry", args.UserEntitlement.ProjectEntitlements)
					}
					return &memberentitlementmanagement.UserEntitlementsPostResponse{
						IsSuccess: boolPtr(true),
						UserEntitlement: &memberentitlementmanagement.UserEntitlement{
							Id: uuidPtr(uuid.MustParse(testUserID)),
						},
					}, nil
				},
			}},
			cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				cr.Spec.ForProvider.ProjectEntitlements = []v1alpha1.ProjectEntitlementParameters{
					{ProjectID: testProjectID, GroupType: "projectContributor"},
				}
			}),
			want: want{external: testUserID},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{userEntitlements: tc.fields.client}
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
		client UserEntitlementClient
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		fields fields
		cr     *v1alpha1.UserEntitlement
		want   want
	}{
		"Success": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				GetUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.GetUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlement, error) {
					return &memberentitlementmanagement.UserEntitlement{Id: uuidPtr(uuid.MustParse(testUserID))}, nil
				},
				UpdateUserEntitlementFn: func(_ context.Context, args memberentitlementmanagement.UpdateUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPatchResponse, error) {
					if args.UserId == nil || args.UserId.String() != testUserID {
						t.Fatalf("UpdateUserEntitlement userID = %v, want %q", args.UserId, testUserID)
					}
					if args.Document == nil || len(*args.Document) != 1 {
						t.Fatalf("UpdateUserEntitlement patch = %v, want 1 operation", args.Document)
					}
					return &memberentitlementmanagement.UserEntitlementsPatchResponse{IsSuccess: boolPtr(true)}, nil
				},
			}},
			cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, testUserID)
			}),
		},
		"WithProjectEntitlementDiff": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				GetUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.GetUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlement, error) {
					return &memberentitlementmanagement.UserEntitlement{Id: uuidPtr(uuid.MustParse(testUserID))}, nil
				},
				UpdateUserEntitlementFn: func(_ context.Context, args memberentitlementmanagement.UpdateUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPatchResponse, error) {
					if args.Document == nil || len(*args.Document) != 2 {
						t.Fatalf("UpdateUserEntitlement patch = %v, want 2 operations (accessLevel replace + project add)", args.Document)
					}
					op := (*args.Document)[1]
					if op.Op == nil || *op.Op != webapi.OperationValues.Add || op.Path == nil || *op.Path != "/projectEntitlements" {
						t.Fatalf("UpdateUserEntitlement project op = %+v, want add on /projectEntitlements", op)
					}
					return &memberentitlementmanagement.UserEntitlementsPatchResponse{IsSuccess: boolPtr(true)}, nil
				},
			}},
			cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, testUserID)
				cr.Spec.ForProvider.ProjectEntitlements = []v1alpha1.ProjectEntitlementParameters{
					{ProjectID: testProjectID, GroupType: "projectContributor"},
				}
			}),
		},
		"MissingExternalName": {
			fields: fields{client: &fakemem.UserEntitlementClient{}},
			cr:     userEntitlementCR(nil),
			want:   want{err: xperrors.New(errMissingExternalName)},
		},
		"InvalidExternalName": {
			fields: fields{client: &fakemem.UserEntitlementClient{}},
			cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, "not-a-uuid")
			}),
			want: want{err: xperrors.Wrap(mustParseErr("not-a-uuid"), errParseExternalName)},
		},
		"GetError": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				GetUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.GetUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlement, error) {
					return nil, errBoom
				},
			}},
			cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, testUserID)
			}),
			want: want{err: xperrors.Wrap(errBoom, errGetUserEntitlement)},
		},
		"UpdateError": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				GetUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.GetUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlement, error) {
					return &memberentitlementmanagement.UserEntitlement{Id: uuidPtr(uuid.MustParse(testUserID))}, nil
				},
				UpdateUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.UpdateUserEntitlementArgs) (*memberentitlementmanagement.UserEntitlementsPatchResponse, error) {
					return nil, errBoom
				},
			}},
			cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, testUserID)
			}),
			want: want{err: xperrors.Wrap(errBoom, errUpdateUserEntitlement)},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{userEntitlements: tc.fields.client}
			_, err := e.Update(context.Background(), tc.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Fatalf("Update(...): -want error, +got error:\n%s", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type fields struct {
		client UserEntitlementClient
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		fields fields
		cr     *v1alpha1.UserEntitlement
		want   want
	}{
		"Success": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				DeleteUserEntitlementFn: func(_ context.Context, args memberentitlementmanagement.DeleteUserEntitlementArgs) error {
					if args.UserId == nil || args.UserId.String() != testUserID {
						t.Fatalf("DeleteUserEntitlement userID = %v, want %q", args.UserId, testUserID)
					}
					return nil
				},
			}},
			cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, testUserID)
			}),
		},
		"NotFoundIgnored": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				DeleteUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.DeleteUserEntitlementArgs) error {
					return notFoundErr()
				},
			}},
			cr: userEntitlementCR(func(cr *v1alpha1.UserEntitlement) {
				meta.SetExternalName(cr, testUserID)
			}),
		},
		"NoExternalName": {
			fields: fields{client: &fakemem.UserEntitlementClient{
				DeleteUserEntitlementFn: func(_ context.Context, _ memberentitlementmanagement.DeleteUserEntitlementArgs) error {
					t.Fatal("DeleteUserEntitlement should not be called without an external name")
					return nil
				},
			}},
			cr: userEntitlementCR(nil),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{userEntitlements: tc.fields.client}
			_, err := e.Delete(context.Background(), tc.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Fatalf("Delete(...): -want error, +got error:\n%s", diff)
			}
		})
	}
}

func TestProjectEntitlementsEqual(t *testing.T) {
	desired := []v1alpha1.ProjectEntitlementParameters{
		{ProjectID: testProjectID, GroupType: "projectContributor"},
	}
	currentMatching := &[]memberentitlementmanagement.ProjectEntitlement{
		{
			ProjectRef: &memberentitlementmanagement.ProjectRef{Id: uuidPtr(uuid.MustParse(testProjectID))},
			Group:      &memberentitlementmanagement.Group{GroupType: groupTypePtr(memberentitlementmanagement.GroupTypeValues.ProjectContributor)},
		},
	}
	currentMismatched := &[]memberentitlementmanagement.ProjectEntitlement{
		{
			ProjectRef: &memberentitlementmanagement.ProjectRef{Id: uuidPtr(uuid.MustParse(testProjectID))},
			Group:      &memberentitlementmanagement.Group{GroupType: groupTypePtr(memberentitlementmanagement.GroupTypeValues.ProjectReader)},
		},
	}

	if !projectEntitlementsEqual(desired, currentMatching) {
		t.Fatal("projectEntitlementsEqual(...) = false, want true for matching entitlements")
	}
	if projectEntitlementsEqual(desired, currentMismatched) {
		t.Fatal("projectEntitlementsEqual(...) = true, want false for mismatched entitlements")
	}
	if projectEntitlementsEqual(nil, nil) != true {
		t.Fatal("projectEntitlementsEqual(nil, nil) = false, want true")
	}
	if projectEntitlementsEqual(desired, nil) {
		t.Fatal("projectEntitlementsEqual(desired, nil) = true, want false")
	}
}

func boolPtr(v bool) *bool { return &v }

func mustParseErr(s string) error {
	_, err := uuid.Parse(s)
	return err
}
