---
title: "Contexts"
description: "Understanding how contexts work in Windsor"
---
# Contexts

In a Windsor project, you work across different deployments or environments as "contexts". You could think of a context in terms of your software development lifecycle (SDLC) environments, such as `development`, `staging`, and `production`. You could also think of a context in terms of different pieces of your organizational infrastructure -- `admin`, `web`, or `observability`. Or, some combination of schemes. You may have a `web-staging` and `web-production` as well as `observability-staging` and `observability-production`.

Generally speaking, it is a good idea to stick to simple patterns. A context may represent a single role with your cloud provider and a single role with your cluster. You consider all the accounts and services associated with a context to share common administrative access. That is, you may be an administrative user of your production AWS account, and likewise have administrative access to a Kubernetes cluster running in a VPC in that same account. You consider all of this to be a part of a shared administrative context.

## Creating Contexts

You create a Windsor project by running `windsor init`. When doing so, it automatically creates a `local` context. This step results in the following:

- Creates a new folder of assets in `contexts/local`
- Adds a new entry to your project's `windsor.yaml` file at `contexts.local`

**Note:** Not all context names are handled in the same manner. Contexts named `local` or that begin with `local-` assume that you will be running a local cloud virtualization, setting defaults accordingly. You can read more in the documentation on the [local workstation](local-workstation.md).

To create another context, say one representing production, you run:

```bash
windsor init production --provider aws
```

This will create a folder, `contexts/production` with a basic `blueprint.yaml` file and default `windsor.yaml` file.

## Switching Contexts

You can switch contexts by running:

```bash
windsor context set <context-name>
```

You can see the current context by running:

```bash
windsor context get
```

Additionally, the `WINDSOR_CONTEXT` environment variable is available to you.

## Blueprint Templates

Contexts are generated from blueprint templates stored in `contexts/_template/`. These templates define reusable blueprint components, features, and schemas that are shared across all contexts. See the [Blueprint Templates](templates.md) guide for details on how templates work.

For detailed reference information about contexts, see the [Contexts Reference](../reference/contexts.md).
