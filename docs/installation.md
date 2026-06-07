---
title: Installation
description: Install the Windsor CLI on macOS, Windows, or Linux.
---

<!--
  Rendered by windsorcli.github.io with a custom OS-tabbed layout. Per-OS
  sections are delimited by HTML comments of the form `os:NAME` (macos /
  windows / linux); everything before the first one is shared preamble. The
  tokens VERSION and RELEASES (each in double curly braces) are substituted by
  the site with the pinned CLI version and the GitHub releases URL.
-->

This page covers installing the Windsor CLI binary and verifying it runs. Choose your OS below for package manager, install script, and manual options.

<!-- os:macos -->

### Homebrew

```sh
brew update
brew tap windsorcli/cli
brew install windsor
```

### Install script

```sh
curl -fsSL https://windsorcli.dev/install.sh | sh
```

Restart your shell or run `source ~/.zshrc` (or `~/.bashrc`) so `windsor` is on your PATH.

### Manual (ARM64)

```sh
curl -L -o windsor_{{VERSION}}_darwin_arm64.tar.gz {{RELEASES}}/download/v{{VERSION}}/windsor_{{VERSION}}_darwin_arm64.tar.gz
tar -xzf windsor_{{VERSION}}_darwin_arm64.tar.gz -C /usr/local/bin
chmod +x /usr/local/bin/windsor
```

### Manual (AMD64)

```sh
curl -L -o windsor_{{VERSION}}_darwin_amd64.tar.gz {{RELEASES}}/download/v{{VERSION}}/windsor_{{VERSION}}_darwin_amd64.tar.gz
tar -xzf windsor_{{VERSION}}_darwin_amd64.tar.gz -C /usr/local/bin
chmod +x /usr/local/bin/windsor
```

## Verify

```sh
windsor version
```

**Shell hook (optional).** You don't need this to install blueprints or run `windsor bootstrap`, and `windsor exec -- <command>` runs any one-off command with the right environment. It's mainly for developing Terraform on top of a blueprint: add `eval "$(windsor hook bash)"` (or `zsh`) to your shell profile so your context's variables — `KUBECONFIG`, cloud profile, Talos config — stay current on every prompt. See [Environment injection](/contexts/environment-injection).

<!-- os:windows -->

### Chocolatey

Run in PowerShell as Administrator:

```powershell
choco install windsor
```

### Install script (PowerShell)

```powershell
irm https://windsorcli.dev/install.ps1 | iex
```

### Manual (PowerShell as Administrator)

```powershell
$installDir = "C:\Program Files\Windsor"
New-Item -Path $installDir -ItemType Directory -Force
Invoke-WebRequest -Uri "{{RELEASES}}/download/v{{VERSION}}/windsor_{{VERSION}}_windows_amd64.zip" -Headers @{"Accept"="application/octet-stream"} -OutFile "windsor_{{VERSION}}_windows_amd64.zip"
Expand-Archive -Path "windsor_{{VERSION}}_windows_amd64.zip" -DestinationPath $installDir -Force
$currentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
if ($currentPath -notlike "*$installDir*") {
  [Environment]::SetEnvironmentVariable("Path", "$currentPath;$installDir", "Machine")
  $env:Path += ";$installDir"
}
```

## Verify

```powershell
windsor version
```

**Shell hook (optional).** You don't need this to install blueprints or run `windsor bootstrap`, and `windsor exec -- <command>` runs any one-off command with the right environment. It's mainly for developing Terraform on top of a blueprint: add `Invoke-Expression (& windsor hook powershell)` to your PowerShell profile so your context's variables stay current on every prompt. See [Environment injection](/contexts/environment-injection).

<!-- os:linux -->

### Install script

```sh
curl -fsSL https://windsorcli.dev/install.sh | sh
```

Restart your shell or run `source ~/.bashrc` (or your profile) so `windsor` is on your PATH.

### Manual (ARM64)

```sh
curl -L -o windsor_{{VERSION}}_linux_arm64.tar.gz {{RELEASES}}/download/v{{VERSION}}/windsor_{{VERSION}}_linux_arm64.tar.gz
sudo tar -xzf windsor_{{VERSION}}_linux_arm64.tar.gz -C /usr/local/bin
sudo chmod +x /usr/local/bin/windsor
```

### Manual (AMD64)

```sh
curl -L -o windsor_{{VERSION}}_linux_amd64.tar.gz {{RELEASES}}/download/v{{VERSION}}/windsor_{{VERSION}}_linux_amd64.tar.gz
sudo tar -xzf windsor_{{VERSION}}_linux_amd64.tar.gz -C /usr/local/bin
sudo chmod +x /usr/local/bin/windsor
```

## Verify

```sh
windsor version
```

**Shell hook (optional).** You don't need this to install blueprints or run `windsor bootstrap`, and `windsor exec -- <command>` runs any one-off command with the right environment. It's mainly for developing Terraform on top of a blueprint: add `eval "$(windsor hook bash)"` (or `zsh`) to your shell profile so your context's variables — `KUBECONFIG`, cloud profile, Talos config — stay current on every prompt. See [Environment injection](/contexts/environment-injection).
