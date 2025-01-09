module "main" {
  source            = "github.com/windsorcli/core//terraform/gitops/flux?ref=main"
  flux_helm_version = var.flux_helm_version
  flux_namespace    = var.flux_namespace
  flux_version      = var.flux_version
  git_auth_secret   = var.git_auth_secret
  git_password      = var.git_password
  git_username      = var.git_username
  ssh_known_hosts   = var.ssh_known_hosts
  ssh_private_key   = var.ssh_private_key
  ssh_public_key    = var.ssh_public_key
}
