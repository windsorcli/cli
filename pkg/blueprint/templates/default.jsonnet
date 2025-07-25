local context = std.extVar("context");

{
  kind: "Blueprint",
  apiVersion: "blueprints.windsorcli.dev/v1alpha1",
  metadata: {
    name: context.name,
    description: "This blueprint outlines resources in the " + context.name + " context",
  },
  repository: {
    url: "",
    ref: {
      branch: "main",
    },
    secretName: "flux-system",
  },
  sources: [
    {
      name: "core",
      url: "oci://ghcr.io/windsorcli/core:v0.5.0",
    },
  ],
  terraform: [],
  kustomize: [],
}
