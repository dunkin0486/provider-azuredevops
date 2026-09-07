// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package groupmembership

import (
	"context"
	"fmt"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/graph"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/groupmembership/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig          = "cannot get Azure DevOps client config"
	errNewClient          = "cannot create new Azure DevOps graph client"
	errGetMembership      = "cannot get group membership"
	errGetMembershipState = "cannot get group membership state"
	errCreateMembership   = "cannot create group membership"
	errDeleteMembership   = "cannot delete group membership"
	errImmutableIdentity  = "memberDescriptor and groupDescriptor are immutable"
)

// SetupGated adds a controller that reconciles GroupMembership managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup GroupMembership controller"))
		}
	}, v1alpha1.GroupMembershipGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles GroupMembership managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.GroupMembershipGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.GroupMembership](&connector{kube: mgr.GetClient()}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))), //nolint:staticcheck // TODO(jbw976) Crossplane needs to update to the new events API, see https://github.com/crossplane/crossplane/issues/7152
	}

	if o.Features.Enabled(feature.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}

	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}

	if o.MetricOptions != nil && o.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.GroupMembershipList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.GroupMembershipList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.GroupMembershipGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.GroupMembership{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube client.Client
}

// Connect resolves cr's ProviderConfig via the shared azuredevops client
// package and uses it to construct the Graph API client. The Graph client is
// resolved by resource area, so the SDK automatically follows the graph
// service's vssps.dev.azure.com endpoint from the base organization URL.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.GroupMembership) (managed.TypedExternalClient[*v1alpha1.GroupMembership], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	membershipClient, err := newGroupMembershipClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{memberships: membershipClient}, nil
}

type external struct {
	memberships GroupMembershipClient
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.GroupMembership) (managed.ExternalObservation, error) {
	memberDescriptor, groupDescriptor, err := membershipIdentityFor(cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetMembership)
	}

	_, err = e.getMembership(ctx, memberDescriptor, groupDescriptor)
	if azuredevops.IsNotFound(err) {
		cr.Status.AtProvider = v1alpha1.GroupMembershipObservation{}
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetMembership)
	}

	state, err := e.getMembershipState(ctx, memberDescriptor)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetMembershipState)
	}

	meta.SetExternalName(cr, externalNameForDescriptors(memberDescriptor, groupDescriptor))
	cr.Status.AtProvider = observationFromMembershipState(state)
	cr.SetConditions(xpv2.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.GroupMembership) (managed.ExternalCreation, error) {
	p := cr.Spec.ForProvider

	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		_, createErr := e.memberships.AddMembership(ctx, graph.AddMembershipArgs{
			SubjectDescriptor:   stringPtr(p.MemberDescriptor),
			ContainerDescriptor: stringPtr(p.GroupDescriptor),
		})
		return createErr
	})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateMembership)
	}

	meta.SetExternalName(cr, externalNameFor(p))
	cr.Status.AtProvider = v1alpha1.GroupMembershipObservation{Active: true}
	return managed.ExternalCreation{}, nil
}

// Update is intentionally a no-op after validating that the identifying
// descriptors have not changed. Azure DevOps graph memberships are modeled as
// an add/remove relationship, so changing either descriptor would require a
// delete-and-recreate and could otherwise orphan the original membership.
func (e *external) Update(_ context.Context, cr *v1alpha1.GroupMembership) (managed.ExternalUpdate, error) {
	if _, _, err := membershipIdentityFor(cr); err != nil {
		return managed.ExternalUpdate{}, err
	}
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.GroupMembership) (managed.ExternalDelete, error) {
	memberDescriptor, groupDescriptor, err := membershipIdentityForDelete(cr)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteMembership)
	}

	err = azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		return e.memberships.RemoveMembership(ctx, graph.RemoveMembershipArgs{
			SubjectDescriptor:   stringPtr(memberDescriptor),
			ContainerDescriptor: stringPtr(groupDescriptor),
		})
	})
	return managed.ExternalDelete{}, errors.Wrap(resource.Ignore(azuredevops.IsNotFound, err), errDeleteMembership)
}

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

func (e *external) getMembership(ctx context.Context, memberDescriptor, groupDescriptor string) (*graph.GraphMembership, error) {
	var membership *graph.GraphMembership
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var getErr error
		membership, getErr = e.memberships.GetMembership(ctx, graph.GetMembershipArgs{
			SubjectDescriptor:   stringPtr(memberDescriptor),
			ContainerDescriptor: stringPtr(groupDescriptor),
		})
		return getErr
	})
	return membership, err
}

func (e *external) getMembershipState(ctx context.Context, memberDescriptor string) (*graph.GraphMembershipState, error) {
	var state *graph.GraphMembershipState
	err := azuredevops.Retry(ctx, azuredevops.DefaultBackoff, func() error {
		var getErr error
		state, getErr = e.memberships.GetMembershipState(ctx, graph.GetMembershipStateArgs{
			SubjectDescriptor: stringPtr(memberDescriptor),
		})
		return getErr
	})
	return state, err
}

func observationFromMembershipState(state *graph.GraphMembershipState) v1alpha1.GroupMembershipObservation {
	observation := v1alpha1.GroupMembershipObservation{Active: true}
	if state != nil && state.Active != nil {
		observation.Active = *state.Active
	}
	return observation
}

func membershipIdentityFor(cr *v1alpha1.GroupMembership) (memberDescriptor, groupDescriptor string, err error) {
	storedMember, storedGroup, hasStored, err := membershipIdentityFromExternalName(meta.GetExternalName(cr))
	if err != nil {
		return "", "", err
	}

	p := cr.Spec.ForProvider
	if hasStored {
		if storedMember != p.MemberDescriptor || storedGroup != p.GroupDescriptor {
			return "", "", errors.New(errImmutableIdentity)
		}
		return storedMember, storedGroup, nil
	}

	return p.MemberDescriptor, p.GroupDescriptor, nil
}

func membershipIdentityForDelete(cr *v1alpha1.GroupMembership) (memberDescriptor, groupDescriptor string, err error) {
	storedMember, storedGroup, hasStored, err := membershipIdentityFromExternalName(meta.GetExternalName(cr))
	if err != nil {
		return "", "", err
	}
	if hasStored {
		return storedMember, storedGroup, nil
	}

	p := cr.Spec.ForProvider
	return p.MemberDescriptor, p.GroupDescriptor, nil
}

func membershipIdentityFromExternalName(name string) (memberDescriptor, groupDescriptor string, hasStored bool, err error) {
	if name == "" {
		return "", "", false, nil
	}

	groupDescriptor, memberDescriptor, ok := strings.Cut(name, ":")
	if !ok || groupDescriptor == "" || memberDescriptor == "" {
		return "", "", false, errors.Errorf("invalid external-name %q", name)
	}
	return memberDescriptor, groupDescriptor, true, nil
}

func externalNameFor(p v1alpha1.GroupMembershipParameters) string {
	return externalNameForDescriptors(p.MemberDescriptor, p.GroupDescriptor)
}

func externalNameForDescriptors(memberDescriptor, groupDescriptor string) string {
	return fmt.Sprintf("%s:%s", groupDescriptor, memberDescriptor)
}

func stringPtr(s string) *string {
	return &s
}
