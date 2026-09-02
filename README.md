# provider-azuredevops

`provider-azuredevops` is a native [Crossplane](https://crossplane.io/)
provider for managing [Azure DevOps](https://azure.microsoft.com/en-us/products/devops)
resources — projects, repositories, pipelines, service connections, branch
policies, and more — declaratively via Kubernetes.

It is built from the [crossplane/provider-template] scaffold and uses the
official [microsoft/azure-devops-go-api] Go SDK to talk to the Azure DevOps
REST API.

See the [project roadmap] for the full list of planned resources and their
priority.

## Developing

1. Run `make submodules` to initialize the `build` Make submodule used for
   CI/CD.
1. Use `make provider.addtype` to scaffold a new resource type:
   ```shell
   export group=repos # lower case, e.g. core, repos, pipelines, security
   export type=GitRepository # Camel case, e.g. GitRepository, BuildDefinition
   make provider.addtype provider=AzureDevOps group=${group} kind=${type}
   ```
1. Register the new type in `apis/azuredevops.go` and its controller in
   `internal/controller/azuredevops.go`.
1. Implement the controller against the Azure DevOps API using the shared
   client helpers in `internal/clients/azuredevops` and the official SDK.
1. Run `make reviewable` to run code generation, linters, and tests.
1. Run `make build` to build the provider.
1. Run `make acceptance-tests` to run the full local kind-based acceptance
   test suite (builds images, stands up a kind cluster, installs Crossplane,
   installs the provider package, and verifies it reports healthy). See
   `.github/copilot-instructions.md` for prerequisites. This is required
   before opening a PR — see `.github/PULL_REQUEST_TEMPLATE.md`.

Refer to Crossplane's [CONTRIBUTING.md] file for more information on how the
Crossplane community prefers to work. The [Provider Development][provider-dev]
guide may also be of use.

[crossplane/provider-template]: https://github.com/crossplane/provider-template
[microsoft/azure-devops-go-api]: https://github.com/microsoft/azure-devops-go-api
[project roadmap]: https://github.com/users/dunkin0486/projects/7
[CONTRIBUTING.md]: https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md
[provider-dev]: https://github.com/crossplane/crossplane/blob/master/contributing/guide-provider-development.md
