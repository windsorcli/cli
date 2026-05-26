---
title: "Installation"
description: "This document describes how to install the Windsor CLI on your development workstation as well as configuring `windsor hook` in your shell."
---
# Installation

This document describes how to install the Windsor CLI on your development workstation as well as configuring `windsor hook` in your shell.

## Installing with Package Managers

=== "Homebrew"
    ```
    brew update
    brew tap windsorcli/cli
    brew install windsor
    ```

=== "Chocolatey"
    Execute the following command in PowerShell with administrative privileges:
    ```
    choco install windsor
    ```

## Manual Installation

=== "MacOS (ARM64)"
    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_darwin_arm64.tar.gz https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_darwin_arm64.tar.gz && \
    tar -xzf windsor_{{ config.extra.release_version }}_darwin_arm64.tar.gz -C /usr/local/bin && \
    chmod +x /usr/local/bin/windsor
    ```

    <details>
    <summary><strong>Verify the signature and checksum of the Windsor binary</strong></summary>

    To enhance security and confirm the integrity of your Windsor CLI installation, it is crucial to verify the downloaded binary. This involves checking the signature and checksum of the binary to ensure it has not been tampered with and is safe for use on your system. Follow the steps below to perform these verifications.

    1. **Import the Public Key**
    ```bash
    gpg --keyserver keys.openpgp.org --recv-keys {{ config.extra.public_key_id }}
    ```    
    
    2. **Download the binary signature file**:
    ```bash
    curl -L -o windsor_darwin_arm64.sig https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_darwin_arm64.sig
    ```

    3. **Verify the binary signature**:
    ```bash
    gpg --verify windsor_darwin_arm64.sig /usr/local/bin/windsor
    ```

    4. **Download the checksum signature file**:
    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_checksums.txt.sig https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_checksums.txt.sig

    ```
    5. **Download the checksum file**:
    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_checksums.txt https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    6. **Verify the checksums signature**:
    ```bash
    gpg --verify windsor_{{ config.extra.release_version }}_checksums.txt.sig windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    7. **Verify the checksum**:
    ```bash
    shasum -a 256 -c windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    </details>

=== "MacOS (AMD64)"
    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_darwin_amd64.tar.gz https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_darwin_amd64.tar.gz && \
    tar -xzf windsor_{{ config.extra.release_version }}_darwin_amd64.tar.gz -C /usr/local/bin && \
    chmod +x /usr/local/bin/windsor
    ```

    <details>
    <summary><strong>Verify the signature and checksum of the Windsor binary</strong></summary>

    To enhance security and confirm the integrity of your Windsor CLI installation, it is crucial to verify the downloaded binary. This involves checking the signature and checksum of the binary to ensure it has not been tampered with and is safe for use on your system. Follow the steps below to perform these verifications.

    1. **Import the Public Key**
    ```bash
    gpg --keyserver keys.openpgp.org --recv-keys {{ config.extra.public_key_id }}
    ```    
    
    2. **Download the binary signature file**:
    ```bash
    curl -L -o windsor_darwin_amd64.sig https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_darwin_amd64.sig
    ```

    3. **Verify the binary signature**:
    ```bash
    gpg --verify windsor_darwin_amd64.sig /usr/local/bin/windsor
    ```

    4. **Download the checksum signature file**:
    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_checksums.txt.sig https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_checksums.txt.sig
    ```

    5. **Download the checksum file**:
    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_checksums.txt https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    6. **Verify the checksums signature**:
    ```bash
    gpg --verify windsor_{{ config.extra.release_version }}_checksums.txt.sig windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    7. **Verify the checksum**:
    ```bash
    shasum -a 256 -c windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    </details>

=== "Windows"
    Execute the following commands in PowerShell with administrative privileges:

    ```powershell
    $installDir = "C:\Program Files\Windsor"
    New-Item -Path $installDir -ItemType Directory -Force
    Invoke-WebRequest -Uri "https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_windows_amd64.zip" -Headers @{"Accept"="application/octet-stream"} -OutFile "windsor_{{ config.extra.release_version }}_windows_amd64.zip"
    Expand-Archive -Path "windsor_{{ config.extra.release_version }}_windows_amd64.zip" -DestinationPath $installDir -Force
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    if ($currentPath -notlike "*$installDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$currentPath;$installDir", "Machine")
        $env:Path += ";$installDir"
    }
    ```

    <details>
    <summary><strong>Verify the signature and checksum of the Windsor binary</strong></summary>

    To enhance security and confirm the integrity of your Windsor CLI installation, it is crucial to verify the downloaded binary. This involves checking the signature and checksum of the binary to ensure it has not been tampered with and is safe for use on your system. Follow the steps below to perform these verifications.

    1. **Import the Public Key**
    ```powershell
    gpg --keyserver keys.openpgp.org --recv-keys {{ config.extra.public_key_id }}
    ```

    2. **Download the binary signature file**:
    ```powershell
    Invoke-WebRequest -Uri "https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor.exe_windows_amd64.sig" -OutFile "windsor.exe_windows_amd64.sig"
    ```

    3. **Verify the binary signature**:
    ```powershell
    gpg --verify windsor.exe_windows_amd64.sig "C:\Program Files\Windsor\windsor.exe"
    ```

    4. **Download the checksum signature file**:
    ```powershell
    Invoke-WebRequest -Uri "https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_checksums.txt.sig" -OutFile "windsor_{{ config.extra.release_version }}_checksums.txt.sig"
    ```

    5. **Download the checksum file**:
    ```powershell
    Invoke-WebRequest -Uri "https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_checksums.txt" -OutFile "windsor_{{ config.extra.release_version }}_checksums.txt"
    ```

    6. **Verify the checksums signature**:
    ```powershell
    gpg --verify windsor_{{ config.extra.release_version }}_checksums.txt.sig windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    7. **Verify the checksum**:
    ```powershell
    $expectedHash = (Select-String -Path "windsor_{{ config.extra.release_version }}_checksums.txt" -Pattern "windsor_{{ config.extra.release_version }}_windows_amd64.zip").Line.Split()[0]
    $actualHash = (Get-FileHash -Algorithm SHA256 -Path "windsor_{{ config.extra.release_version }}_windows_amd64.zip").Hash.ToLower()
    if ($expectedHash -eq $actualHash) {
        Write-Host "Checksum verified successfully"
    } else {
        Write-Host "Checksum verification failed" -ForegroundColor Red
    }
    ```
    </details>

=== "Linux (ARM64)"

    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_linux_arm64.tar.gz https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_linux_arm64.tar.gz && \
    sudo tar -xzf windsor_{{ config.extra.release_version }}_linux_arm64.tar.gz -C /usr/local/bin && \
    sudo chmod +x /usr/local/bin/windsor
    ```

    <details>
    <summary><strong>Verify the signature and checksum of the Windsor binary</strong></summary>

    To enhance security and confirm the integrity of your Windsor CLI installation, it is crucial to verify the downloaded binary. This involves checking the signature and checksum of the binary to ensure it has not been tampered with and is safe for use on your system. Follow the steps below to perform these verifications.

    1. **Import the Public Key**
    ```bash
    gpg --keyserver keys.openpgp.org --recv-keys {{ config.extra.public_key_id }}
    ```
    2. **Download the binary signature file**:
    ```bash
    curl -L -o windsor_linux_arm64.sig https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_linux_arm64.sig
    ```

    3. **Verify the binary signature**:
    ```bash
    gpg --verify windsor_linux_arm64.sig /usr/local/bin/windsor
    ```

    4. **Download the checksum signature file**:
    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_checksums.txt.sig https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_checksums.txt.sig
    ```

    5. **Download the checksum file**:
    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_checksums.txt https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    6. **Verify the checksums signature**:
    ```bash
    gpg --verify windsor_{{ config.extra.release_version }}_checksums.txt.sig windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    7. **Verify the checksums**:
    ```bash
    sha256sum -c windsor_{{ config.extra.release_version }}_checksums.txt 
    ```

    </details>

=== "Linux (AMD64)"

    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_linux_amd64.tar.gz https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_linux_amd64.tar.gz && \
    sudo tar -xzf windsor_{{ config.extra.release_version }}_linux_amd64.tar.gz -C /usr/local/bin && \
    sudo chmod +x /usr/local/bin/windsor
    ```

    <details>
    <summary><strong>Verify the signature and checksum of the Windsor binary</strong></summary>

    To enhance security and confirm the integrity of your Windsor CLI installation, it is crucial to verify the downloaded binary. This involves checking the signature and checksum of the binary to ensure it has not been tampered with and is safe for use on your system. Follow the steps below to perform these verifications.

    1. **Import the Public Key**
    ```bash
    gpg --keyserver keys.openpgp.org --recv-keys {{ config.extra.public_key_id }}
    ```
    2. **Download the binary signature file**:
    ```bash
    curl -L -o windsor_linux_amd64.sig https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_linux_amd64.sig
    ```

    3. **Verify the binary signature**:
    ```bash
    gpg --verify windsor_linux_amd64.sig /usr/local/bin/windsor
    ```

    4. **Download the checksum signature file**:
    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_checksums.txt.sig https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_checksums.txt.sig
    ```

    5. **Download the checksum file**:
    ```bash
    curl -L -o windsor_{{ config.extra.release_version }}_checksums.txt https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    6. **Verify the checksums signature**:
    ```bash
    gpg --verify windsor_{{ config.extra.release_version }}_checksums.txt.sig windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    7. **Verify the checksums**:
    ```bash
    sha256sum -c windsor_{{ config.extra.release_version }}_checksums.txt
    ```

    </details>

## Supply Chain Verification

Every Windsor release ships with two artifacts beyond the binary and GPG signature: a CycloneDX SBOM per archive, and a SLSA v1.0 build-provenance attestation covering every release artifact. These let supply-chain reviewers verify both *what* is in the release (the SBOM) and *how* it was built (the attestation, signed via GitHub's OIDC-backed Sigstore root). The per-platform GPG checks above cover binary tamper detection; the steps in this section cover provenance and dependency review.

### SLSA Build Provenance

Each release archive, checksums file, and SBOM is covered by a SLSA v1.0 build-provenance attestation signed via Sigstore and published to GitHub's attestation API. Verify any downloaded artifact with [`gh attestation verify`](https://cli.github.com/manual/gh_attestation_verify) (`gh` ≥ 2.49.0):

```bash
gh attestation verify windsor_{{ config.extra.release_version }}_darwin_arm64.tar.gz --repo windsorcli/cli
```

The command fetches the attestation from `github.com/windsorcli/cli/attestations`, confirms the artifact's hash matches the signed claim, and verifies the signer was the Windsor CLI release workflow running on a GitHub-hosted runner. A successful run prints `✓ Verification succeeded!` and confirms the artifact was produced by the published release pipeline rather than a side-channel build.

### CycloneDX SBOM

Each release archive is accompanied by a sibling `.sbom.cdx.json` file with the full dependency tree in CycloneDX 1.5 format. Download it from the release page alongside the archive:

```bash
curl -L -o windsor_{{ config.extra.release_version }}_darwin_arm64.tar.gz.sbom.cdx.json \
  https://github.com/windsorcli/cli/releases/download/v{{ config.extra.release_version }}/windsor_{{ config.extra.release_version }}_darwin_arm64.tar.gz.sbom.cdx.json
```

Common consumers:

- **[`grype`](https://github.com/anchore/grype)** — scan the SBOM for known CVEs: `grype sbom:windsor_{{ config.extra.release_version }}_darwin_arm64.tar.gz.sbom.cdx.json`
- **[`cyclonedx-cli`](https://github.com/CycloneDX/cyclonedx-cli)** — convert to SPDX, diff against a previous release, or check against a policy file
- **Manual inspection** — `jq '.components[] | {name, version}' windsor_*.sbom.cdx.json` lists every dependency with its pinned version

The SBOM file is covered by the same SLSA attestation as the archive, so a single `gh attestation verify` run confirms provenance for both.

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
