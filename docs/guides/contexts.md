# Contexts

In a Windsor project, you work across different deployments or environments as "contexts". You could think of a context in terms of your software development lifecycle (SDLC) environments, such as `development`, `staging`, and `production`. You could also think of a context in terms of different pieces of your organizational infrastructure -- `admin`, `web`, or `observability`. Or, some combination of schemes. You may have a `web-staging` and `web-production` as well as `observability-staging` and `observability-production`.

Context assets are stored at `contexts/<context-name>/` and consist of the following :

- Authentication details for a cloud service provider account. (_e.g._, AWS credentials)
- Authentication details for a Kubernetes cluster. (_e.g._, a kube config file)
- Encrypted secrets used to configure resources. (_e.g._, KMS-encrypted SOPS file)
- A [Blueprint](../reference/blueprint.md) resource and its associated Terraform and Kustomize configurations.

Generally speaking, it is a good idea to stick to simple patterns. A context may represent a single role with your cloud provider and a single role with your cluster. You consider all the accounts and services associated with a context to share common administrative access. That is, you may be an administrative user of your production AWS account, and likewise have adminstrative access to a Kubernetes cluster running in a VPC in that same account. You consider all of this to be a part of a shared administrative context.

## Creating contexts

You create a Windsor project by running `windsor init`. When doing so, it automatically creates a `local` context. This step results in the following:

- Creates a new folder of assets in `contexts/local`
- Adds a new entry to your project's `windsor.yaml` file at `contexts.local`

**Note:** Not all context names are handled in the same manner. Contexts named `local` or that begin with `local-` assume that you will be running a local cloud virtualization, setting defaults accordingly. You can read more in the documentation on the [local workstation](../guides/local-workstation.md).

To create another context, say one representing production, you run:

```
windsor init production
```

This will create a folder, `contexts/production` with a basic `blueprint.yaml` file.

## Switching contexts

You can switch contexts by running:

```
windsor context set <context-name>
```

You can see the current context by running:

```
windsor context get
```

Additionally, the `WINDSOR_CONTEXT` enviroment variable is available to you.


### Displaying the Active Context in Your Shell Prompt

To visualize the active Windsor context in your shell prompt, you can modify your shell configuration file to include the `WINDSOR_CONTEXT` environment variable.

=== "BASH"
    To include the active context in your prompt, add the following line to your `~/.bashrc` file:
    ```bash
    export PS1="\[\e[32m\]${WINDSOR_CONTEXT:+(\$WINDSOR_CONTEXT)}\[\e[0m\] $PS1"
    ```
    This will prepend the context in green to your existing prompt if `WINDSOR_CONTEXT` is defined.

=== "ZSH"
    To include the active context in your prompt, add the following line to your `~/.zshrc` file:
    ```zsh
    PROMPT="%F{green}${WINDSOR_CONTEXT:+(\$WINDSOR_CONTEXT)}%f $PROMPT"
    ```
    This will prepend the context in green to your existing prompt if `WINDSOR_CONTEXT` is defined.

=== "POWERSHELL"
    To include the active context in your prompt, modify your PowerShell profile script as follows:
    ```powershell
    function prompt {
        if ($env:WINDSOR_CONTEXT) {
            Write-Host "($env:WINDSOR_CONTEXT)" -ForegroundColor Green -NoNewline
        }
        & { $function:prompt }  # Call the original prompt function
    }
    ```
    This will prepend the context in green to your existing prompt if `WINDSOR_CONTEXT` is defined.


These changes will allow you to see the active Windsor context directly in your shell prompt, making it easier to manage and switch between different contexts.
