# Windsor CLI Setup and Installation

This document describes how to setup and initialize a new blueprint on MacOS.  

## Prerequisites

Ensure you have the following prerequisites installed:

### Install Windsor on MacOS

To install Windsor, use the following commands:

```bash
curl -L -o /usr/local/bin/windsor https://github.com/windsorcli/cli/releases/download/v0.2.0/windsor-darwin-arm64
```
```bash
chmod +x /usr/local/bin/windsor
```

### Install Aqua

Aqua is a tool for managing multiple versions of executables. Install it from [Aqua's official documentation](https://aquaproj.github.io/docs/install).


### Configure Aqua YAML

Run the following command to create a aqua.yaml that specifies install the necessary tools for the CLI:

```bash
cat << 'EOF' > aqua.yaml
---
# yaml-language-server: $schema=https://raw.githubusercontent.com/aquaproj/aqua/main/json-schema/aqua-yaml.json
# aqua - Declarative CLI Version Manager
# https://aquaproj.github.io/
# checksum:
#   enabled: true
#   require_checksum: true
#   supported_envs:
#   - all
registries:
  - type: standard
    ref: v4.284.1 # renovate: depName=aquaproj/aqua-registry
packages:
  - name: hashicorp/terraform@v1.10.3
  - name: siderolabs/talos@v1.9.0
  - name: siderolabs/omni/omnictl@v0.45.1
  - name: siderolabs/omni/omni@v0.45.1
  - name: kubernetes/kubectl@v1.32.0
  - name: go-task/task@v3.40.1
  - name: golang/go@go1.23.4
  - name: abiosoft/colima@v0.8.1
  - name: lima-vm/lima@v1.0.2
  - name: docker/cli@v27.4.1
  - name: docker/compose@v2.32.1
  - name: aws/aws-cli@2.22.23
  - name: helm/helm@v3.16.4
  - name: fluxcd/flux2@v2.4.0
  - name: hashicorp/vault@v1.18.3
  - name: derailed/k9s@v0.32.7
  - name: getsops/sops@v3.9.2
```

### Install Dependencies

Execute the aqua install command in the same folder as the aqua.yaml file:

```bash
aqua install
```

This command ensures that all tools are installed in their specified versions, thereby maintaining a consistent development environment.


## Shell Integration: Seamless Environment Management

To enable the automatic loading of environment variables with the Windsor CLI, incorporate the following `precmd()` function into your shell configuration file (e.g., `.zshrc` for Zsh or `.bashrc` for Bash):

```bash
precmd() {
  if command -v windsor >/dev/null 2>&1; then
    eval "$(windsor env)"
  fi
}
```

This function loads the environment variables contextually.

## Version Check

To verify the installation and check the version of the Windsor CLI, execute the following command:

```bash
windsor version
```

<div>
  {{ footer('Home', '../../index.html', 'Quick Start', '../../tutorial/quick-start/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../tutorial/quick-start/index.html'; 
  });
</script>
