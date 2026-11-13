variable "talos_image" {
  description = "Talos node image."
  type        = string
  # artifact: type=image
  default = "ghcr.io/siderolabs/installer:v1.11.5"
}
