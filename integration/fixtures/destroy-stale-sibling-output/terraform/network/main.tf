terraform {}

resource "terraform_data" "this" {}

output "vpc_id" {
  value = "vpc-fake-123"
}
