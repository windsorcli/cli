// Access the external variable "context"
local context = std.extVar("context");

// Safely access control plane nodes from the context, defaulting to an empty array if not present
local cpNodes = if std.objectHas(context.cluster.controlplanes, "nodes")
                then std.objectValues(context.cluster.controlplanes.nodes)
                else [];

// Select the first node or default to null if no nodes are present
local firstNode = if std.length(cpNodes) > 0 then cpNodes[0] else null;

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
    if firstNode != null then {
      path: "cluster/talos",
      source: "core",
      values: {
        // Construct the cluster endpoint URL using the first node's address
        cluster_endpoint: "https://" + firstNode.node + ":6443",
        cluster_name: "talos",
        // Map control plane nodes to their configuration if available
        controlplanes: std.mapWithKey(
          function(k, v) {
            endpoint: v.endpoint,
            hostname: k + ".test",
            node: v.node,
          },
          context.cluster.controlplanes.nodes
        ),
        // Map worker nodes to their configuration if available
        workers: std.mapWithKey(
          function(k, v) {
            endpoint: v.endpoint,
            hostname: k + ".test",
            node: v.node,
          },
          context.cluster.workers.nodes
        ),
        // Convert common configuration patches to YAML format
        common_config_patches: std.manifestYamlDoc({
          cluster: {
            apiServer: {
              certSANs: [
                "localhost",
                firstNode.node,
              ],
            },
          },
          machine: {
            certSANs: [
              "localhost",
              firstNode.node,
            ],
            features: {
              hostDNS: {
                forwardKubeDNSToHost: true,
              },
            },
            network: {
              interfaces: [
                {
                  ignore: true,
                  interface: "eth0",
                },
              ],
            },
          },
        }),
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
          default: "1.31.4",
        },
        talos_version: {
          type: "string",
          description: "The talos version to deploy.",
          default: "1.8.4",
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
    } else {}
  ],
}
