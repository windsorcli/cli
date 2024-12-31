# Setup and Installation

This document describes how to install the windsor CLI on your development workstation.

## Download and Install The Windsor Binary

=== "MacOS"
    ```bash
    curl -L -o /usr/local/bin/windsor https://github.com/windsorcli/cli/releases/download/v0.2.1/windsor-darwin-arm64
    chmod +x /usr/local/bin/windsor
    ```

=== "Windows"
    ```powershell
    Invoke-WebRequest -Uri "https://github.com/windsorcli/cli/releases/download/v0.2.1/windsor-windows-amd64.exe" -OutFile "C:\Program Files\Windsor\windsor.exe"
    ```

=== "Linux"
    ```bash
    curl -L -o /usr/local/bin/windsor https://github.com/windsorcli/cli/releases/download/v0.2.1/windsor-linux-amd64
    chmod +x /usr/local/bin/windsor
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
