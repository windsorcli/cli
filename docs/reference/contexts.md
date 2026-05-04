---
title: "Contexts"
description: "Reference for context configuration and structure"
---
# Contexts

Contexts represent a group of configuration details specific to a deployment environment in a Windsor project. Each context has its own directory at `contexts/<context-name>/` containing configuration files, credentials, and generated artifacts.

## Context Structure

A typical context folder is structured as follows:

```
contexts/
└── local/
    ├── .aws/
    │   └── config
    ├── .kube/
    │   └── config
    ├── .talos/
    │   └── config
    ├── .terraform/
    │   └── ...
    ├── .tf_state/
    │   └── ...
    ├── terraform/
    │   ├── cluster/
    │   │   └── talos.tfvars
    │   └── gitops/
    │       └── flux.tfvars
    ├── blueprint.yaml
    └── values.yaml
```

## Configuration Files

### windsor.yaml

The root `windsor.yaml` file declares the project version.

Located at: `windsor.yaml` (project root)

See [Configuration Reference](configuration.md) for details.

### values.yaml

The `values.yaml` file is the primary context-level configuration file. When `windsor init` runs, context-specific defaults are written here. Values in this file override schema defaults and are available to facets for expression evaluation.

Located at: `contexts/<context-name>/values.yaml`

The `values.yaml` file is:
- Automatically loaded and merged with the context configuration
- Validated against the blueprint's JSON Schema (if provided)
- Available to Facets for conditional logic and input evaluation
- Merged with schema defaults to provide complete configuration values

Example `values.yaml`:

```yaml
cluster:
  controlplanes:
    cpu: 6
    memory: 8
dns:
  enabled: true
  domain: test
```

### blueprint.yaml

The `blueprint.yaml` file outlines references and configuration specific to the context. See [Blueprint Reference](blueprint.md) for details.

Located at: `contexts/<context-name>/blueprint.yaml`

## Directory Structure

### `.aws/`

Contains the AWS config file for authenticating with the context's AWS API.

### `.kube/`

Contains the kubectl config file used for authenticating with the context's Kubernetes API.

### `.talos/`

Contains the talosctl config file for authenticating with the context's Talos API endpoint.

### `.terraform/`

Contains files typically used by the Terraform CLI such as modules and providers. Additionally, the `TF_DATA_DIR` resides here, along with terraform plans and state metadata files.

### `.tf_state/`

Used as the local file Terraform backend state. This is the default state until a proper remote state has been configured, or while working in a local development environment.

### `terraform/`

Contains terraform variables as `.tfvars` files. These are automatically passed to corresponding terraform projects deployed in the current context. These are explicitly referenced in the `blueprint.yaml` file. See the [Terraform Guide](../guides/terraform.md) for more details.

## Context Management

### Creating Contexts

Create new contexts by running:

```bash
windsor init <context-name>
```

This creates:
- A new folder at `contexts/<context-name>/`
- A `values.yaml` with context defaults (overrides that differ from schema/facet defaults)
- Adds a new entry to your project's root `windsor.yaml` at `contexts.<context-name>`

**Note:** Contexts named `local` or that begin with `local-` assume that you will be running a local cloud virtualization, setting defaults accordingly.

### Switching Contexts

Switch contexts by running:

```bash
windsor context set <context-name>
```

View the current context:

```bash
windsor context get
```

The current context is also available via the `WINDSOR_CONTEXT` environment variable.
