variable "flux_helm_version" {
  type        = string
  default     = "2.14.0"
  description = "The version of Flux Helm chart to install"
}
variable "flux_namespace" {
  type        = string
  default     = "system-gitops"
  description = "The namespace in which Flux will be installed"
}
variable "flux_version" {
  type        = string
  default     = "2.4.0"
  description = "The version of Flux to install"
}
variable "git_auth_secret" {
  type        = string
  default     = "flux-system"
  description = "The name of the secret to store the git authentication details"
}
variable "git_password" {
  type        = string
  default     = ""
  description = "The git password or PAT used to authenticate with the git provider"
  sensitive   = true
}
variable "git_username" {
  type        = string
  default     = "git"
  description = "The git user to use to authenticate with the git provider"
}
variable "ssh_known_hosts" {
  type        = string
  default     = ""
  description = "The known hosts to use for SSH authentication"
  sensitive   = true
}
variable "ssh_private_key" {
  type        = string
  default     = ""
  description = "The private key to use for SSH authentication"
  sensitive   = true
}
variable "ssh_public_key" {
  type        = string
  default     = ""
  description = "The public key to use for SSH authentication"
  sensitive   = true
}
