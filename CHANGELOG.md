# Changelog

## [0.5.0](https://github.com/dunkin0486/provider-azuredevops/compare/v0.4.0...v0.5.0) (2026-09-08)


### Features

* add AgentPool resource ([#94](https://github.com/dunkin0486/provider-azuredevops/issues/94)) ([821f427](https://github.com/dunkin0486/provider-azuredevops/commit/821f4270a6364668e9d6466410f1a15145ea273a))
* add ServiceEndpointDockerRegistry resource ([#99](https://github.com/dunkin0486/provider-azuredevops/issues/99)) ([6456169](https://github.com/dunkin0486/provider-azuredevops/commit/6456169d7522705542828cc6e81f6283a1cbb51e))
* add ServiceEndpointKubernetes resource ([#97](https://github.com/dunkin0486/provider-azuredevops/issues/97)) ([27157a9](https://github.com/dunkin0486/provider-azuredevops/commit/27157a98d6e7648aa8311b781ba4c106c8e539b8))


### Bug Fixes

* use salted PBKDF2-HMAC-SHA256 for docker registry/kubernetes secret hashes ([#101](https://github.com/dunkin0486/provider-azuredevops/issues/101)) ([7ac6dc2](https://github.com/dunkin0486/provider-azuredevops/commit/7ac6dc2c30a45cf6a38be5f8c2a5a05207087427)), closes [#100](https://github.com/dunkin0486/provider-azuredevops/issues/100)
* use salted PBKDF2-HMAC-SHA256 for secret drift-detection hashes ([#98](https://github.com/dunkin0486/provider-azuredevops/issues/98)) ([ebba81b](https://github.com/dunkin0486/provider-azuredevops/commit/ebba81b5e12513ab57148cf43241394b2a62b954)), closes [#95](https://github.com/dunkin0486/provider-azuredevops/issues/95)

## [0.4.0](https://github.com/dunkin0486/provider-azuredevops/compare/v0.3.1...v0.4.0) (2026-09-08)


### Features

* add Environment resource ([#91](https://github.com/dunkin0486/provider-azuredevops/issues/91)) ([3bf1a9c](https://github.com/dunkin0486/provider-azuredevops/commit/3bf1a9cb0a9d31dfd1c94cb0ca314a849b1e56a9))
* add ServiceEndpointGeneric resource ([#86](https://github.com/dunkin0486/provider-azuredevops/issues/86)) ([ff215e0](https://github.com/dunkin0486/provider-azuredevops/commit/ff215e02430ccfb26a4eadea32481c03cc301954))
* add ServiceEndpointGitHub resource ([#90](https://github.com/dunkin0486/provider-azuredevops/issues/90)) ([cc22743](https://github.com/dunkin0486/provider-azuredevops/commit/cc2274347518d549b213f41a77b5756b1d0f1d49))

## [0.3.1](https://github.com/dunkin0486/provider-azuredevops/compare/v0.3.0...v0.3.1) (2026-09-07)


### Bug Fixes

* embed Marketplace icon via up alpha xpkg append ([#80](https://github.com/dunkin0486/provider-azuredevops/issues/80)) ([4bad432](https://github.com/dunkin0486/provider-azuredevops/commit/4bad43226eb4b26ebbeb130be5441f9d640160f6)), closes [#55](https://github.com/dunkin0486/provider-azuredevops/issues/55)

## [0.3.0](https://github.com/dunkin0486/provider-azuredevops/compare/v0.2.0...v0.3.0) (2026-09-07)


### Features

* add Marketplace logo and fix package metadata apiVersion ([#78](https://github.com/dunkin0486/provider-azuredevops/issues/78)) ([fc7bd3b](https://github.com/dunkin0486/provider-azuredevops/commit/fc7bd3bf5788324c98ad422e9c3693c5d65f72ef))

## [0.2.0](https://github.com/dunkin0486/provider-azuredevops/compare/v0.1.1...v0.2.0) (2026-09-07)


### Features

* add BranchPolicyMinReviewers resource ([#76](https://github.com/dunkin0486/provider-azuredevops/issues/76)) ([30747ed](https://github.com/dunkin0486/provider-azuredevops/commit/30747ed6aeaf9028c3a450e94d29982355f2f587))
* add BuildDefinition managed resource ([#52](https://github.com/dunkin0486/provider-azuredevops/issues/52)) ([8ee0068](https://github.com/dunkin0486/provider-azuredevops/commit/8ee00689e60dedeb21f972888035f63e0e132cc2))
* add GroupMembership resource ([#75](https://github.com/dunkin0486/provider-azuredevops/issues/75)) ([c1121fe](https://github.com/dunkin0486/provider-azuredevops/commit/c1121fe325a4203ad3b1d0f9b180856de3e01cd0))
* add Team resource ([#74](https://github.com/dunkin0486/provider-azuredevops/issues/74)) ([d35e62a](https://github.com/dunkin0486/provider-azuredevops/commit/d35e62a0bf46c7f21e0a344c9c16994111da060e))

## [0.1.1](https://github.com/dunkin0486/provider-azuredevops/compare/v0.1.0...v0.1.1) (2026-09-05)


### Bug Fixes

* auto-dispatch ci.yml release build after manual tag creation ([#50](https://github.com/dunkin0486/provider-azuredevops/issues/50)) ([91f0710](https://github.com/dunkin0486/provider-azuredevops/commit/91f0710dcb305ea7c888245dac8b6abfd47797f0))
