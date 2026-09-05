# Copilot instructions for provider-azuredevops

## What this repo is

A native [Crossplane](https://crossplane.io) provider for Azure DevOps. It was
bootstrapped from the [`crossplane/provider-template`](https://github.com/crossplane/provider-template)
scaffold (now de-templated — see below) and follows standard Crossplane
provider conventions (managed resources + controllers reconciled by
`controller-runtime`, no Terraform/Upjet involved).

**Current state:** the repo has been de-templated
(`make provider.prepare provider=AzureDevOps` has been run): `PROJECT_NAME :=
provider-azuredevops` in `Makefile`, API group `azuredevops.crossplane.io` in
`apis/`, and the sample `MyType`/`mytype` API and controller have been
removed. The only resources that exist so far are the `ProviderConfig` /
`ProviderConfigUsage` scaffolding in `apis/v1alpha1` — no real Azure DevOps
managed resources have been implemented yet. See the roadmap below and the
"Foundation" issues (#30–#34) for the required groundwork (SDK adoption,
`ProviderConfig` design, shared client package, testing conventions) that
must land before/alongside the first real resource controller.

## Roadmap / planned resources

See the GitHub Project ["Azure DevOps Crossplane Provider Roadmap"](https://github.com/users/dunkin0486/projects/7)
(Roadmap view) for the full prioritized backlog. Highlights:
- **Foundation (P0, #30–#34):** de-template scaffold (done), adopt the
  official [`microsoft/azure-devops-go-api`](https://github.com/microsoft/azure-devops-go-api)
  Go SDK, design `ProviderConfig` (PAT-via-Secret auth), build a shared
  `internal/clients/azuredevops` client package, establish testing
  conventions.
- **Resources (#1–#10, highest priority):** `Project`, `GitRepository`,
  `ServiceEndpointAzureRM`, `VariableGroup`, `BuildDefinition`,
  `BranchPolicyMinReviewers`, `Team`, `GroupMembership`,
  `ServiceEndpointGitHub`, `Environment` — roughly in that dependency order
  (`Project` is the root nearly everything else references).
- **Resources (#12–#29, lower priority):** the remaining ADO resources
  (additional service endpoints, agent pools/queues, branch policies,
  release definitions, checks, wiki, feeds, etc.) — see individual issues.

## Repo layout

- `apis/` — CRD types (`<group>/<version>/<kind>_types.go`), one Go module
  per API group. `zz_generated.*.go` files are generated — never hand-edit.
- `internal/controller/` — one package per managed resource implementing
  `managed.ExternalClient` (Observe/Create/Update/Delete) against the Azure
  DevOps REST API.
- `internal/controller/config/` — the `ProviderConfig` controller (auth/credentials).
- `internal/clients/azuredevops/` — shared Azure DevOps client helpers built
  on the official [`microsoft/azure-devops-go-api`](https://github.com/microsoft/azure-devops-go-api)
  Go SDK (module `github.com/microsoft/azure-devops-go-api/azuredevops/v7`).
  **Every new resource controller's `Connect` should call `GetConfig` rather
  than resolving `ProviderConfig`/credentials itself.** Key helpers:
  - `GetConfig(ctx, kube, mg)` resolves the `ProviderConfig` or
    `ClusterProviderConfig` referenced by a managed resource
    (`resource.ModernManaged`), tracks its usage via
    `resource.NewProviderConfigUsageTracker`, and extracts its PAT via
    `resource.CommonCredentialExtractor`, returning a `*Config`.
  - `(*Config).Connection()` wraps `azuredevops.NewPatConnection` (also
    exposed directly as `NewConnection`); pass the result to the SDK's
    per-area `NewClient` functions (e.g. `core.NewClient`, `git.NewClient`,
    `build.NewClient`) to get a typed client for a given API area. **Use
    this SDK for all new resource controllers — do not hand-roll raw REST
    calls.**
  - `errors.go` translates SDK errors: `IsNotFound` (404, use with
    `resource.Ignore` in `Observe`/`Delete`), `IsThrottled` (429),
    `IsUnauthorized` (401/403), and `IsRetryable` (429/5xx/network errors).
  - `ListAll`/`PageFunc` (`pagination.go`) drain `ContinuationToken`-based
    list APIs into a single slice.
  - `Retry`/`DefaultBackoff` (`retry.go`) wrap a single API call with
    exponential backoff, retrying only while `IsRetryable` is true.
  - Per-resource client interfaces + fakes for unit-testing controllers
    (mirroring `provider-gitlab`'s `pkg/*/clients/<resource>/fake`
    convention) should live alongside each controller package as they're
    added, since they depend on a specific SDK client's method set.
- `package/crds/` — generated CRD YAML bundled into the provider xpkg.
- `examples/` — sample YAML manifests per resource, used for manual testing
  and documentation.
- `cluster/local/integration_tests.sh` — kind-based install/uninstall smoke
  test invoked by `make e2e.run` / `make test-integration`.
- `hack/helpers/` — codegen helper scripts for `make provider.prepare`
  and `make provider.addtype`.
- `build/` — git submodule (`crossplane/build`), the shared Makefile
  machinery. Don't edit; run `make submodules` to refresh it.

## Adding a new managed resource

1. `make provider.addtype provider=AzureDevOps group=<group> kind=<Kind>`
   scaffolds the API type + controller from `hack/helpers/*.tmpl`.
2. Register the new API in `apis/azuredevops.go` and the controller in
   `internal/controller/azuredevops.go`.
3. Define `<Kind>Parameters` (spec, user input) and `<Kind>Observation`
   (status, server-computed fields) — never mix the two.
4. Implement the `managed.ExternalClient` against the real Azure DevOps REST
   API (see issue bodies in the roadmap project for the specific API and
   fields per resource). Any secret-bearing fields (tokens, service
   principal secrets) must be sourced via `SecretKeySelector` refs — never
   stored in spec/status plaintext.
5. Use `crossplane-runtime` reference resolution
   (`<field>Ref`/`<field>Selector`) for cross-resource references (e.g. a
   `GitRepository` referencing a `Project`).
6. Add an example manifest under `examples/<kind>/`.
7. Run `make generate` to refresh deepcopy/CRD YAML, then `make lint` and
   `make test`.

## Build & test commands

- `make build` — compile the provider binary.
- `make generate` — regenerate deepcopy methods + CRD YAML after changing types.
- `make lint` — golangci-lint (see `.golangci.yml`).
- `make test` — Go unit tests.
- `make e2e.run` (alias for `make test-integration`) — **spins up a local
  kind cluster**, builds/loads the provider image, installs Crossplane +
  the provider package, and asserts the provider reaches `healthy` before
  tearing the cluster down. This requires Docker running locally.
- `make acceptance-tests` — the required-before-merge target: runs
  `make build.all` then `make test-integration` (i.e. builds the images
  first, then does the same kind-cluster install/health-check as above).
  Not run in CI; see "CI / PR requirements" below.
- `make dev` / `make dev-clean` — create/delete a long-lived local kind
  cluster for iterative manual testing (`kubectl apply` example manifests
  against a controller run via `make run`).

## CI / PR requirements

`.github/workflows/ci.yml` runs on every PR: `lint`, `check-diff`
(generated-file drift), and `unit-tests` (plus `publish-artifacts` on
pushes). All must pass.

Acceptance tests are **intentionally not run in CI** (a kind cluster + full
Crossplane install is expensive in GitHub Actions minutes). Instead, every
PR must have `make acceptance-tests` run **locally** before merge — it's a
required checkbox in `.github/PULL_REQUEST_TEMPLATE.md`. That target:
1. Builds the provider (and controller) images (`make build.all`).
2. Spins up a local kind cluster and installs Crossplane.
3. Loads the built image and installs the provider package.
4. Waits for the provider to report `healthy`, then uninstalls it and
   tears the cluster down (`cluster/local/integration_tests.sh`).

Requires Docker running locally. `make e2e.run` / `make test-integration`
remain available as lower-level aliases for the same script.

## Before opening a pull request

Every PR (including ones opened by Copilot/an agent) must have, locally,
before it is opened:
1. **`make acceptance-tests` run and passing** for the branch's changes (see
   above) — this is the same required checkbox enforced by
   `.github/PULL_REQUEST_TEMPLATE.md`, and it should be done as part of
   preparing the PR, not deferred until review time.
2. **A code review pass on the diff** — use the `code-review` agent (or
   equivalent) against the branch's changes before opening the PR, and fix
   any high/medium-confidence findings it reports. CI (`lint`,
   `check-diff`, `unit-tests`) validates mechanics but does not catch
   logic bugs, so a dedicated review is required in addition to CI.
3. **A [Conventional Commits](https://www.conventionalcommits.org/) PR
   title** — e.g. `feat: add GitRepository resource`, `fix: ...`,
   `chore: ...`. This repo squash-merges PRs, and the PR title becomes the
   `main` commit message that `release-please` (see `RELEASING.md`)
   inspects to decide whether/how to cut a release. A PR title without a
   `feat:`/`fix:`/etc. prefix means the merge is silently invisible to
   `release-please` and no release is triggered.

## PR merging policy

A human must always review and approve a pull request before it is merged.
Copilot (or any agent acting on its behalf) must never merge a PR — via
`gh pr merge`, the GitHub API/UI, auto-merge, or any other method — unless a
human explicitly instructs it to merge that specific PR in that moment.
Opening a PR, pushing commits, or a human saying a PR "looks good"/"is
ready" does not imply permission to merge; wait for an explicit merge
instruction.

## Style notes

- Keep resources' `Parameters`/`Observation` structs and controller logic
  scoped to one Azure DevOps resource per Go package — don't combine
  multiple ADO resource types in one controller.
- Prefer the smallest targeted `go test ./...` / `make test` subset that
  covers changed packages; only fall back to the full suite when needed.
- Don't hand-edit `zz_generated.*.go`, `package/crds/*.yaml`, or files
  under `build/` — regenerate or update the submodule instead.
