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
    local localOverride = if std.objectHas(registryInfo, "local")
                          then
                            local parts = std.split(registryInfo["local"], "//");
                            if std.length(parts) > 1 then parts[1] else registryInfo["local"]
                          else "";
    
    if std.objectHas(registryInfo, "hostname") && registryInfo.hostname != "" then
      acc + {
        [(if localOverride != "" then localOverride else key)]: {
          endpoints: ["http://" + registryInfo.hostname + ":5000"],
        },
      }
    else
      acc,
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
        // renovate: datasource=github-branches depName=windsorcli/core
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
              network: if context.vm.driver == "docker-desktop" then {
                interfaces: [
                  {
                    ignore: true,
                    interface: "eth0",
                  },
                ],
              } else {},
              kubelet: {
                extraArgs: {
                  "rotate-server-certificates": "true",
                },
              },
            },
          }
          +
          // Conditionally add 'machine.registries' only if registryMirrors is non-empty
          (if std.length(std.objectFields(registryMirrors)) == 0 then
            {}
          else
            {
              machine+: {
                registries: {
                  mirrors: registryMirrors,
                },
              },
            })
        ),
        worker_config_patches: std.manifestYamlDoc(
          if std.objectHas(context.cluster.workers, "volumes") then
            {
              machine: {
                kubelet: {
                  extraMounts: std.map(
                    function(volume)
                      local parts = std.split(volume, ":");
                      {
                        destination: parts[1],
                        type: "bind",
                        source: parts[1],
                        options: [
                          "rbind",
                          "rw",
                        ],
                      },
                    context.cluster.workers.volumes
                  ),
                },
              },
            }
          else
            {}
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
        webhook_token: "abcdef123456",
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
        webhook_token: {
          description: "The token to use for the webhook",
          type: "string",
          default: "",
          sensitive: true,
        },
      }
    }
  ] else [],
  kustomize: [
    {
      name: "policy-base",
      source: "core",
      path: "policy/base",
      components: [
        "kyverno"
      ],
    },
    {
      name: "policy-resources",
      source: "core",
      path: "policy/resources",
      dependsOn: [
        "policy-base"
      ],
    },
    {
      name: "csi",
      source: "core",
      path: "csi",
      dependsOn: [
        "policy-resources"
      ],
      force: true,
      components: [
        "openebs",
        "openebs/dynamic-localpv",
      ],
    },
  ] + (if context.vm.driver != "docker-desktop" then [
    {
      name: "lb-base",
      source: "core",
      path: "lb/base",
      dependsOn: [
        "policy-resources"
      ],
      force: true,
      components: [
        "metallb"
      ],
    },
    {
      name: "lb-resources",
      source: "core",
      path: "lb/resources",
      dependsOn: [
        "lb-base"
      ],
      force: true,
      components: [
        "metallb/layer2"
      ],
    }
  ] else []) + [
    {
      name: "ingress-base",
      source: "core",
      path: "ingress/base",
      dependsOn: [
        "pki-resources"
      ],
      force: true,
      components: if context.vm.driver == "docker-desktop" then [
        "nginx",
        "nginx/nodeport",
        "nginx/coredns",
        "nginx/flux-webhook",
        "nginx/web"
      ] else [
        "nginx",
        "nginx/loadbalancer",
        "nginx/coredns",
        "nginx/flux-webhook",
        "nginx/web"
      ],
    },
    {
      name: "pki-base",
      source: "core",
      path: "pki/base",
      dependsOn: [
        "policy-resources"
      ],
      force: true,
      components: [
        "cert-manager",
        "trust-manager"
      ],
    },
    {
      name: "pki-resources",
      source: "core",
      path: "pki/resources",
      dependsOn: [
        "pki-base"
      ],
      force: true,
      components: [
        "private-issuer/ca",
        "public-issuer/selfsigned"
      ],
    },
    {
      name: "dns",
      source: "core",
      path: "dns",
      dependsOn: [
        "ingress-base",
        "pki-base"
      ],
      force: true,
      components: if context.vm.driver == "docker-desktop" then [
        "coredns",
        "coredns/etcd",
        "external-dns",
        "external-dns/localhost",
        "external-dns/coredns",
        "external-dns/ingress"
      ] else [
        "coredns",
        "coredns/etcd",
        "external-dns",
        "external-dns/coredns",
        "external-dns/ingress"
      ],
    },
    {
      name: "gitops",
      source: "core",
      path: "gitops/flux",
      dependsOn: [
        "ingress-base"
      ],
      force: true,
      components: [
        "webhook"
      ],
    },
    {
      name: "demo",
      source: "core",
      path: "demo/bookinfo",
      dependsOn: [
        "ingress-base"
      ],
      force: true,
      components: [
        "ingress"
      ],
    }
  ],
}
