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
    local registryInfo = context.docker.registries[key];
    // Must have a non-empty hostname
    if !(std.objectHas(registryInfo, "hostname")) || registryInfo.hostname == "" then
      acc
    else
      acc + {
        [key]: {
          endpoints: ["http://" + registryInfo["hostname"] + ":5000"],
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
  repository: {
    url: "http://git.test/git/" + context.projectName,
    ref: {
      branch: "main",
    },
    secretName: "flux-system",
  },
  sources: [
    {
      name: "core",
      url: "github.com/windsorcli/core",
      ref: {
        branch: "main",
      },
    },
  ],
  terraform: if firstNode != null then [
    {
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

        // Create a list of worker nodesq
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
              extraManifests: [
                // renovate: datasource=github-releases depName=kubelet-serving-cert-approver package=alex1989hu/kubelet-serving-cert-approver
                "https://raw.githubusercontent.com/alex1989hu/kubelet-serving-cert-approver/v0.8.7/deploy/standalone-install.yaml",
              ],
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
              kubelet: {
                extraArgs: {
                  "rotate-server-certificates": "true",
                },
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
    },
    {
      path: "gitops/flux",
      source: "core",
      values: {
        git_username: "local",
        git_password: "local",
      },
      variables: {
        flux_namespace: {
          description: "The namespace in which Flux will be installed",
          type: "string",
          default: "system-gitops",
        },
        flux_helm_version: {
          description: "The version of Flux Helm chart to install",
          type: "string",
          default: "2.14.0",
        },
        flux_version: {
          description: "The version of Flux to install",
          type: "string",
          default: "2.4.0",
        },
        ssh_private_key: {
          description: "The private key to use for SSH authentication",
          type: "string",
          default: "",
          sensitive: true,
        },
        ssh_public_key: {
          description: "The public key to use for SSH authentication",
          type: "string",
          default: "",
          sensitive: true,
        },
        ssh_known_hosts: {
          description: "The known hosts to use for SSH authentication",
          type: "string",
          default: "",
          sensitive: true,
        },
        git_auth_secret: {
          description: "The name of the secret to store the git authentication details",
          type: "string",
          default: "flux-system",
        },
        git_username: {
          description: "The git user to use to authenticate with the git provider",
          type: "string",
          default: "git",
        },
        git_password: {
          description: "The git password or PAT used to authenticate with the git provider",
          type: "string",
          default: "",
          sensitive: true,
        },
      }
    }
  ] else [],
  kustomize: [
    {
      name: "local",
      path: "",
    }
  ]
}
