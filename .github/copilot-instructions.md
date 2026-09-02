# Copilot instructions for provider-azuredevops

## What this repo is

A native [Crossplane](https://crossplane.io) provider for Azure DevOps. It is
built from the [`crossplane/provider-template`](https://github.com/crossplane/provider-template)
scaffold and follows standard Crossplane provider conventions (managed
resources + controllers reconciled by `controller-runtime`, no Terraform/Upjet
involved).

**Current state:** the repo has *not yet* been run through
`make provider.prepare provider=AzureDevOps`. You will still see template
placeholders (`PROJECT_NAME := provider-template`, API group
`template.crossplane.io`, sample type `mytype`) in `Makefile`, `apis/`,
`internal/controller/`, and `package/`. Treat these as scaffolding to be
replaced as real resources are added, not as intentional product decisions.

## Roadmap / planned resources

See the GitHub Project ["Azure DevOps Crossplane Provider Roadmap"](https://github.com/users/dunkin0486/projects/7)
(Roadmap view) and issues #1–#10 for the prioritized list of managed
resources to implement first: `Project`, `GitRepository`,
`ServiceEndpointAzureRM`, `VariableGroup`, `BuildDefinition`,
`BranchPolicyMinReviewers`, `Team`, `GroupMembership`,
`ServiceEndpointGitHub`, `Environment` — roughly in that dependency order
(`Project` is the root nearly everything else references).

## Repo layout

- `apis/` — CRD types (`<group>/<version>/<kind>_types.go`), one Go module
  per API group. `zz_generated.*.go` files are generated — never hand-edit.
- `internal/controller/` — one package per managed resource implementing
  `managed.ExternalClient` (Observe/Create/Update/Delete) against the Azure
  DevOps REST API.
- `internal/controller/config/` — the `ProviderConfig` controller (auth/credentials).
- `package/crds/` — generated CRD YAML bundled into the provider xpkg.
- `examples/` — sample YAML manifests per resource, used for manual testing
  and documentation.
- `cluster/local/integration_tests.sh` — kind-based install/uninstall smoke
  test invoked by `make e2e.run` / `make test-integration`.
- `hack/helpers/` — codegen templates/scripts for `make provider.prepare`
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

## Style notes

- Keep resources' `Parameters`/`Observation` structs and controller logic
  scoped to one Azure DevOps resource per Go package — don't combine
  multiple ADO resource types in one controller.
- Prefer the smallest targeted `go test ./...` / `make test` subset that
  covers changed packages; only fall back to the full suite when needed.
- Don't hand-edit `zz_generated.*.go`, `package/crds/*.yaml`, or files
  under `build/` — regenerate or update the submodule instead.
