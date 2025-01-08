variable "cluster_endpoint" {
  type        = string
  default     = "https://localhost:6443"
  description = "The external controlplane API endpoint of the kubernetes API"
}
variable "cluster_name" {
  type        = string
  default     = "talos"
  description = "The name of the cluster"
}
variable "common_config_patches" {
  type        = string
  default     = ""
  description = "A YAML string of common config patches to apply"
}
variable "context_path" {
  type        = string
  default     = ""
  description = "Where kubeconfig and talosconfig are stored"
}
variable "controlplane_config_patches" {
  type        = string
  default     = ""
  description = "A YAML string of controlplane config patches to apply"
}
variable "controlplanes" {
  type        = list(any)
  default     = []
  description = "Machine config details for control planes"
}
variable "kubernetes_version" {
  type        = string
  default     = "1.31.4"
  description = "Kubernetes version to deploy"
}
variable "os_type" {
  type        = string
  default     = "unix"
  description = "Must be 'unix' or 'windows'"
}
variable "talos_version" {
  type        = string
  default     = "1.8.4"
  description = "The Talos version to deploy"
}
variable "worker_config_patches" {
  type        = string
  default     = ""
  description = "A YAML string of worker config patches to apply"
}
variable "workers" {
  type        = list(any)
  default     = []
  description = "Machine config details for workers"
}
