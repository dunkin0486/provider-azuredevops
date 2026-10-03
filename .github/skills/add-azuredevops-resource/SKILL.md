---
name: add-azuredevops-resource
description: "Use in the dunkin0486/provider-azuredevops repo when asked to implement one or more new Crossplane managed resources from roadmap issues (e.g. 'Resource: <Kind>' issues), to review/merge a backlog of Dependabot PRs, or to validate pending resource/dependency PRs end-to-end in a local kind cluster before requesting human review. Covers the full loop: dependency-PR triage, parallel resource implementation via background agents in git worktrees, post-merge rebase, and kind-cluster acceptance validation."
---

# /add-azuredevops-resource

Repeatable workflow for this repo (`dunkin0486/provider-azuredevops`, a Crossplane provider for Azure DevOps) covering two related jobs that are usually requested together or back-to-back:

1. **Triage and merge a backlog of Dependabot dependency-bump PRs.**
2. **Implement one or more new managed resources from roadmap issues in parallel**, each ending in its own PR, then **validate everything together in a local kind cluster** before handing off for human review/merge.

This repo's `AGENTS.md`/custom instructions are the source of truth for conventions (testing pattern, PR policy, etc.) — this skill is the operational playbook for *orchestrating* that work across multiple PRs/agents in one sitting. Re-read the repo's custom instructions at the start of every session; if they conflict with anything below, the custom instructions win.

## Hard rules (do not violate)

- **Never merge a PR without an explicit human instruction to merge** *that specific PR or batch*. Approving + merging already-green Dependabot PRs is normal once the human says "go ahead" / "merge those", but a brand-new `release-please` PR must **never** be approved or merged unless the human explicitly says so — they often want to bundle several merged changes into one release and will trigger it themselves. If unsure whether a PR is a release PR, check its author/title (`release-please[bot]`, title starting with `chore(main): release`) and leave it alone.
- **Agents never merge their own PRs.** Every background agent that implements a resource must open a PR and stop — leave it for human review.
- Only touch files inside the worktree assigned to a given unit of work. Never edit the main checkout at the repo root while background agents or other worktrees are active against the same branch.
- Don't hand-edit `zz_generated.*.go`, `package/crds/*.yaml`, or anything under `build/` (git submodule) — regenerate via `make generate` or update the submodule.

## Part 1 — Dependency PR triage (Dependabot backlog)

1. List open PRs: `gh pr list --state open --json number,title,author,isDraft,mergeable`. Separate GitHub Actions/workflow bumps (touch only `.github/workflows/**`) from Go module bumps (touch `go.mod`/`go.sum`) — the former almost never conflict with each other; the latter frequently do because they all touch the same two files.
2. For every PR, check CI with `gh pr checks <n>` and confirm `mergeable == MERGEABLE`.
3. Check branch protection once per session: `gh api repos/<owner>/<repo>/branches/main/protection`. If `required_pull_request_reviews.required_approving_review_count > 0`, you must approve before merging — only do this after the human has told you to proceed with merging (see hard rules above). If `dismiss_stale_reviews` is true, know that **every Dependabot rebase will dismiss your approval** — you'll need to re-approve after each rebase.
4. If `required_status_checks.strict == true`, a PR can only merge if its branch is up to date with the current `main` tip. This means **merges of same-file PRs (e.g. several go.mod bumps) must happen strictly one at a time**, rebasing the next one after each merge:
   - Merge PR A (`gh pr merge <n> --squash --delete-branch`).
   - Comment `@dependabot rebase` on every other still-open PR touching the same files (or just the next one in queue) and wait for the new commit + CI to go green (poll `gh pr checks <n>` and `gh pr view <n> --json mergeStateStatus,reviewDecision` every ~15s; `mergeStateStatus` is more reliable than `mergeable` for "is this ready right now" — `CLEAN` means go, `BLOCKED` usually means CI or review is still pending, `BEHIND`/`DIRTY` means it needs another rebase).
   - Re-approve if `reviewDecision == REVIEW_REQUIRED` (stale-review dismissal from the rebase).
   - Merge, then move to the next PR and repeat.
   - **Write this as a bash loop with `set +e`-safe error handling** — do NOT use `set -e` with `gh pr checks` in a condition/command-substitution context; `gh pr checks` exits non-zero while any check is pending, which silently kills a `set -e` script after the first PR. Use explicit `if`/`continue`/`break` logic instead and tolerate non-zero exits from `gh` commands you're polling.
   - It's normal for Dependabot to auto-close a PR mid-loop with a comment like "X is up-to-date now, so this is no longer needed" once an unrelated merge satisfies its version constraint transitively (e.g. a `k8s.io/client-go` bump becoming redundant after `k8s.io/api`/`k8s.io/apimachinery` bumps land). Treat `state == CLOSED` (not `MERGED`) as a legitimate terminal state, not a failure — just move on.
5. Run this whole loop as an **async/background bash command** (it can take 10-20+ minutes for a long queue) and poll it periodically with `read_bash` rather than blocking the conversation.

## Part 2 — Implementing new resources in parallel from roadmap issues

This repo's resources are all small, independent Go packages (`apis/<resource>/v1alpha1`, `internal/controller/<resource>`), so N roadmap issues can be implemented fully in parallel by N background agents, each in its own git worktree.

### 2.1 Set up one git worktree + branch per resource

This repo's convention (see any `pado-*` sibling directories next to the main checkout) is one throwaway worktree per unit of work, named `../pado-<short-name>`, branched from the **latest** `origin/main`:

```bash
cd <main-repo-checkout>
git fetch origin main --quiet
git worktree add ../pado-<resourcelowercase> -b feature/<resourcelowercase>-resource origin/main
```

Do this for all N resources before dispatching agents, so each agent gets an isolated working directory and branch — no file-lock contention, no risk of one agent's `git checkout` clobbering another's.

### 2.2 Find the closest existing resource to use as a template

Before writing the agent prompts, identify the most similar already-implemented resource in `internal/controller/` / `apis/` (e.g. for a new `BranchPolicyXyz` resource, `branchpolicyminreviewers` is the canonical template — it demonstrates CRD types with `*Ref`/`*Selector` cross-resource references, a `managed.ExternalClient` Observe/Create/Update/Delete implementation against the `azure-devops-go-api` `policy` SDK package, typed-settings JSON encode/decode against the ADO "Policy Configurations" API, scope/branch-ref matching, immutable-field handling, and annotation-based tracking). Also always point agents at `internal/controller/team/client.go` + `internal/controller/team/fake/fake.go` for the resource-scoped-client-interface-plus-hand-written-fake pattern used for every controller's unit tests.

### 2.3 Dispatch one background `general-purpose` agent per resource

**Before dispatching, confirm which model to run the agents with.** This work (implementing a full CRD/controller/client/fake/test suite against an undocumented-in-code external API, self-checking against issue acceptance criteria) benefits from a high-capability model. Default to the best available **Opus** model (e.g. `claude-opus-5` / `claude-opus-5.5`, whichever is current) and use the `ask_user` tool to let the human confirm or override before launching:

```
ask_user: "Which model should the resource-implementation agents use?"
  field "model": type=string, enum of available high-capability models, default="claude-opus-5" (or latest Opus variant available)
```

Pass the confirmed choice as the `model` parameter on every `task` tool call in this step (same model for all N agents unless the human asks for different ones per resource). Don't skip this prompt even if Opus seems like the obvious default — the human may want to trade cost/speed for quality on a given batch.

Launch all N agents in the same response (parallel background `task` tool calls, `mode: "background"`). Each prompt must be **fully self-contained** (agents are stateless subprocesses) and should specify, at minimum:

- The exact worktree path to work in and the branch it's already on (created in 2.1), and an explicit instruction to **never** edit the main repo checkout — only read it for reference.
- Which existing resource package(s) to treat as the template (2.2), and what specifically to copy/adapt (CRD types file shape, controller control flow, client interface + fake pattern, test table shape).
- The target Azure DevOps REST API, including the exact policy-type GUID / endpoint and the concrete spec/status fields, taken straight from the GitHub issue body (`gh issue view <n> --json body -q .body`) — don't paraphrase loosely, quote the issue's acceptance criteria so the agent can self-check against them.
- If multiple agents' resources are policy-configuration variants of the same ADO API (as with branch policies), explicitly tell each agent about its siblings ("agent X is concurrently implementing Y in its own worktree/branch — ignore it, you won't conflict since you're isolated") so it doesn't get confused or wait on them.
- The full list of required deliverables: CRD types + doc package file, controller + `client.go` + `fake/fake.go`, table-driven tests covering not-found/up-to-date/needs-update/deleted/error paths for Observe, and success/error paths for Create/Update/Delete, the two registration edits (`apis/azuredevops.go`, `internal/controller/azuredevops.go` — alphabetically ordered import + scheme/setup entries), an example manifest under `examples/<resource>/`, and running `make generate`/`make lint`/`make test` (or targeted `go test ./apis/<resource>/... ./internal/controller/<resource>/...`) until green. Explicitly say **do not** run `make acceptance-tests`/`make e2e.run` inside the agent (expensive, and will be done centrally afterward per Part 3).
- Conventional-commit commit message and PR title (`feat: add <Kind> resource`), a PR body with `Closes #<issue>` and a checklist mirroring the issue's acceptance criteria, the `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>` trailer, and **explicit instruction not to merge the PR** — open it and stop.
- A final "report back" instruction (files changed, lint/test result, PR URL) so you can summarize without re-reading every file yourself.

While agents run in the background, do independent work in parallel (e.g. Part 1's dependency-PR loop) rather than polling — you'll get a notification when each agent finishes.

### 2.4 After each agent finishes

Read results with `read_agent`. Sanity-check the PR's CI with `gh pr checks <n>` and confirm the base commit isn't stale:

```bash
git fetch origin main --quiet
main_sha=$(git rev-parse origin/main)
gh pr view <n> --json baseRefOid -q .baseRefOid   # compare to $main_sha
```

If other PRs (e.g. the Part 1 dependency backlog) merged into `main` *after* the agent's worktree was branched, every agent's branch is now behind and must be rebased (see 2.5) before it's safe to validate/merge.

### 2.5 Rebase each feature branch onto current `main`

Per worktree:

```bash
cd ../pado-<resourcelowercase>
git fetch origin main --quiet
git rebase origin/main        # should be a clean, conflict-free rebase since each resource's files are disjoint
go build ./... && go test ./apis/<resource>/... ./internal/controller/<resource>/...
git push --force-with-lease origin feature/<resourcelowercase>-resource
```

Registration-file edits (`apis/azuredevops.go`, `internal/controller/azuredevops.go`) are the only files multiple resource PRs touch, so conflicts there are possible if two resource branches are later merged together locally (see Part 3) but should *not* appear when each branch is rebased individually onto `main` (no other resource's registration edits are on `main` yet, since none have merged).

## Part 3 — Local kind-cluster acceptance validation (before requesting human review)

The repo's own policy requires `make acceptance-tests` to pass locally before a PR is opened for merge (see `.github/PULL_REQUEST_TEMPLATE.md` / repo custom instructions). When several resource PRs are ready at once, validate them **together** in one combined pass instead of spinning up a kind cluster per PR — Docker + kind + Crossplane install is the expensive part, and the resources are independent:

1. **Check Docker is running** (`docker info`); if not, `open -a Docker` and poll every ~10s until `docker info` succeeds (can take 1-2 minutes cold-start).
2. **Create one throwaway combined worktree** off `origin/main`, merge every resource branch you want to validate into it:
   ```bash
   git worktree add ../pado-validate-<topic> -b chore/validate-<topic> origin/main
   cd ../pado-validate-<topic>
   git merge --no-edit origin/feature/<resource-a>-resource
   git merge --no-edit origin/feature/<resource-b>-resource   # repeat per branch
   ```
   Expect merge conflicts **only** in `apis/azuredevops.go` and `internal/controller/azuredevops.go` (every resource touches both). Resolve by keeping both sides' import + registration lines, re-sorted alphabetically — never drop an entry. Every other file is disjoint per resource and auto-merges cleanly.
   This combined branch is **validation-only scaffolding** — never push it, never open a PR from it, delete the worktree when done (`git worktree remove ../pado-validate-<topic>`).
3. **Init the build submodule** if the worktree is fresh (`git submodule update --init --recursive`) — `make generate`/`make build.all` fail with "No rule to make target" until this is done in each new worktree.
4. Run `make generate` and confirm `git status --short` is empty afterward (no drift) — this catches CRD/deepcopy staleness per-branch before you even get to kind.
5. Run `go build ./...` then the full `go test ./apis/... ./internal/controller/...` on the combined tree as a fast pre-check.
6. Run the real acceptance test as a long-lived background process (it builds provider images, creates a kind cluster, installs Crossplane, loads the image, installs the provider package, waits for `Healthy`, then tears the cluster down):
   ```bash
   cd ../pado-validate-<topic>
   nohup make acceptance-tests > /tmp/acceptance-tests.log 2>&1 &
   ```
   This can take 10-20+ minutes (cross-compiling the provider binary for `linux_amd64` is the slowest step). Poll `tail -n 60 /tmp/acceptance-tests.log` periodically rather than blocking; look for the final `[ OK ] acceptance tests passed` line, or `[ FAIL ]`/non-zero exit plus a stack trace on failure.
7. **This validates provider packaging, CRD installation, and that the controller manager reaches `Healthy`** — it does *not* exercise real Azure DevOps API calls (no live PAT/org in this environment). For deeper validation that the new CRs reconcile through to the controller's `Observe`/`Create` logic (schema acceptance, reference resolution, no panics), additionally, once the cluster is up (re-use `make dev` instead of `make acceptance-tests` for this if you need an interactive cluster+controller to poke at):
   - Apply a `ProviderConfig` backed by a dummy/placeholder PAT secret.
   - Apply the example manifests for each new resource (and their prerequisite `Project`/`GitRepository`/etc. objects) from `examples/<resource>/`.
   - Check `kubectl get <kind>` status conditions and controller logs: expect `Synced=False`/auth or network errors against the fake PAT (since there's no real ADO org), but **no panics, no schema-validation rejections, no reference-resolution failures**. That absence of crash/validation errors is the pass/fail signal in a sandbox without real ADO credentials.
8. Report per-PR: CI status, rebase status, and the combined acceptance-test outcome, and remind the human that approving/merging is their call (per the hard rules above).

## Summary checklist to report back to the user

- [ ] Dependency PRs: N merged, N auto-closed-as-superseded (if any), 0 left needing attention; any `release-please` PR explicitly **not** touched.
- [ ] Resource PRs: one per issue, each with CI green, rebased onto latest `main`, not merged.
- [ ] `make generate` no-op on every branch (individually and combined).
- [ ] `go test ./...` green on every branch (individually and combined).
- [ ] `make acceptance-tests` result (pass/fail + log excerpt) from the combined kind-cluster run.
- [ ] Explicit note that a human needs to review/approve/merge the resource PRs (and separately decide when to trigger a release).
