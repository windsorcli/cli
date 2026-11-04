variable "chart_repository" {
  type    = string
  default = "oci://ghcr.io/stefanprodan/charts"
}

variable "image_registry" {
  type    = string
  default = "ghcr.io/example"
}

resource "helm_release" "podinfo" {
  repository = var.chart_repository
  chart      = "podinfo"
  name       = "podinfo-bootstrap"
  namespace  = "apps"
  version    = "6.9.2"
}

resource "kubernetes_deployment_v1" "worker" {
  metadata {
    name = "worker"
  }
  spec {
    template {
      metadata {}
      spec {
        container {
          name  = "worker"
          image = "${var.image_registry}/worker:1.0.0"
        }
      }
    }
  }
}
