# Releasing

`provider-azuredevops` is distributed as a [Crossplane package][xpkg-spec] --
an OCI image containing the provider's CRDs and (embedded) controller image.
Crossplane packages aren't tied to any single registry: `spec.package` on a
`Provider` object can point at any OCI-compatible registry. This repo
publishes to both **GitHub Container Registry (GHCR)**, `ghcr.io/dunkin0486`,
and **Upbound's registry**, `xpkg.upbound.io/cd0486` (see "Upbound
Marketplace" below), configured via `XPKG_REG_ORGS` in the `Makefile` /
`ci.yml`.

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
secret setup is required for GHCR. The same run also logs in to
`xpkg.upbound.io` using the `UPBOUND_MARKETPLACE_PUSH_ROBOT_USR`/`_PSW`
secrets and pushes there too (see "Upbound Marketplace" below).

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
loops), which builds and publishes the versioned package to GHCR.

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

Either way, CI builds and pushes both
`ghcr.io/dunkin0486/provider-azuredevops:v0.1.0` and
`xpkg.upbound.io/cd0486/provider-azuredevops:v0.1.0` (multi-arch:
`linux/amd64`, `linux/arm64`) once the tag lands.

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
  package: xpkg.upbound.io/cd0486/provider-azuredevops:v0.1.0
```

`ghcr.io/dunkin0486/provider-azuredevops:v0.1.0` works identically -- both
registries carry the same multi-arch image.

or with Helm, at Crossplane install time:

```shell
helm install crossplane crossplane-stable/crossplane \
  --namespace crossplane-system --create-namespace \
  --set provider.packages='{xpkg.upbound.io/cd0486/provider-azuredevops:v0.1.0}'
```

## Upbound Marketplace

Publishing to Upbound's registry (`xpkg.upbound.io/cd0486`) is **enabled**.
Per [docs.upbound.io/manuals/platform/robots](https://docs.upbound.io/manuals/platform/robots/),
`docker login xpkg.upbound.io` requires a **Robot account** token (not a
personal account/PAT), assigned to a **Team** with push permission on the
`provider-azuredevops` repository under the `cd0486` org. That's set up,
and the robot's ID/token are stored as the `UPBOUND_MARKETPLACE_PUSH_ROBOT_USR`/
`_PSW` GitHub Actions secrets on this repo -- `ci.yml`'s `publish-artifacts`
job appends `xpkg.upbound.io/cd0486` to `XPKG_REG_ORGS` whenever those
secrets are present (see the "Publish Artifacts" step), so both an
unstable `main` build and every tagged release push to both GHCR and
Upbound with no further manual steps.

### Marketplace listing (discoverability)

Per [docs.upbound.io/manuals/marketplace/overview](https://docs.upbound.io/manuals/marketplace/overview/),
"all extensions in the Marketplace are OCI images served from
repositories" -- there's no separate build/package step beyond pushing a
valid xpkg to a registry. Whether a given repository is *browsable* in
the Marketplace UI (as opposed to merely installable by an exact
`spec.package` reference, which works today) is controlled by a
visibility/"list in Marketplace" toggle in the Upbound console under the
`cd0486` org's repository settings (`console.upbound.io`) -- this isn't
documented in the public docs, but was confirmed by inspecting the
console directly. That toggle has been enabled for
`provider-azuredevops`, and the listing is **confirmed live** at
[marketplace.upbound.io/providers/cd0486/provider-azuredevops](https://marketplace.upbound.io/providers/cd0486/provider-azuredevops).
Per the checklist in [#55], add the Marketplace badge to `README.md`
(not yet done as of this writing) and close #55.

### Listing icon

Per [docs.upbound.io/manuals/marketplace/packages](https://docs.upbound.io/manuals/marketplace/packages/#add-documentation-icons-and-other-assets-to-your-package),
the **only** Upbound-documented way to attach an icon (or release notes,
readme, additional docs, SBOMs) to a Marketplace listing is to append it
to an already-pushed package version with the alpha `up` CLI:

```shell
up alpha xpkg append --extensions-root=./extensions \
  xpkg.upbound.io/cd0486/provider-azuredevops:<version>
```

The `meta.crossplane.io/iconURI` annotation in `package/crossplane.yaml`
(pointing at `extensions/icons/icon.svg`'s raw GitHub URL) is *not* a
documented or confirmed-working Marketplace rendering mechanism -- it's a
convention some community providers use as a fallback comment, but it did
not make the icon appear on our listing (confirmed after v0.3.0). The
`up alpha xpkg append` step above is what actually matters.

This is wired into CI (`.github/workflows/ci.yml`, `publish-artifacts`
job): after a tagged release is pushed to `xpkg.upbound.io/cd0486`, an
"Install up CLI" step installs a pinned `up` version and an "Append
Marketplace Extensions" step runs the command above against the release's
`VERSION` (as computed by `make build.vars`). Both steps are gated on
`UPBOUND_MARKETPLACE_PUSH_ROBOT_USR` being set and only run for `v*` tags
(unstable main-channel builds don't need Marketplace extensions).

This repo doesn't sign packages (no cosign step configured), so there's no
append-vs-sign ordering concern here, unlike some other community
providers.

**Note:** the append step only affects *future* published versions -- it
does not retroactively add the icon to versions already pushed before this
was wired in (e.g. v0.1.0-v0.3.0). Those would need `up alpha xpkg append`
run against them manually (requires Upbound registry credentials) if the
icon is wanted on old versions too.

[xpkg-spec]: https://github.com/crossplane/crossplane/blob/main/contributing/specifications/xpkg.md
[#43]: https://github.com/dunkin0486/provider-azuredevops/issues/43
[#55]: https://github.com/dunkin0486/provider-azuredevops/issues/55

