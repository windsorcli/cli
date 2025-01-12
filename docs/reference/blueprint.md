# Blueprint

The blueprint stores references and configuration specific to a context. It's configured in a `blueprint.yaml` file located in your context's configuration folder, such as `contexts/local/blueprint.yaml`.

When you run `windsor init local`, a default local blueprint file is created, as follows:

```
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

The sections in this file are outlined below.

## kind
Following Kubernetes API conventions, the kind or type of resource. In this case, a `Blueprint`.

## apiVersion
Specifies the version of the API schema the blueprint adheres to. Changes to this field represent breaking changes to the schema and will require migration.

## metadata
Contains metadata about the blueprint, including its name and a brief description. This section helps identify and describe the blueprint's purpose.
