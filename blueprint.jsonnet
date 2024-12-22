local cpNodes = std.objectValues(context.cluster.controlplanes.nodes);

// Pick "the first" node in the object as a fallback. If there are no nodes,
// you can default to something known or raise an error.
local firstNode = if std.length(cpNodes) > 0 then
  cpNodes[0]
else
  error "No controlplane nodes defined";

{
  kind: "Blueprint",
  apiVersion: "blueprints.windsorcli.dev/v1alpha1",
  metadata: {
    name: "local",
    description: "This blueprint outlines resources in the local context",
  },
  sources: [
    {
      name: "core",
      url: "github.com/windsorcli/core",
      ref: "v0.1.0",
    },
  ],
  terraform: [
    {
      path: "cluster/talos",
      values: {
        kubernetes_version: "1.30.3",
        talos_version: "1.7.6",
        cluster_endpoint: "https://"+firstNode.node+":6443",
        controlplanes: std.map(function(node)
          node, std.objectValues(context.cluster.controlplanes.nodes)),
        workers: std.map(function(node)
          node, std.objectValues(context.cluster.workers.nodes))
      },
      variables: {
        context_path: {
          type: "string",
          description: "The path to the context folder, where kubeconfig and talosconfig are stored",
          default: "",
        },
        os_type: {
          type: "string",
          description: "The operating system type, must be either 'unix' or 'windows'",
          default: "unix",
        },
        kubernetes_version: {
          type: "string",
          description: "The kubernetes version to deploy.",
          default: "1.30.3",
        },
        talos_version: {
          type: "string",
          description: "The talos version to deploy.",
          default: "1.7.6",
        },
        cluster_name: {
          type: "string",
          description: "The name of the cluster.",
          default: "talos",
        },
        cluster_endpoint: {
          type: "string",
          description: "The external controlplane API endpoint of the kubernetes API.",
          default: "https://localhost:6443",
        },
        controlplanes: {
          type: "list(any)",
          description: "A list of machine configuration details for control planes.",
          default: [],
        },
        workers: {
          type: "list(any)",
          description: "A list of machine configuration details",
          default: [],
        },
        common_config_patches: {
          type: "string",
          description: "A YAML string of common config patches to apply. Can be an empty string or valid YAML.",
          default: "",
        },
        controlplane_config_patches: {
          type: "string",
          description: "A YAML string of controlplane config patches to apply. Can be an empty string or valid YAML.",
          default: "",
        },
        worker_config_patches: {
          type: "string",
          description: "A YAML string of worker config patches to apply. Can be an empty string or valid YAML.",
          default: "",
        },
      }
    }
  ],
}
