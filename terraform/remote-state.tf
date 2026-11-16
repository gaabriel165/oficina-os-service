data "terraform_remote_state" "k8s" {
  backend = "s3"

  config = {
    bucket = var.state_bucket
    key    = "k8s/terraform.tfstate"
    region = var.region
  }
}

data "aws_caller_identity" "current" {}

locals {
  oidc_provider_arn = data.terraform_remote_state.k8s.outputs.oidc_provider_arn
  oidc_provider     = data.terraform_remote_state.k8s.outputs.oidc_provider
}
