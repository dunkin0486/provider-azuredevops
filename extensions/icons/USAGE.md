# Usage

`icon.svg` is the Azure DevOps brand mark from the
[Simple Icons](https://github.com/simple-icons/simple-icons) project
(CC0 1.0 licensed). Keep square/viewBox dimensions; recolor via the `fill`
attribute if a different shade is needed rather than sourcing a new mark.

Marketplace listing icons (and other supported extensions -- release notes,
readme, additional docs) are attached to a published version with:

```sh
up alpha xpkg append --extensions-root=./extensions \
  xpkg.upbound.io/cd0486/provider-azuredevops:<version>
```

This repo's CI (`.github/workflows/ci.yml`, `publish-artifacts` job) runs
this automatically after every push to `xpkg.upbound.io/cd0486`, so no
manual step is required for new releases. See `RELEASING.md`'s "Listing
icon" section for details.
