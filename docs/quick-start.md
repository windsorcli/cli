# Quick Start

## MacOS Quick Start
This guide will walk you through launching the windsor environment in your project. At the end of this guide, you should be running a local Kubernetes cluster with a single worker and controlplane.

This guide is expected to run optimally on an Apple M2 with 8 CPU cores, 8GB of RAM, and 60GB of free storage space available.

It is assumed you have installed the windsor CLI and configured `windsor hook` in your shell. Please see the [Setup and Installation](../install/install.md) page for instructions.

## Initialize a folder as a git project
Windsor expects to be running in a git project. You can either create a new folder and initialize it as a git repository or use an existing folder that is already a git project. If are creating a new folder be sure to initialize a git repository in the root of your project.

```sh
git init
```

## Install tool dependencies
You will need several tools installed on your system to successfuly run the Windsor environment. You may already have the required tools installed. To check, run:

```
windsor check
```

It is recommended to use a tool versions manager such as [aqua](https://github.com/aquaproj/aqua) or [asdf](https://github.com/asdf-vm/asdf). For your convenience, we have provided sample setup files for these tools. If your system has been configured with a tool versions manager, place one of these in the root of your project.

=== "aqua"
    Create an `aqua.yaml` file in your project's root directory with the following content:
    ```yaml
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
    - name: kubernetes/kubectl@v1.32.0
    - name: getsops/sops@v3.9.2
    - name: abiosoft/colima@v0.8.1
    - name: lima-vm/lima@v1.0.3
    - name: docker/cli@v27.4.1
    - name: docker/compose@v2.32.1
    ```

    To install the tools specified in `aqua.yaml`, run:
    ```bash
    aqua install
    ```

=== "asdf"
    Create a `.tool-versions` file in your project's root directory with the following content:
    ```plaintext
    terraform 1.10.3
    talos 1.9.1
    kubectl 1.32.0
    sops 3.9.2
    colima 0.8.1
    lima 1.0.3
    docker-cli 27.4.1
    docker-compose 2.32.1
    ```

    Additionally, ensure you add the necessary plugins for each tool before installation by executing the following commands:
    ```bash
    asdf plugin-add terraform
    asdf plugin-add talos
    asdf plugin-add kubectl
    asdf plugin-add sops
    asdf plugin-add colima
    asdf plugin-add lima
    asdf plugin-add docker-cli
    asdf plugin-add docker-compose
    ```

    To install the tools specified in `.tool-versions`, execute:
    ```bash
    asdf install
    ```

## Initialize the "local" context
The windsor tool will create a few folders in your project. In particular, it will create a folder called `contexts/` where your context configurations will reside. Initialize windsor with the docker driver by running:

```sh
windsor init local
```

Verify that the default 'local' context was selected:

```sh
windsor context get
```

### Start the environment
Start the local environment by running:

```sh
windsor up --verbose
```

This command will start appropriate docker containers, run kubernetes nodes and support services with docker compose, and bootstrap your cluster using Terraform. It can take up to 5 minutes to fully launch, so be patient!

### Verify the environment
Display the list of Kubernetes nodes:

```sh
kubectl get nodes
```

### Tear down the environment
Windsor provides a command to help you tear down your development environment. To destroy the cluster and support services, run:

```sh
windsor down --clean
```

<div>
{{ previous_footer('Installation', '../../install/install/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../install/install/index.html'; 
  });
</script>
