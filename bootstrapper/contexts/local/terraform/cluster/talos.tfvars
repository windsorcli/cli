// Managed by Windsor CLI: This file is partially managed by the windsor CLI. Your changes will not be overwritten.
// Module source: github.com/windsorcli/core//terraform/cluster/talos?ref=v0.3.0

// The external controlplane API endpoint of the kubernetes API
cluster_endpoint = "https://127.0.0.1:6443"

// The name of the cluster
cluster_name = "talos"

// A YAML string of common config patches to apply
common_config_patches = "\"cluster\":\n  \"apiServer\":\n    \"certSANs\":\n    - \"localhost\"\n    - \"127.0.0.1\"\n  \"extraManifests\":\n  - \"https://raw.githubusercontent.com/alex1989hu/kubelet-serving-cert-approver/v0.8.7/deploy/standalone-install.yaml\"\n\"machine\":\n  \"certSANs\":\n  - \"localhost\"\n  - \"127.0.0.1\"\n  \"kubelet\":\n    \"extraArgs\":\n      \"rotate-server-certificates\": \"true\"\n  \"network\":\n    \"interfaces\":\n    - \"ignore\": true\n      \"interface\": \"eth0\"\n  \"registries\":\n    \"mirrors\":\n      \"docker.io\":\n        \"endpoints\":\n        - \"http://registry-1.docker.test:5000\"\n      \"gcr.io\":\n        \"endpoints\":\n        - \"http://gcr.test:5000\"\n      \"ghcr.io\":\n        \"endpoints\":\n        - \"http://ghcr.test:5000\"\n      \"quay.io\":\n        \"endpoints\":\n        - \"http://quay.test:5000\"\n      \"registry.k8s.io\":\n        \"endpoints\":\n        - \"http://registry.k8s.test:5000\"\n      \"registry.test\":\n        \"endpoints\":\n        - \"http://registry.test:5000\""

// A YAML string of controlplane config patches to apply
controlplane_config_patches = ""

// Machine config details for control planes
controlplanes = [{
  endpoint = "127.0.0.1:50000"
  hostname = "controlplane-1.test"
  node     = "127.0.0.1"
}]

// A YAML string of worker config patches to apply
worker_config_patches = "\"machine\":\n  \"kubelet\":\n    \"extraMounts\":\n    - \"destination\": \"/var/local\"\n      \"options\":\n      - \"rbind\"\n      - \"rw\"\n      \"source\": \"/var/local\"\n      \"type\": \"bind\""

// Machine config details for workers
workers = [{
  endpoint = "127.0.0.1:50001"
  hostname = "worker-1.test"
  node     = "127.0.0.1"
}]
