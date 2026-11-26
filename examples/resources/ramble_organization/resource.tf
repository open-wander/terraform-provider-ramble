# The user who owns the API token becomes the first owner.
resource "ramble_organization" "platform" {
  name        = "platform-team"
  description = "Packs and jobs of the platform team"
}
