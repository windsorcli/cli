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

## [Quick Start](#quick-start)

- **[Setup and Installation](./docs/install/install.md)**
- **[Quick Start](./docs/tutorial/quick-start.md)**


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



# Creating a GPG Private Key and Fingerprint

## Step 1: Install GPG

If you don't have GPG installed, you can install it using the following commands:

- **On macOS:** Use Homebrew
  ```bash
  brew install gnupg
  ```

- **On Linux:** Use your package manager
  ```bash
  sudo apt-get install gnupg
  ```

- **On Windows:** Download and install from [GnuPG's official site](https://gnupg.org/download/).

## Step 2: Generate a GPG Key

1. **Open a Terminal:**
   - On Windows, you can use Git Bash or any terminal emulator.

2. **Generate the Key:**
   ```bash
   gpg --full-generate-key
   ```

3. **Follow the Prompts:**
   - **Key Type:** Choose the default (RSA and RSA).
   - **Key Size:** 4096 bits is recommended for strong security.
   - **Key Expiration:** Choose a suitable expiration period or select "0" for no expiration.
   - **User ID Information:** Enter your name, email, and an optional comment.
   - **Passphrase:** Choose a strong passphrase to protect your key.

## Step 3: Retrieve the GPG Fingerprint

After generating the key, you can list your keys and find the fingerprint:

```bash
gpg --list-keys
```

This will display a list of keys. Look for the key you just created, and you will see a line that starts with `pub` followed by the key ID and the fingerprint.

## Step 4: Export the GPG Private Key

To export your private key, use the following command:

```bash
gpg --armor --export-secret-keys your-email@example.com > private-key.asc
```

Replace `your-email@example.com` with the email address you used when creating the key. This will create a file named `private-key.asc` containing your private key in ASCII format.

## Step 5: Use the Key and Fingerprint in GitHub Actions

- **GPG Private Key:** Open `private-key.asc` in a text editor and copy its contents. Add this as the `GPG_PRIVATE_KEY` secret in your GitHub repository.
- **GPG Fingerprint:** Copy the fingerprint from the `gpg --list-keys` output and add it as the `GPG_FINGERPRINT` secret in your GitHub repository.

By following these steps, you'll have a GPG private key and fingerprint ready to use in your GitHub Actions workflows.

# Brew Tap


```
brew tap tvangundy/cli
brew install windsor
```