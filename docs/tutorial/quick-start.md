# Quick Start Guide

This guide will help you create a windsor project folder and deploy your first windsor cluster.

## Step-by-Step Instructions

### Open your terminal.

### Make the windsor project directory.

```
mkdir windsor-project
```

### Navigate to the directory where you want to create the top level windsor project.

```
cd windsor-project
```

### Create the `aqua.yaml` file using the `cat` command.

   Execute the following command to create the file and input the necessary content:

```
cat > aqua.yaml << 'EOF'
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
    ref: v4.285.0 # renovate: depName=aquaproj/aqua-registry
packages:
- name: hashicorp/terraform@v1.10.3
- name: siderolabs/talos@v1.9.1
- name: siderolabs/omni/omnictl@v0.45.1
- name: siderolabs/omni/omni@v0.45.1
- name: kubernetes/kubectl@v1.32.0
- name: go-task/task@v3.40.1
- name: golang/go@go1.23.4
- name: getsops/sops@v3.9.2
- name: abiosoft/colima@v0.8.1
- name: lima-vm/lima@v1.0.3
- name: docker/cli@v27.4.1
- name: securego/gosec@v2.21.4
- name: docker/compose@v2.32.1
- name: google/go-jsonnet@v0.20.0
- name: mikefarah/yq@v4.44.6
EOF
```

### Verify the file creation.

You can check the contents of the `aqua.yaml` file by using:

```bash
cat aqua.yaml
```

This should display the content you just entered.

### Install Dependencies

Run the following command to install the dependencies specified in the `aqua.yaml` file:

```bash
aqua install
```

### Initialize Git Repository

If the target folder is not already a git repository, initialize it:

```bash
git init
```

### Initialize the Environment

Set up the necessary configurations and name the context:

```sh
windsor init local --vm-driver colima
```

### Confirm Context

Verify that the default 'local' context was selected:

```sh
windsor context get
```

### Start Talos Kubernetes Cluster

Build the virtual environment and start the Talos operating system:

```sh
windsor up
```

### List Kubernetes Nodes

Display the list of Kubernetes nodes:

```sh
kubectl get nodes
```

<div>
{{ previous_footer('Installation', '../../install/install/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../install/install/index.html'; 
  });
</script>
