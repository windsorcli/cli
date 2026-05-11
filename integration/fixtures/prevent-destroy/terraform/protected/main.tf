resource "null_resource" "guarded" {
  lifecycle {
    prevent_destroy = true
  }
}
