local context = std.extVar("context");

// Safely access control plane nodes from the context, defaulting to an empty array if not present
local cpNodes = if std.objectHas(context.cluster.controlplanes, "nodes")
                then std.objectValues(context.cluster.controlplanes.nodes)
                else [];

// Select the first node or default to null if no nodes are present
local firstNode = if std.length(cpNodes) > 0 then cpNodes[0] else null;

// Build the mirrors dynamically
local registryMirrors = std.foldl(
  function(acc, key)
    local regData = context.docker.registries[key];

    // Figure out which endpoint to use:
    //   1) local, if present
    //   2) remote, if present
    local endpointsVal =
      if std.objectHas(regData, "local") && regData["local"] != "" then regData["local"]
      else if std.objectHas(regData, "remote") && regData["remote"] != "" then regData["remote"]
      else null;

    // Must have a non-empty hostname, and endpointsVal must be non-null
    if !(std.objectHas(regData, "hostname")) || regData.hostname == "" || endpointsVal == null then
      acc
    else
      acc + {
        [regData["hostname"]]: {
          endpoints: [endpointsVal],
        },
      },
  std.objectFields(context.docker.registries),
  {}
);

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

        // Create a list of control plane nodes
        controlplanes: std.map(
          function(v) {
            endpoint: v.endpoint,
            hostname: v.hostname,
            node: v.node,
          },
          std.objectValues(context.cluster.controlplanes.nodes)
        ),

        // Create a list of worker nodes
        workers: std.map(
          function(v) {
            endpoint: v.endpoint,
            hostname: v.hostname,
            node: v.node,
          },
          std.objectValues(context.cluster.workers.nodes)
        ),

        // Convert common configuration patches to YAML format
        common_config_patches: std.manifestYamlDoc(
          // We'll build the 'cluster' and 'machine' objects,
          // then conditionally add 'registries' if needed
          {
            cluster: {
              apiServer: {
                certSANs: [
                  "localhost",
                  firstNode.node,
                ],
              },
            },
          }
          +
          // Merge in the base `machine` config
          {
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
          }
          +
          // Conditionally add 'machine.registries' only if registryMirrors is non-empty
          if std.length(std.objectFields(registryMirrors)) == 0 then
            {}
          else
            {
              machine+: {
                registries: {
                  mirrors: registryMirrors,
                },
              },
            }
        ),
      },
      variables: {
        context_path: {
          type: "string",
          description: "Where kubeconfig and talosconfig are stored",
          default: "",
        },
        os_type: {
          type: "string",
          description: "Must be 'unix' or 'windows'",
          default: "unix",
        },
        kubernetes_version: {
          type: "string",
          description: "Kubernetes version to deploy",
          default: "1.31.4",
        },
        talos_version: {
          type: "string",
          description: "The Talos version to deploy",
          default: "1.8.4",
        },
        cluster_name: {
          type: "string",
          description: "The name of the cluster",
          default: "talos",
        },
        cluster_endpoint: {
          type: "string",
          description: "The external controlplane API endpoint of the kubernetes API",
          default: "https://localhost:6443",
        },
        controlplanes: {
          type: "list(any)",
          description: "Machine config details for control planes",
          default: [],
        },
        workers: {
          type: "list(any)",
          description: "Machine config details for workers",
          default: [],
        },
        common_config_patches: {
          type: "string",
          description: "A YAML string of common config patches to apply",
          default: "",
        },
        controlplane_config_patches: {
          type: "string",
          description: "A YAML string of controlplane config patches to apply",
          default: "",
        },
        worker_config_patches: {
          type: "string",
          description: "A YAML string of worker config patches to apply",
          default: "",
        },
      }
    } else {}
  ],
}
