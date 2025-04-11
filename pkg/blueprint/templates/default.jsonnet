local context = std.extVar("context");

// Determine the platform, defaulting to an empty string if not defined
local platform = if std.objectHas(context, "cluster") && std.objectHas(context.cluster, "platform") && context.cluster.platform != null then context.cluster.platform else "";

// Determine the vmDriver, defaulting to an empty string if not defined
local vmDriver = if std.objectHas(context, "vm") && std.objectHas(context.vm, "driver")
                 then context.vm.driver
                 else "";

// Safely access control plane nodes from the context, defaulting to an empty array if not present
local cpNodes = if std.objectHas(context, "cluster") && std.objectHas(context.cluster, "controlplanes") && std.objectHas(context.cluster.controlplanes, "nodes")
                then std.objectValues(context.cluster.controlplanes.nodes)
                else [];

// Select the first node or default to null if no nodes are present
local firstNode = if std.length(cpNodes) > 0 then cpNodes[0] else null;

// Extract baseUrl from endpoint
local extractBaseUrl(endpoint) = 
  if endpoint == "" then "" else
    local parts = std.split(endpoint, "://");
    if std.length(parts) > 1 then
      local hostParts = std.split(parts[1], ":");
      hostParts[0]
    else
      local hostParts = std.split(endpoint, ":");
      hostParts[0];

// Determine the endpoint, using cluster.endpoint if available, otherwise falling back to firstNode
local endpoint = if std.objectHas(context.cluster, "endpoint") then context.cluster.endpoint else if firstNode != null then firstNode.endpoint else "";
local baseUrl = extractBaseUrl(endpoint);

// Build the mirrors dynamically, only if registries are defined
local registryMirrors = if std.objectHas(context, "docker") && std.objectHas(context.docker, "registries") then
  std.foldl(
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
  )
else {};

// Blueprint metadata
local blueprintMetadata = {
  kind: "Blueprint",
  apiVersion: "blueprints.windsorcli.dev/v1alpha1",
  metadata: {
    name: context.name,
    description: "This blueprint outlines resources in the " + context.name + " context",
  },
};

// Repository configuration
local repositoryConfig = {
  url: if platform != "local" then "" else "http://git.test/git/" + context.projectName,
  ref: {
    branch: "main",
  },
  secretName: "flux-system",
};

// Source configuration
local sourceConfig = [
  {
    name: "core",
    url: "github.com/windsorcli/core",
    ref: {
      // renovate: datasource=github-branches depName=windsorcli/core
      branch: "main",
    },
  },
];

// Terraform configuration
local terraformConfig = if platform == "local" || platform == "metal" then [
  {
    path: "cluster/talos",
    source: "core",
    values: {
      // Use the determined endpoint
      cluster_endpoint: if endpoint != "" then "https://" + baseUrl + ":6443" else "",
      cluster_name: "talos",

      // Create a list of control plane nodes
      controlplanes: if std.objectHas(context.cluster, "controlplanes") && std.objectHas(context.cluster.controlplanes, "nodes") then
        std.map(
          function(v) {
            endpoint: v.endpoint,
            node: v.node,
          },
          std.objectValues(context.cluster.controlplanes.nodes)
        )
      else [],

      // Create a list of worker nodes
      workers: if std.objectHas(context.cluster, "workers") && std.objectHas(context.cluster.workers, "nodes") then
        std.map(
          function(v) {
            endpoint: v.endpoint,
            node: v.node,
          },
          std.objectValues(context.cluster.workers.nodes)
        )
      else [],

      // Convert common configuration patches to YAML format
      common_config_patches: std.manifestYamlDoc(
        // We'll build the 'cluster' and 'machine' objects,
        // then conditionally add 'registries' if needed
        {
          cluster: {
            apiServer: {
              certSANs: [
                "localhost",
                baseUrl,
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
              baseUrl,
            ],
            network: if vmDriver == "docker-desktop" then {
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
      worker_config_patches: if std.objectHas(context.cluster, "workers") && std.objectHas(context.cluster.workers, "volumes") && std.length(context.cluster.workers.volumes) > 0 then
        std.manifestYamlDoc(
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
        )
      else
        "",
      controlplane_config_patches: if std.objectHas(context.cluster, "controlplanes") && std.objectHas(context.cluster.controlplanes, "volumes") && std.length(context.cluster.controlplanes.volumes) > 0 then
        std.manifestYamlDoc(
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
                  context.cluster.controlplanes.volumes
                ),
              },
            },
          }
        )
      else
        "",
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
    values: if platform == "local" then {
      git_username: "local",
      git_password: "local",
      webhook_token: "abcdef123456",
    } else {},
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
] else [];

// Determine the blueprint, defaulting to an empty string if not defined
local blueprint = if std.objectHas(context, "blueprint") then context.blueprint else "";

// Kustomize configuration
local kustomizeConfig = if blueprint == "full" then [
  {
    name: "telemetry-base",
    source: "core",
    path: "telemetry/base",
    components: [
      "prometheus",
      "prometheus/flux"
    ],
  },
  {
    name: "telemetry-resources",
    source: "core",
    path: "telemetry/resources",
    dependsOn: [
      "telemetry-base"
    ],
    components: [
      "metrics-server",
      "prometheus",
      "prometheus/flux"
    ],
  },
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
] + (if vmDriver != "docker-desktop" then [
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
    components: if vmDriver == "docker-desktop" then [
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
      "pki-base"
    ],
    force: true,
    components: if vmDriver == "docker-desktop" then [
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
  }
] else [];

// Start of Blueprint
blueprintMetadata + {
  repository: repositoryConfig,
  sources: sourceConfig,
  terraform: terraformConfig,
  kustomize: kustomizeConfig,
}
