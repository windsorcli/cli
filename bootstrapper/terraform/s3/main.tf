provider "aws" {
  region = "us-east-2" // Specify your desired region
}

variable "bucket_name" {
  description = "The name of the S3 bucket to use."
  type        = string
  default     = "bootstrapper-bucket"
}

resource "aws_s3_bucket" "this" {
  bucket = var.bucket_name

  versioning {
    enabled = true
  }

  server_side_encryption_configuration {
    rule {
      apply_server_side_encryption_by_default {
        sse_algorithm = "AES256"
      }
    }
  }

  tags = {
    Name        = "Bootstrapper S3 Bucket"
    Environment = "Production"
  }
}

resource "aws_s3_bucket_public_access_block" "bootstrapper" {
  bucket = aws_s3_bucket.this.id

  block_public_acls   = true
  block_public_policy = true
  ignore_public_acls  = true
  restrict_public_buckets = true
}

output "bootstrapper_bucket_name" {
  value = aws_s3_bucket.this.id
}
