# A pack published to the registry under a user or organisation namespace.
resource "ramble_resource" "mysql" {
  namespace      = "alice"
  name           = "mysql"
  type           = "pack"
  repository_url = "https://github.com/alice/packs"
  file_path      = "packs/mysql"
  description    = "MySQL on Nomad"
  license        = "MIT"
  tags           = ["database", "sql"]

  # Changing the version later adds it to the resource.
  version = "v1.0.0"
}

# Configure a repository webhook to publish new versions.
output "webhook_url" {
  value = ramble_resource.mysql.webhook_url
}

output "webhook_secret" {
  value     = ramble_resource.mysql.webhook_secret
  sensitive = true
}
