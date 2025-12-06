---
title: "Terraform"
description: "The Windsor CLI streamlines your workflow with Terraform assets by providing a context-aware environment."
---
# Terraform
The Windsor CLI streamlines your workflow with Terraform assets by providing a context-aware environment. Once you've installed the Windsor CLI and the `windsor hook`, your setup should be automatically configured for Terraform.

## Folder Structure
To ensure compatibility with Windsor, your Terraform project should adhere to a specific folder structure. Below is a typical layout for Terraform assets in a Windsor project:

```plaintext
.windsor/
└── contexts/
    └── local/
        ├── cluster/
        │   └── talos/
        │       ├── main.tf
        │       └── variables.tf
        └── gitops/
            └── flux/
                ├── main.tf
                └── variables.tf
contexts/
└── local/
    ├── .terraform/
    │   └── ...
    ├── .tf_state/
    │   └── ...
    ├── terraform/
    │   ├── cluster/
    │   │   └── talos.tfvars
    │   ├── gitops/
    │   │   └── flux.tfvars
    │   └── database/
    │       └── postgres.tfvars
    └── blueprint.yaml
terraform/
└── database/
    └── postgres/
        ├── main.tf
        └── variables.tf
```

## Blueprint
You can simplify infrastructure development by referencing Terraform modules in the `blueprint.yaml` file. For example, the default local blueprint includes:

```yaml
kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: local
  description: This blueprint outlines resources in the local context
repository:
  url: http://git.test/git/tmp
  ref:
    branch: main
  secretName: flux-system
sources:
- name: core
  url: github.com/windsorcli/core
  ref:
    branch: main
terraform:
- source: core
  path: cluster/talos
- source: core
  path: gitops/flux
kustomize:
- name: local
  path: ""
```

Modules like `cluster/talos` and `gitops/flux` are remote, with shims in `.windsor/contexts/<context>/cluster/talos` and `.windsor/contexts/<context>/gitops/flux`. Running `windsor up` applies these modules sequentially.

Store your Terraform code in a `terraform/` folder within your project. To reference it in `blueprint.yaml`, add a section without a `source` field. For example, if your code is in `terraform/example/my-app`, add:

```yaml
terraform:
- source: core
  path: cluster/talos
- source: core
  path: gitops/flux
- path: example/my-app   # Add the path to your local Terraform module
```

Now, running `windsor up` will execute your module after the `gitops/flux` module.

## Importing Resources
The Windsor CLI offers a unique method for importing and using remote Terraform modules. Running `windsor init local` unpacks shims that reference basic modules from Windsor's [core blueprint](https://github.com/windsorcli/core), stored in `.windsor/contexts/<context>/`.

Think of the `contexts/<context>/` folder as the remote module counterpart to the local `terraform` folder. Variables for these modules are located in `.windsor/contexts/<context>/path/to/module/terraform.tfvars`.

## Terraform CLI Assistance

The Windsor CLI enhances your Terraform workflow by automatically managing environment-specific configurations. This is achieved through the use of context-specific `.tfvars` files, which allow your Terraform project files to remain environment-agnostic.

### Contextual Configuration

In your project, the `terraform/` directory houses your standard Terraform files, while the `contexts/<context-name>/terraform` directory contains `.tfvars` files that define specific environments. For example:

- **Terraform Directory**: `terraform/database/postgres`
- **Local Context Variables**: `contexts/local/terraform/database/postgres.tfvars`

When you set a context using `windsor context set`, the Windsor CLI automatically includes the appropriate `.tfvars` files in your Terraform commands. This means running `terraform plan` in the `terraform/database/postgres` directory is equivalent to:

```bash
terraform plan --var-file path/to/contexts/local/terraform/database/postgres.tfvars
```

### Environment Variable Management

The Windsor CLI simplifies the Terraform CLI experience by injecting `TF_CLI_ARGS_*` variables into your environment. These variables ensure that Terraform commands are executed with the correct context and configurations, making your workflow more efficient.

Key functionalities include:

- **Environment Variables Setup**: Automatically configures environment variables to ensure Terraform commands are executed with the correct context.
- **Backend Configuration**: Dynamically creates configuration files for different storage backends (e.g., local, S3, Kubernetes)
- **Alias Management**: Provides the ability to alias Terraform commands, such as using `tflocal` when Localstack is enabled.

For more details on how these environment variables are managed, refer to the [Terraform documentation](https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_cli_args-and-tf_cli_args_name). To view all environment variables managed by the CLI, use:

```bash
windsor env
```

This setup allows for a streamlined workflow, typically requiring only:

```bash
terraform init
terraform plan
terraform apply
```

<!-- Footer Start -->

<div>
  {{ footer('Kustomize', '../kustomize/index.html', 'Secrets Management', '../secrets-management/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../kustomize/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../secrets-management/index.html'; 
  });
</script>

<!-- Footer End -->
