# Contexts
Contexts represent a group of configuration details specific to a deployment environment in a Windsor project.

## Working with contexts via the cli

You can create new contexts by running:

```
windsor init <new-context>
```

You may then switch contexts by running:

```
windsor context set <context-name>
```

You can see your current context by running:

```
windsor context get
```

The current context is also available via the `WINDSOR_CONTEXT` environment variable.

## Context folder contents

The `contexts/` folder contains subfolders for each context. For example, files related to configuring the local context can be found at `contexts/local`. A typical context folder may be structured as follows:

```
contexts/
├── local/
│   ├── .aws/
│   │   └── config
│   ├── .kube/
│   │   └── config
│   ├── .talos/
│   │   └── config
│   │   .terraform/
│   │   └──...
│   │   .tf_state/
│   │   └──...
│   ├── terraform/
│   │   ├── cluster/
│   │   │   └── talos.tfvars
│   │   └── gitops/
│   │       └── flux.tfvars
│   └── blueprint.yaml
└── <new-context>/
    ├── .aws/
    │   └── config
    ├── .kube/
    │   └── config
    ├── .talos/
    │   └── config
    │   .terraform/
    │   └──...
    │   .tf_state/
    │   └──...
    ├── terraform/
    │   ├── cluster/
    │   │   └── talos.tfvars
    │   └── gitops/
    │       └── flux.tfvars
    └── blueprint.yaml
```

### `.aws/`
Contains the aws config file for authenticating with the context's AWS API.

### `.kube/`
Contains the kubectl config file used for authenticating with the context's Kubernetes API.

### `.talos/`
Contains the talosctl config file for authenticating with the context's Talos API endpoint.

### `.terraform/`
Contains files typically used by the Terraform CLI such as modules and providers. Additionally, the `TF_DATA_DIR` resides here, along with terraform plans and state metadata files.

### `.tf_state/`
Used as the local file Terraform backend state. This is the default state until a proper remote state has been configured, or while working in a local development environment.

### `terraform/`
Contains terraform variables as `.tfvars` files. These are automatically passed to corresponding terraform projects deployed in the current context. These are explicitly referenced in the `blueprint.yaml` file. Please refer to the [Terraform](terraform.md) reference for more details.

### `blueprint.yaml`
The `blueprint.yaml` file outlines references and configuration specific to the context. Please refer to the [blueprint](blueprint.md) documentation for more details.
