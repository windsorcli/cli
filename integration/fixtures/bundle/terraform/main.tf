variable "flux_cli_version" {
  description = "Version of the Flux CLI release."
  type        = string
  # renovate: datasource=github-releases depName=flux package=fluxcd/flux2
  default = "v2.3.0"
}
