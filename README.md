## [Windsor Command Line Interface](https://windsorcli.github.io)

![GitHub release (latest by date)](https://img.shields.io/github/v/release/windsorcli/cli)
![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/windsorcli/cli/ci.yaml)

The Windsor Command Line Interface (CLI) is a powerful tool designed to streamline your workflow and enhance productivity. With a small suite of intuitive commands, the Windsor CLI allows you to efficiently manage contexts for other tools in your development environment.

## [Purpose](#purpose)

The Windsor CLI is designed to simplify and enhance the development workflow for developers working on various projects. Its primary purpose is to provide a unified command-line interface that streamlines project setup, configuration management, and integration with other tools. 

By offering a consistent and efficient way to manage "contexts", the Windsor CLI aims to reduce the time and effort required for repetitive tasks, allowing developers to focus more on coding and less on setup and configuration.

Key objectives of the Windsor CLI include:

- **Efficiency**: Start with a pre-configured, working, and fully functional environment.
- **Consistency**: Ensure a standardized approach and consistent tool usage across different environments and teams.
- **Flexibility**: Support a wide range of configurations and integrations to accommodate diverse project needs.
- **Scalability**: Scale the environment and the production workload.
- **Project Initialization**: Quickly set up new projects with predefined configurations.
- **Configuration Management**: Easily manage and switch between different project configurations.
- **Shell Integration**: Seamlessly integrates with your shell environment for enhanced productivity.
- **Cross-Platform Support**: Works on Windows, macOS, and Linux.

## Installation

To install Windsor CLI, you need to have Go installed on your system. You can then install Windsor CLI using the following command:

```sh
go install github.com/windsorcli/cli@latest
```

## Usage

After installation, you can use the `windsor` command to interact with the CLI. Here are some common commands:

### Initialize a Project

```sh
windsor init [context]
```

This command initializes the application by setting up necessary configurations and environment.

## Configuration

Windsor CLI uses configuration files to manage settings. The configuration files are typically located in the following paths:

- **CLI Configuration**: `~/.config/windsor/config.yaml`
- **Project Configuration**: `./windsor.yaml` or `./windsor.yml`

You can customize these configurations to suit your needs.

### Example Configuration

Here is an example of a CLI configuration file:

```yaml
context: default
contexts:
  default:
    environment:
      FOO_VAR: bar
```

## Shell Integration

To automatically load Windsor CLI environment variables in your shell, you can add the following to your `precmd()` function in your shell configuration file (e.g., `.zshrc` for Zsh or `.bashrc` for Bash):

```sh
precmd() {
  if command -v windsor >/dev/null 2>&1; then
    eval "$(windsor env)"
  fi
}
```

This will ensure that the Windsor CLI environment variables are loaded every time a new shell session is started.

## [Contributing](#contributing)
We welcome contributions to Windsor CLI! If you would like to contribute, please follow these steps:

1. Fork the repository.
2. Create a new branch for your feature or bugfix.
3. Make your changes and commit them with a descriptive message.
4. Push your changes to your fork.
5. Create a pull request to the main repository.

Please ensure that your code adheres to our coding standards and includes appropriate tests.

## [License](#license)

Windsor CLI is licensed under the Mozilla Public License Version 2.0. See the [LICENSE](LICENSE) file for more details.


## [Contact Information](#contact-information)

Thank you for using Windsor CLI! If you have any questions or need further assistance, please feel free to open an issue on our GitHub repository.
