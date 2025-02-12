---
title: "Contexts"
description: "The Windsor CLI enables a contextual workflow, dynamically reconfiguring your environment and toolchain for each specific deployment context."
image: "https://windsorcli.github.io/latest/img/windsor-logo.png"
---
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

<div>
  {{ footer('Quick Start', '../../quick-start/index.html', 'Environment Injection', '../../guides/environment-injection/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../quick-start/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../guides/environment-injection/index.html'; 
  });
</script>

