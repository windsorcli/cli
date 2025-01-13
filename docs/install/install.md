# Setup and Installation

This document describes how to install the windsor CLI on your development workstation.

## Downloading the Binary

To download the Windsor CLI binary, navigate to the [Releases](https://github.com/tvangundy/cli/releases) page of the repository. Select the version you wish to download and choose the appropriate binary for your operating system and architecture:

- **Linux**: `windsorcli_<version>_linux_amd64.tar.gz` or `windsorcli_<version>_linux_arm64.tar.gz`
- **macOS (Darwin)**: `windsorcli_<version>_darwin_amd64.tar.gz` or `windsorcli_<version>_darwin_arm64.tar.gz`
- **Windows**: `windsorcli_<version>_windows_amd64.zip` or `windsorcli_<version>_windows_arm64.zip`

Replace `<version>` with the specific version number you are downloading.

## Verifying the Signature

Each release includes a checksum file that is signed. To verify the signature, you will need to have GPG installed on your system.

1. **Import the Public Key**: If you haven't already, import the public key used to sign the checksums.
   ```bash
   gpg --keyserver hkp://keyserver.ubuntu.com --recv-keys <public-key-id>
   ```

2. **Verify the Signature**: Download the `.asc` file associated with the checksum file and verify it using:
   ```bash
   gpg --verify <checksum-file>.asc <checksum-file>
   ```

   Replace `<checksum-file>` with the actual checksum file name.

## Checking the Checksums

After downloading the binary, you should verify its integrity by checking its checksum.

1. **Download the Checksum File**: Download the checksum file from the release page, which is typically named `checksums.txt`.

2. **Verify the Checksum**: Use the following command to verify the checksum of the downloaded binary:
   - **Linux/macOS**:
     ```bash
     sha256sum -c checksums.txt
     ```
   - **Windows**: Use a tool like `CertUtil` to verify the checksum:
     ```cmd
     CertUtil -hashfile <binary-file> SHA256
     ```

   Ensure that the output indicates that the checksum is correct.

## Example

Here is an example of how you might download and verify a binary for Linux on an `amd64` architecture:

```bash
# Download the binary
wget https://github.com/windsorcli/cli/releases/download/v<version>/windsorcli_<version>_linux_amd64.tar.gz

# Download the checksum file
wget https://github.com/windsorcli/cli/releases/download/v<version>/checksums.txt

# Download the signature file
wget https://github.com/windsorcli/cli/releases/download/v<version>/checksums.txt.asc

# Verify the signature
gpg --verify checksums.txt.asc checksums.txt

# Check the checksum
sha256sum -c checksums.txt
```

Replace `<version>` with the specific version number you are downloading.


## Shell Integration: Seamless Environment Management

Windsor acts as an environment variable manager in your shell. It dynamically injects environment variables into your shell as you switch contexts and work on various components in your project.

You can add the `windsor hook` to various shells as follows:

=== "BASH"
    Add the following line at the end of the `~/.bashrc` file:
    ```sh
    eval "$(windsor hook bash)"
    ```
    Make sure it appears even after rvm, git-prompt, and other shell extensions that manipulate the prompt.

=== "ZSH"
    Add the following line at the end of the `~/.zshrc` file:
    ```sh
    eval "$(windsor hook zsh)"
    ```

=== "FISH"
    Add the following line to your `config.fish` file:
    ```fish
    eval (windsor hook fish)
    ```

=== "TCSH"
    Add the following line to your `~/.tcshrc` file:
    ```tcsh
    eval `windsor hook tcsh`
    ```

=== "ELVISH"
    Add the following line to your `rc.elv` file:
    ```elvish
    eval (windsor hook elvish)
    ```

=== "POWERSHELL"
    Add the following line to your PowerShell profile script:
    ```powershell
    Invoke-Expression (& windsor hook powershell)
    ```

## Version Check

To verify the installation and check the version of the Windsor CLI, execute the following command:

```bash
windsor version
```

<div>
  {{ footer('Home', '../../index.html', 'Quick Start', '../../tutorial/macos-quick-start/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../tutorial/macos-quick-start/index.html'; 
  });
</script>
