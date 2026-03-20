terraform {
  required_providers {
    exoscale = {
      source  = "exoscale/exoscale"
      version = ">= 0.62.0"
    }
    aws = {
      source  = "hashicorp/aws"
      version = "6.37.0"
    }
  }

  # Exoscale SOS (S3-compatible) remote state.
  # Credentials and state key are passed via -backend-config in CI.
  # To bootstrap: create the bucket once with `exo storage mb sos://axonops-operator-tfstate`
  backend "s3" {
    bucket = "axonops-operator-tfstate"
    region = "ch-gva-2"
  }
}

provider "aws" {
  endpoints {
    s3 = "https://sos-${local.zone}.exo.io"
  }

  region     = local.zone

  # Disable AWS-specific features
  skip_credentials_validation = true
  skip_region_validation      = true
  skip_requesting_account_id  = true
}

locals {
  zone = "ch-gva-2"
}

variable "exoscale_api_key" {
  description = "Exoscale API key. Can also be set via the EXOSCALE_API_KEY environment variable."
  type        = string
  default     = null
  sensitive   = true
}

variable "exoscale_api_secret" {
  description = "Exoscale API secret. Can also be set via the EXOSCALE_API_SECRET environment variable."
  type        = string
  default     = null
  sensitive   = true
}

variable "exoscale_ssh_public_key" {
  description = "SSH public key allowing access to these instances"
  type        = string
  sensitive   = true
}

provider "exoscale" {
  key    = var.exoscale_api_key
  secret = var.exoscale_api_secret
}

resource "exoscale_ssh_key" "ssh_key" {
  name       = "github-ssh-key"
  public_key = var.exoscale_ssh_public_key
}

module "k3s" {
  source            = "github.com/digitalis-io/terraform-exoscale-k3s"
  ssh_key_name      = "github-ssh-key"
  agent_count       = 3
  ssh_allowed_cidrs = ["0.0.0.0/0"]
}

output "server_ip" {
  value = module.k3s.server_public_ip
}

output "kubeconfig_cmd" {
  value = module.k3s.kubeconfig_command
}

