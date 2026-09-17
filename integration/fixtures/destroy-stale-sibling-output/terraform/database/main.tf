terraform {}

variable "vpc_id" {
  type    = string
  default = null

  validation {
    condition     = var.vpc_id != null
    error_message = "vpc_id is required; pipe network's vpc_id output"
  }
}

resource "terraform_data" "this" {
  input = var.vpc_id
}
