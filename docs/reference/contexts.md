# Contexts

In a Windsor project, you work across different deployments or environments as "contexts". You could think of a context in terms of your software development lifecycle (SDLC) environments, such as `development`, `staging`, and `production`. You could also think of a context in terms of different pieces of your organizational infrastructure -- `admin`, `web`, or `observability`. Or, some combination of schemes. You may have a `web-staging` and `web-production` as well as `observability-staging` and `observability-production`.

You create your Windsor project by running `windsor init`. When doing so, it automatically creates a `local` context. This step results in the following:

- Creates a new folder of assets in `contexts/local`
- Adds a new entry to your project's `windsor.yaml` file at `contexts.local`

Note: Not all context names are handled in the same manner. Contexts named `local` or that begin with `local-` assume that you will be running a local cloud virtualization, setting defaults accordingly. You can read more in the documentation on the [local workstation](guides/local-workstation.md)

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

### `.aws/` folder
Contains the aws config file for authenticating with the context's AWS API.

### `.kube/` folder
Contains the kubectl config file used for authenticating with the context's Kubernetes API.

### `.talos/` folder
Contains the talosctl config file for authenticating with the context's Talos API endpoint.

### `.terraform/` folder
Contains files typically used by the Terraform CLI such as modules and providers. Additionally, the `TF_DATA_DIR` resides here, along with terraform plans and state metadata files.

### `terraform/` folder
Contains terraform variables as `.tfvars` files. These are automatically passed to corresponding terraform projects deployed in the current context. These are explicitly referenced in the `blueprint.yaml` file. Please refer to the [Terraform](terraform.md) reference for more details.

### `.tf_state/` folder
Used as the local file Terraform backend state. This is the default state until a proper remote state has been configured, or while working in a local development environment.

### `blueprint.yaml`
The `blueprint.yaml` file outlines references and configuration specific to the context. Please refer to the [blueprint](blueprint.md) documentation for more details.
