module "main" {
  source                      = "github.com/windsorcli/core//terraform/cluster/talos?ref=main"
  cluster_endpoint            = var.cluster_endpoint
  cluster_name                = var.cluster_name
  common_config_patches       = var.common_config_patches
  context_path                = var.context_path
  controlplane_config_patches = var.controlplane_config_patches
  controlplanes               = var.controlplanes
  kubernetes_version          = var.kubernetes_version
  os_type                     = var.os_type
  talos_version               = var.talos_version
  worker_config_patches       = var.worker_config_patches
  workers                     = var.workers
}
