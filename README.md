# Windsor Command Line Interface

<p align="center">
  <img src="docs/img/windsor-logo.png" alt="Windsor CLI Logo" style="width: 20%; height: auto;">
</p>

<p align="center">
  <img src="https://img.shields.io/github/v/release/windsorcli/cli" alt="GitHub release (latest by date)">
  <img src="https://img.shields.io/github/actions/workflow/status/windsorcli/cli/ci.yaml" alt="GitHub Workflow Status">
</p>

---

## Purpose

The Windsor CLI is designed to streamline the cloud-native developer experience. Built in Go, it runs seamlessly on Linux, macOS, and Windows.

Windsor addresses several challenges common in building and running software platforms by integrating various tools into a cohesive workflow:

- **Complete Local Cloud**: Simulates complete cloud-native infrastructure locally using a native virtualization platform (currently supports Colima).
- **Support Services**: Push and pull containers to local image registries, and browse your local services at `*.local.test` domains.
- **Livereload GitOps**: Reflects your source via a local git repository, enabling you to work with GitOps tooling locally.
- **Contextual Workflow**: Code once, deploy-to-many with an elegant contextual workflow, dynamically reconfiguring your toolchain as you target different deployment environments.

## Quick Start

- **[Setup and Installation](./docs/install/install.md)**
- **[Quick Start](./docs/tutorial/quick-start.md)**

## Supported Tools

The following tools are supported by the Windsor CLI:

- [**Docker**](https://github.com/docker/docker-ce)
- [**Kubernetes**](https://github.com/kubernetes/kubernetes)
- [**AWS**](https://github.com/aws/aws-cli)
- [**Terraform**](https://github.com/hashicorp/terraform)
- [**SOPS**](https://github.com/mozilla/sops)
- [**Localstack**](https://github.com/localstack/localstack)
- [**Colima**](https://github.com/abiosoft/colima)
- [**Talos Linux**](https://github.com/siderolabs/talos)

## Contributing

Contributions are welcome! To get started, fork the repository, create a new branch, make your changes, and submit a pull request. Ensure your code follows our standards and includes tests. Thank you for your contributions!

## License

Windsor CLI is licensed under the Mozilla Public License Version 2.0. See the [LICENSE](LICENSE) file for more details.

## Contact Information

If you have any questions or need further assistance, please feel free to open an issue on our GitHub repository.
