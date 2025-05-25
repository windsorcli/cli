local context = std.extVar("context");

// Repository configuration
local repositoryConfig = {
  url: "",
  ref: {
    branch: "main",
  },
  secretName: "flux-system",
};

// Terraform configuration
local terraformConfig = [
  {
    path: "network/aws-vpc",
    source: "core",
  },
  {
    path: "cluster/aws-eks",
    source: "core",
  },
  {
    path: "gitops/flux",
    source: "core",
    destroy: false,
  }
];

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
    name: "ingress",
    source: "core",
    path: "ingress",
    dependsOn: [
      "pki-resources"
    ],
    force: true,
    components: [
      "nginx",
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
    name: "observability",
    source: "core",
    path: "observability",
    dependsOn: [
      "ingress"
    ],
    components: [
      "grafana",
      "grafana/ingress",
      "grafana/prometheus",
      "grafana/node",
      "grafana/kubernetes",
      "grafana/flux"
    ],
  }
] else [];

// Blueprint metadata
local blueprintMetadata = {
  kind: "Blueprint",
  apiVersion: "blueprints.windsorcli.dev/v1alpha1",
  metadata: {
    name: context.name,
    description: "This blueprint outlines resources in the " + context.name + " context",
  },
};

// Source configuration
local sourceConfig = [
  {
    name: "core",
    url: "github.com/windsorcli/core",
    ref: {
      branch: "main",
    },
  },
];

// Start of Blueprint
blueprintMetadata + {
  repository: repositoryConfig,
  sources: sourceConfig,
  terraform: terraformConfig,
  kustomize: kustomizeConfig,
} 
