variable "cluster_endpoint" {
  type        = string
  default     = "https://localhost:6443"
  description = "The external controlplane API endpoint of the kubernetes API."
}
variable "cluster_name" {
  type        = string
  default     = "talos"
  description = "The name of the cluster."
}
variable "common_config_patches" {
  type        = string
  default     = ""
  description = "A YAML string of common config patches to apply. Can be an empty string or valid YAML."
}
variable "context_path" {
  type        = string
  default     = ""
  description = "The path to the context folder, where kubeconfig and talosconfig are stored"
}
variable "controlplane_config_patches" {
  type        = string
  default     = ""
  description = "A YAML string of controlplane config patches to apply. Can be an empty string or valid YAML."
}
variable "controlplanes" {
  type        = list(any)
  default     = []
  description = "A list of machine configuration details for control planes."
}
variable "kubernetes_version" {
  type        = string
  default     = "1.30.3"
  description = "The kubernetes version to deploy."
}
variable "os_type" {
  type        = string
  default     = "unix"
  description = "The operating system type, must be either 'unix' or 'windows'"
}
variable "talos_version" {
  type        = string
  default     = "1.7.6"
  description = "The talos version to deploy."
}
variable "worker_config_patches" {
  type        = string
  default     = ""
  description = "A YAML string of worker config patches to apply. Can be an empty string or valid YAML."
}
variable "workers" {
  type        = list(any)
  default     = []
  description = "A list of machine configuration details"
}
