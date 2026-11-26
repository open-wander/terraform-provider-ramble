# An organisation, a member and a pack published under the organisation.
terraform {
  required_providers {
    ramble = {
      source = "open-wander/ramble"
    }
  }
}

# Set the token with the RAMBLE_TOKEN environment variable.
provider "ramble" {
  # address = "https://ramble.openwander.org"
}

resource "ramble_organization" "platform" {
  name        = "platform-team"
  description = "Packs and jobs of the platform team"
}

resource "ramble_organization_member" "alice" {
  organization = ramble_organization.platform.name
  username     = "alice"
  role         = "member"
}

# Using the organisation's name as the namespace makes Terraform destroy the
# pack before the organisation, which Ramble refuses to delete while it holds
# packs or jobs.
resource "ramble_resource" "mysql" {
  namespace      = ramble_organization.platform.name
  name           = "mysql"
  type           = "pack"
  repository_url = "https://github.com/platform-team/packs"
  file_path      = "packs/mysql"
  version        = "v1.0.0"
}
