
# `windsor env`

The `windsor env` command provides the current environment configuration for your project. It is especially useful for dynamically managing environment variables based on the context of your current working directory.

This command outputs the intended state of shell environment variables, determined by the command prompt's location within the file structure and the current context.

## Purpose

The `windsor env` command is designed to dynamically manage and output environment variables specific to the current working directory. This allows developers to:

- **Adapt to Context Changes**: Automatically adjust environment variables as you navigate through different directories, ensuring that the correct settings are applied for each project or environment.
- **Simplify Configuration Management**: Reduce manual configuration efforts by providing a snapshot of the current environment settings, which can be easily reviewed and modified as needed.
- **Enhance Debugging and Troubleshooting**: Quickly verify and adjust environment variables to resolve issues, ensuring that the development environment is correctly configured at all times.

## Context Sensitivity and Folder Structure Awareness

The `windsor env` command is designed to be context-sensitive and aware of the folder structure. This means:

- **Dynamic Environment Variables**: The command generates environment variables that are specific to the current directory. As you navigate through different directories, the environment variables are adjusted to match the needs of the project or environment in that directory.
- **Project-Specific Context**: The command uses the directory structure to determine the appropriate context and settings, ensuring that the right configurations are applied automatically.

## How It Works

```bash
+-------------------+     +---------------------+     +-------------------------------+
| Current Context   |---->|  windsor env        |     | $shell                        |
+-------------------+     |    export env-var-1 |---->| precmd(eval "$(windsor env)") |
+-------------------+     |    export env-var-n |     | %prompt%                      |
| Current Directory |---->|                     |     |                               |
+-------------------+     +---------------------+     +-------------------------------+
```

![full-bootstrap](../img/full-bootstrap.gif)


- **`precmd()` Function**: This function is executed before each prompt is displayed in your shell. By placing the `windsor env` command inside this function, you ensure that the environment variables are updated every time you change directories or execute a command.
- **Context Sensitivity**: The `windsor env` command generates environment variable settings that are context-sensitive to the location of the command prompt in the file structure. This means that as you navigate through different directories, the environment variables are adjusted accordingly to match the needs of the project or environment in that directory.

## Benefits

- **Dynamic Configuration**: Automatically updates environment variables based on the current directory, ensuring that you always have the correct settings for your project.
- **Consistency**: Helps maintain consistent environment settings across different projects and team members.
- **Efficiency**: Reduces the need for manual configuration changes, saving time and minimizing errors.

By using the `windsor env` command in conjunction with the `precmd()` function, developers can maintain a flexible and efficient development environment that adapts to their current working context.

## Example Usage with Ansible, Terraform, Talos, AWS, and Kubectl

The `windsor env`command outputs a list of environment variables for the current context or state. 

````bash
cd terraform/talos
talos% windsor env
unset OMNICONFIG
export ANSIBLE_BECOME_PASSWORD="****"
unset COMPOSE_FILE
export TF_CLI_ARGS_apply="*****/contexts/local/.terraform/cluster/talos/terraform.tfplan"
export TF_CLI_ARGS_destroy="-var-file=*****/contexts/local/terraform/cluster/talos.tfvars \
  -var-file=*****/contexts/local/terraform/cluster/talos_generated.tfvars.json"
export TF_CLI_ARGS_import="-var-file=*****/contexts/local/terraform/cluster/talos.tfvars \
  -var-file=*****/contexts/local/terraform/cluster/talos_generated.tfvars.json"
export TF_CLI_ARGS_init="-backend=true -backend-config=path=*****/contexts/local/.tfstate/cluster/talos/terraform.tfstate"
export TF_CLI_ARGS_plan="-out=*****/contexts/local/.terraform/cluster/talos/terraform.tfplan \
  -var-file=*****/contexts/local/terraform/cluster/talos.tfvars \
  -var-file=*****/contexts/local/terraform/cluster/talos_generated.tfvars.json"
export TF_DATA_DIR="*****/contexts/local/.terraform/cluster/talos"
export TF_VAR_context_path="*****/contexts/local"
export TALOSCONFIG="*****/contexts/local/.talos/config"
export WINDSOR_CONTEXT="local"
export WINDSOR_PROJECT_ROOT="*****"
export KUBECONFIG="*****/contexts/local/.kube/config"
export KUBE_CONFIG_PATH="*****/contexts/local/.kube/config"
export AWS_CONFIG_FILE="*****/contexts/local/.aws/config"
export AWS_ENDPOINT_URL="http://aws.test:4566"
export AWS_PROFILE="local"
export MWAA_ENDPOINT="http://mwaa.local.aws.test:4566"
export S3_HOSTNAME="http://s3.local.aws.test:4566"
````

In the case above, the talos folder is under the terraform folder so environment variables for terraform, talos, kube, aws and other tools are set

## [Environment Variables](../guides/configuration.md)

The exection of the windsor env output in the command prompt sets environment variables to configure various tools. Environment variables are key-value pairs that can be used to customize the behavior of software applications without altering the code. They provide a flexible way to manage configuration settings, such as API keys, database connections, and other sensitive information, which can vary between different environments (e.g., development, testing, production).

In this context, the `windsor env` command is used to set up these environment variables. It is a tool-agnostic command, meaning it does not directly interact with or wrap the functionality of any specific tool. Instead, it prepares the environment by setting the necessary variables, allowing any tool that relies on these variables to function correctly. This separation of concerns ensures that `windsor env` can be used in conjunction with a wide range of tools, providing a consistent and reusable way to manage environment configurations.

By using environment variables, tools can be more easily configured and deployed across different environments, enhancing portability and reducing the risk of configuration errors. This approach also supports better security practices by keeping sensitive information out of the source code.

See [Environment Variables](../guides/configuration.md) for a more detailed description of environment variables.