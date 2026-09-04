<!--
Thank you for helping to improve Crossplane!

Please read through https://git.io/fj2m9 if this is your first time opening a
Crossplane pull request. Find us in https://slack.crossplane.io/messages/dev if
you need any help contributing.
-->

### Description of your changes

<!--
Briefly describe what this pull request does. Be sure to direct your reviewers'
attention to anything that needs special consideration.

We love pull requests that resolve an open Crossplane issue. If yours does, you
can uncomment the below line to indicate which issue your PR fixes, for example
"Fixes #500":

-->
Fixes #

I have:

- [ ] Read and followed Crossplane's [contribution process].
- [ ] Run `make reviewable` to ensure this PR is ready for review.
- [ ] Run `make acceptance-tests` locally (spins up a kind cluster, builds
      and loads the provider image, installs Crossplane + the provider, and
      verifies it becomes healthy) and confirmed it passes. This is **not**
      run in CI to conserve GitHub Actions minutes, so it must be verified
      locally before merge.
- [ ] This PR's title follows [Conventional Commits](https://www.conventionalcommits.org/)
      (e.g. `feat: ...`, `fix: ...`) -- PRs are squash-merged, so the title
      becomes the `main` commit message that `release-please` inspects to
      decide whether to cut a release (see `RELEASING.md`).
- [ ] Added `backport release-x.y` labels to auto-backport this PR if necessary.

### How has this code been tested

<!--
Before reviewers can be confident in the correctness of this pull request, it
needs to tested and shown to be correct. Briefly describe the testing that has
already been done or which is planned for this change.
-->

[contribution process]: https://git.io/fj2m9
