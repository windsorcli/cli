# Windsor CLI

Windsor CLI is a powerful command-line interface designed to streamline and enhance your development workflow. It provides a suite of tools and commands to manage your projects efficiently.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Usage](#usage)
- [Configuration](#configuration)
- [Contributing](#contributing)
- [License](#license)

## Features

- **Project Initialization**: Quickly set up new projects with predefined configurations.
- **Configuration Management**: Easily manage and switch between different project configurations.
- **Shell Integration**: Seamlessly integrates with your shell environment for enhanced productivity.
- **Cross-Platform Support**: Works on Windows, macOS, and Linux.

## Installation

To install Windsor CLI, you need to have Go installed on your system. You can then install Windsor CLI using the following command:

```sh
go install github.com/windsor-hotel/cli@latest
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
```

## Contributing

We welcome contributions to Windsor CLI! If you would like to contribute, please follow these steps:

1. Fork the repository.
2. Create a new branch for your feature or bugfix.
3. Make your changes and commit them with a descriptive message.
4. Push your changes to your fork.
5. Create a pull request to the main repository.

Please ensure that your code adheres to our coding standards and includes appropriate tests.

## License

Windsor CLI is licensed under the Mozilla Public License Version 2.0. See the [LICENSE](LICENSE) file for more details.

---

Thank you for using Windsor CLI! If you have any questions or need further assistance, please feel free to open an issue on our GitHub repository.
