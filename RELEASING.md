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

1. Make sure `main` is green (CI passing) and `CHANGELOG`-worthy changes
   since the last tag are ready to ship.
2. Tag the release with a `v`-prefixed [semver](https://semver.org/) version
   and push the tag:
   ```shell
   git tag v0.1.0
   git push origin v0.1.0
   ```
3. CI builds and pushes `ghcr.io/dunkin0486/provider-azuredevops:v0.1.0`
   (multi-arch: `linux/amd64`, `linux/arm64`).
4. Verify the package is pullable and its visibility is **public** the first
   time you release (new GHCR packages inherit repo visibility, but check
   under the repo's "Packages" tab -- `Package settings` -- the first time,
   since GHCR occasionally requires manually linking/publicizing a package
   created via `GITHUB_TOKEN`).
5. (Optional) Draft a GitHub Release for the tag summarizing user-facing
   changes.

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
