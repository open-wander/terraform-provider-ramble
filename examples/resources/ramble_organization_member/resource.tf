resource "ramble_organization" "platform" {
  name = "platform-team"
}

# Creating this for a user who is already a member adopts the membership and
# sets the role.
resource "ramble_organization_member" "alice" {
  organization = ramble_organization.platform.name
  username     = "alice"
  role         = "member"
}
