# Installation

This document describes how to install the Windsor CLI on your development workstation as well as configuring `windsor hook` in your shell.

## Installing with Brew

```
brew update
brew tap windsorcli/cli
brew install windsor
```

## Manual Installation

=== "MacOS"
    ```bash
    curl -L -o windsor_0.3.0_darwin_arm64.tar.gz https://github.com/windsorcli/cli/releases/download/v0.3.0/windsor_0.3.0_darwin_arm64.tar.gz && \
    tar -xzf windsor_0.3.0_darwin_arm64.tar.gz -C /usr/local/bin && \
    chmod +x /usr/local/bin/windsor
    ```

=== "Windows"
    ```powershell
    Invoke-WebRequest -Uri "https://github.com/windsorcli/cli/releases/download/v0.3.0/windsor_0.3.0_windows_amd64.tar.gz" -OutFile "windsor_0.3.0_windows_amd64.tar.gz" ; \
    tar -xzf windsor_0.3.0_windows_amd64.tar.gz -C "C:\Program Files\Windsor" ; \
    Rename-Item "C:\Program Files\Windsor\windsor.exe" -NewName "windsor.exe"
    ```

=== "Linux"
    ```bash
    curl -L -o windsor_0.3.0_linux_amd64.tar.gz https://github.com/windsorcli/cli/releases/download/v0.3.0/windsor_0.3.0_linux_amd64.tar.gz && \
    tar -xzf windsor_0.3.0_linux_amd64.tar.gz -C /usr/local/bin && \
    chmod +x /usr/local/bin/windsor
    ```

To enhance security and confirm the integrity of your Windsor CLI installation, it is crucial to verify the downloaded binary. This involves checking the signature and checksum of the binary to ensure it has not been tampered with and is safe for use on your system. Follow the steps below to perform these verifications.

=== "macOS"
    1. **Import the Public Key**
    ```bash
    gpg --keyserver keys.openpgp.org --recv-keys <public-key-id>
    ```

    2. **Download the signature file**:
    ```bash
    curl -L -o windsor_0.3.0_checksums.txt.sig https://github.com/windsorcli/cli/releases/download/v0.3.0/windsor_0.3.0_checksums.txt.sig
    ```

    3. **Download the checksum file**:
    ```bash
    curl -L -o windsor_0.3.0_checksums.txt https://github.com/windsorcli/cli/releases/download/v0.3.0/windsor_0.3.0_checksums.txt
    ```

    4. **Verify the Signature**:
    ```bash
    gpg --verify windsor_0.3.0_checksums.txt.sig windsor_0.3.0_checksums.txt
    ```

    5. **Verify the Checksum**:
    ```bash
    shasum -a 256 -c windsor_0.3.0_checksums.txt
    ```

=== "Windows"
    1. **Import the Public Key**
    ```powershell
    gpg --keyserver keys.openpgp.org --recv-keys <public-key-id>
    ```

    2. **Download the signature file**:
    ```powershell
    Invoke-WebRequest -Uri "https://github.com/windsorcli/cli/releases/download/v0.3.0/windsor_0.3.0_checksums.txt.sig" -OutFile "windsor_0.3.0_checksums.txt.sig"
    ```

    3. **Download the checksum file**:
    ```powershell
    Invoke-WebRequest -Uri "https://github.com/windsorcli/cli/releases/download/v0.3.0/windsor_0.3.0_checksums.txt" -OutFile "windsor_0.3.0_checksums.txt"
    ```

    4. **Verify the Signature**:
    ```powershell
    gpg --verify windsor_0.3.0_checksums.txt.sig windsor_0.3.0_checksums.txt
    ```

    5. **Verify the Checksum**:
    ```powershell
    Get-FileHash -Algorithm SHA256 -Path "windsor_0.3.0_checksums.txt" | Format-List
    ```

=== "Linux"
    1. **Import the Public Key**
    ```bash
    gpg --keyserver keys.openpgp.org --recv-keys <public-key-id>
    ```

    2. **Download the signature file**:
    ```bash
    wget -O windsor_0.3.0_checksums.txt.sig https://github.com/windsorcli/cli/releases/download/v0.3.0/windsor_0.3.0_checksums.txt.sig
    ```

    3. **Download the checksum file**:
    ```bash
    wget -O windsor_0.3.0_checksums.txt https://github.com/windsorcli/cli/releases/download/v0.3.0/windsor_0.3.0_checksums.txt
    ```

    4. **Verify the Signature**:
    ```bash
    gpg --verify windsor_0.3.0_checksums.txt.sig windsor_0.3.0_checksums.txt
    ```

    5. **Verify the Checksum**:
    ```bash
    sha256sum -c windsor_0.3.0_checksums.txt
    ```

## Version Check

To verify the installation and check the version of the Windsor CLI, execute the following command:

```bash
windsor version
```

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

<div>
  {{ footer('Home', '../index.html', 'Quick Start', '../quick-start/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../quick-start/index.html'; 
  });
</script>
