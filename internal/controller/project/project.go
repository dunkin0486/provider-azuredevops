// SPDX-FileCopyrightText: 2025 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/operations"

	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"

	v1alpha1 "github.com/dunkin0486/provider-azuredevops/apis/project/v1alpha1"
	azuredevops "github.com/dunkin0486/provider-azuredevops/internal/clients/azuredevops"
)

const (
	errGetConfig       = "cannot get Azure DevOps client config"
	errNewClient       = "cannot create new Azure DevOps client"
	errGetProject      = "cannot get project"
	errResolveProcess  = "cannot resolve work item process template"
	errProcessNotFound = "work item process template not found"
	errCreateProject   = "cannot create project"
	errUpdateProject   = "cannot update project"
	errDeleteProject   = "cannot delete project"
	errWaitOperation   = "async operation did not complete"
)

// operationPollBackoff bounds how long Create/Update/Delete will actively
// wait for the Azure DevOps async operation they kick off to finish, before
// returning control to the reconciler (which will pick the operation's
// progress back up on the next Observe, since Azure DevOps project creation,
// update, and deletion are all asynchronous, operation-tracked calls).
var operationPollBackoff = wait.Backoff{
	Duration: 1 * time.Second,
	Factor:   1.5,
	Steps:    8,
}

// SetupGated adds a controller that reconciles Project managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup Project controller"))
		}
	}, v1alpha1.ProjectGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles Project managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.ProjectGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*v1alpha1.Project](&connector{
			kube: mgr.GetClient(),
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))), //nolint:staticcheck // TODO(jbw976) Crossplane needs to update to the new events API, see https://github.com/crossplane/crossplane/issues/7152
	}

	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}

	if o.MetricOptions != nil && o.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.ProjectList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.ProjectList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.ProjectGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Project{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector produces an ExternalClient by resolving the managed resource's
// ProviderConfig and building Azure DevOps SDK clients from it.
type connector struct {
	kube client.Client
}

// Connect resolves cr's ProviderConfig via the shared azuredevops client
// package and uses it to construct the Core and Operations API clients.
func (c *connector) Connect(ctx context.Context, cr *v1alpha1.Project) (managed.TypedExternalClient[*v1alpha1.Project], error) {
	cfg, err := azuredevops.GetConfig(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errGetConfig)
	}

	projectClient, opsClient, err := newProjectClient(ctx, cfg.Connection())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{project: projectClient, operations: opsClient}, nil
}

// An external observes, then either creates, updates, or deletes an
// external Azure DevOps Project to ensure it reflects the managed
// resource's desired state.
type external struct {
	project    ProjectClient
	operations OperationsClient
}

func (c *external) Observe(ctx context.Context, cr *v1alpha1.Project) (managed.ExternalObservation, error) { //nolint:gocyclo // Straightforward branching over a handful of well-formed states.
	name := meta.GetExternalName(cr)
	if name == "" {
		return managed.ExternalObservation{}, nil
	}

	tp, err := c.project.GetProject(ctx, core.GetProjectArgs{ProjectId: &name})
	if azuredevops.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetProject)
	}

	cr.Status.AtProvider = observationFromProject(tp)

	if tp.State != nil && *tp.State != core.ProjectStateValues.WellFormed {
		// The project is still being created or deleted by Azure DevOps.
		// Report it as existing (so we don't attempt to re-create it) but
		// not yet up to date, and let the next reconcile re-Observe its
		// progress.
		cr.SetConditions(xpv2.Creating())
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: true,
		}, nil
	}

	cr.SetConditions(xpv2.Available())

	upToDate := isUpToDate(cr.Spec.ForProvider, tp)

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: upToDate,
	}, nil
}

func (c *external) Create(ctx context.Context, cr *v1alpha1.Project) (managed.ExternalCreation, error) {
	p := cr.Spec.ForProvider

	tp := &core.TeamProject{
		Name: &p.Name,
	}
	if p.Description != "" {
		tp.Description = &p.Description
	}
	if p.Visibility != "" {
		v := core.ProjectVisibility(p.Visibility)
		tp.Visibility = &v
	}

	capabilities := map[string]map[string]string{}
	if p.VersionControl != "" {
		capabilities["versioncontrol"] = map[string]string{"sourceControlType": p.VersionControl}
	}
	if p.WorkItemTemplate != "" {
		templateID, err := c.resolveProcessTemplateID(ctx, p.WorkItemTemplate)
		if err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errResolveProcess)
		}
		capabilities["processTemplate"] = map[string]string{"templateTypeId": templateID}
	}
	if len(capabilities) > 0 {
		tp.Capabilities = &capabilities
	}

	// Azure DevOps project names are unique within an organization and
	// immutable, so we use the name as the external-name from the start --
	// this lets Observe look the project up by name even while its GUID is
	// still being assigned by the (asynchronous) create operation below.
	meta.SetExternalName(cr, p.Name)

	op, err := c.project.QueueCreateProject(ctx, core.QueueCreateProjectArgs{ProjectToCreate: tp})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateProject)
	}

	if err := c.waitForOperation(ctx, op); err != nil {
		// The create was successfully queued; a failure here just means we
		// didn't wait long enough for it to finish. The next Observe will
		// pick its progress back up via the project's reported State.
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateProject)
	}

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, cr *v1alpha1.Project) (managed.ExternalUpdate, error) {
	p := cr.Spec.ForProvider

	id := cr.Status.AtProvider.ID
	if id == "" {
		return managed.ExternalUpdate{}, errors.New(errUpdateProject)
	}

	projectID, err := parseUUID(id)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateProject)
	}

	update := &core.TeamProject{}
	if p.Description != "" {
		update.Description = &p.Description
	}
	if p.Visibility != "" {
		v := core.ProjectVisibility(p.Visibility)
		update.Visibility = &v
	}

	op, err := c.project.UpdateProject(ctx, core.UpdateProjectArgs{
		ProjectId:     projectID,
		ProjectUpdate: update,
	})
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateProject)
	}

	if err := c.waitForOperation(ctx, op); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateProject)
	}

	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, cr *v1alpha1.Project) (managed.ExternalDelete, error) {
	id := cr.Status.AtProvider.ID
	if id == "" {
		// Nothing to delete: we never observed a project GUID, so either it
		// was never created or a prior create operation is still in
		// flight. Either way there's nothing we can ask Azure DevOps to
		// delete yet.
		return managed.ExternalDelete{}, nil
	}

	projectID, err := parseUUID(id)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteProject)
	}

	op, err := c.project.QueueDeleteProject(ctx, core.QueueDeleteProjectArgs{ProjectId: projectID})
	if azuredevops.IsNotFound(err) {
		return managed.ExternalDelete{}, nil
	}
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteProject)
	}

	if err := c.waitForOperation(ctx, op); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteProject)
	}

	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(_ context.Context) error {
	return nil
}

// resolveProcessTemplateID looks up the process template GUID for a
// human-readable work item template name (e.g. "Agile", "Scrum", "Basic").
func (c *external) resolveProcessTemplateID(ctx context.Context, name string) (string, error) {
	processes, err := c.project.GetProcesses(ctx, core.GetProcessesArgs{})
	if err != nil {
		return "", err
	}
	if processes != nil {
		for _, p := range *processes {
			if p.Name != nil && strings.EqualFold(*p.Name, name) && p.Id != nil {
				return p.Id.String(), nil
			}
		}
	}
	return "", errors.Errorf("%s: %q", errProcessNotFound, name)
}

// waitForOperation polls an async Azure DevOps operation until it reaches a
// terminal state or operationPollBackoff is exhausted.
func (c *external) waitForOperation(ctx context.Context, op *operations.OperationReference) error {
	if op == nil || op.Id == nil {
		return nil
	}

	var last *operations.Operation
	err := wait.ExponentialBackoffWithContext(ctx, operationPollBackoff, func(ctx context.Context) (bool, error) {
		o, err := c.operations.GetOperation(ctx, operations.GetOperationArgs{OperationId: op.Id})
		if err != nil {
			return false, err
		}
		last = o
		if o.Status == nil {
			return false, nil
		}
		switch *o.Status {
		case operations.OperationStatusValues.Succeeded:
			return true, nil
		case operations.OperationStatusValues.Failed, operations.OperationStatusValues.Cancelled:
			return false, errors.Errorf("operation %s", *o.Status)
		default:
			return false, nil
		}
	})
	if err != nil {
		if last != nil && last.DetailedMessage != nil {
			return errors.Wrap(err, *last.DetailedMessage)
		}
		return errors.Wrap(err, errWaitOperation)
	}
	return nil
}

// observationFromProject translates an Azure DevOps TeamProject into a
// ProjectObservation.
func observationFromProject(tp *core.TeamProject) v1alpha1.ProjectObservation {
	o := v1alpha1.ProjectObservation{}
	if tp.Id != nil {
		o.ID = tp.Id.String()
	}
	if tp.State != nil {
		o.State = string(*tp.State)
	}
	if tp.Revision != nil {
		o.Revision = int64(*tp.Revision) //nolint:gosec // Revision numbers are small counters, no overflow risk in practice.
	}
	if tp.Url != nil {
		o.URL = *tp.Url
	}
	return o
}

// parseUUID parses an Azure DevOps project GUID observed in status back into
// a *uuid.UUID for use in SDK calls that require it.
func parseUUID(s string) (*uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse project id")
	}
	return &id, nil
}

// isUpToDate reports whether the mutable fields of tp already match the
// desired parameters. Name, VersionControl, and WorkItemTemplate are
// immutable once a project is created, so they're intentionally excluded.
func isUpToDate(p v1alpha1.ProjectParameters, tp *core.TeamProject) bool {
	if p.Description != "" {
		if tp.Description == nil || *tp.Description != p.Description {
			return false
		}
	}
	if p.Visibility != "" {
		if tp.Visibility == nil || string(*tp.Visibility) != p.Visibility {
			return false
		}
	}
	return true
}
