# Releasing

`provider-azuredevops` is distributed as a [Crossplane package][xpkg-spec] --
an OCI image containing the provider's CRDs and (embedded) controller image.
Crossplane packages aren't tied to any single registry: `spec.package` on a
`Provider` object can point at any OCI-compatible registry. This repo
publishes to **GitHub Container Registry (GHCR)**, `ghcr.io/dunkin0486`,
configured via `XPKG_REG_ORGS` in the `Makefile`.

## What CI does automatically

`.github/workflows/ci.yml`'s `publish-artifacts` job builds the package with
`make build.all` and pushes it with `make publish` on:

- Every push to `main` -- publishes an **unstable** build tagged with a
  git-describe-derived pseudo-version, e.g. `v0.0.0.12.gabc1234` (useful for
  testing the latest `main`, not recommended for production use).
- Every push of a `v*` tag -- publishes a **real, versioned release** tagged
  with that exact version, e.g. `v0.1.0`.

Both cases log in to `ghcr.io` using the workflow's own `GITHUB_TOKEN` (via
the `packages: write` permission on the job) -- no external account or
secret setup is required for GHCR.

The same job also pushes to `xpkg.upbound.io/cd0486` using the
`UPBOUND_MARKETPLACE_PUSH_ROBOT_USR`/`_PSW` repo secrets (a robot account
access ID/token from your Upbound account), which are configured -- so every
`main` and `v*` tag build publishes to both GHCR and Upbound's registry.

## Cutting a release

Tagging is automated via [release-please] -- you normally don't need to run
`git tag` manually.

### Automatic (default)

`.github/workflows/release-please.yml` runs on every push to `main` and
inspects commit messages since the last release for [conventional commit]
prefixes:

- `fix: ...` -> patch bump (`v0.1.0` -> `v0.1.1`)
- `feat: ...` -> minor bump (`v0.1.0` -> `v0.2.0`)
- `feat!: ...` / a `BREAKING CHANGE:` footer -> major bump (`v0.1.0` -> `v1.0.0`)

If a qualifying commit is found, release-please opens/updates a
`chore(main): release vX.Y.Z` pull request with an auto-generated
`CHANGELOG.md` entry. The workflow then auto-approves and auto-merges that
PR (using the `RELEASE_PLEASE_PAT` secret -- a human maintainer's token,
since a PR authored by `github-actions[bot]` can't approve itself, and
`main` requires 1 approval). Merging it makes release-please push the tag
and create a GitHub Release, then the workflow explicitly dispatches
`ci.yml` on that tag (a tag pushed by the default `GITHUB_TOKEN` doesn't
trigger other workflows on its own -- GitHub blocks that to avoid infinite
loops), which builds and publishes the versioned package to GHCR and
Upbound's registry.

**End result: merging a PR with a conventional-commit message (e.g.
`feat: add GitRepository resource`) fully releases a new version with no
further manual steps.**

If no commit since the last release matches the convention, nothing
happens beyond the usual unstable `main` build.

### Manual (fallback)

For merges that didn't use conventional-commit messages, or to force a
specific version, trigger the existing `Tag` workflow instead:

```shell
gh workflow run tag.yml -f version=v0.1.0 -f message="Release v0.1.0"
```

or tag and push locally:

```shell
git tag v0.1.0
git push origin v0.1.0
```

Either way, CI builds and pushes
`ghcr.io/dunkin0486/provider-azuredevops:v0.1.0` (multi-arch: `linux/amd64`,
`linux/arm64`) once the tag lands.

### After any release

Verify the GHCR package is pullable and its visibility is **public** the
first time you release (new GHCR packages inherit repo visibility, but
check under the repo's "Packages" tab -- `Package settings` -- since GHCR
occasionally requires manually linking/publicizing a package created via
`GITHUB_TOKEN`).

[release-please]: https://github.com/googleapis/release-please


[conventional commit]: https://www.conventionalcommits.org/

## Installing a released version

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-azuredevops
spec:
  package: ghcr.io/dunkin0486/provider-azuredevops:v0.1.0
```

or with Helm, at Crossplane install time:

```shell
helm install crossplane crossplane-stable/crossplane \
  --namespace crossplane-system --create-namespace \
  --set provider.packages='{ghcr.io/dunkin0486/provider-azuredevops:v0.1.0}'
```

## Upbound Marketplace

Now that the package is also pushed to `xpkg.upbound.io/cd0486` (see above),
installing from Upbound's registry directly works the same way as GHCR:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-azuredevops
spec:
  package: xpkg.upbound.io/cd0486/provider-azuredevops:v0.1.0
```

Getting *listed* on the [Upbound Marketplace](https://marketplace.upbound.io/providers?tier=community)
UI (so it's discoverable/browsable, not just installable by exact
reference) still appears to be a separate, likely self-service step through
the Upbound console -- other community providers (e.g.
[`ankasoftco/provider-cmdb`](https://github.com/ankasoftco/provider-cmdb),
which is listed) distribute solely via Docker Hub with no Upbound registry
push at all, so listing clearly isn't tied to which registry hosts the
package.

This hasn't been fully verified against Upbound's current process. Before
pursuing it:

1. Create a free account at [upbound.io](https://upbound.io).
2. Look for a "submit"/"list your provider" flow in the Upbound console or
   marketplace UI.
3. If nothing is self-service, ask in the [Crossplane Slack](https://slack.crossplane.io)
   (`#upbound` channel) or Upbound support.

This is tracked as a stretch goal in
[#41](https://github.com/dunkin0486/provider-azuredevops/issues/41) -- not
required for the provider to be installable and usable via GHCR.

[xpkg-spec]: https://github.com/crossplane/crossplane/blob/main/contributing/specifications/xpkg.md
