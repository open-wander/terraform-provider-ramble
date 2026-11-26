data "ramble_organization" "platform" {
  name = "platform-team"
}

output "platform_member_count" {
  value = data.ramble_organization.platform.member_count
}

# Null when the token's user is not a member of the organisation.
output "platform_owners" {
  value = [
    for m in coalesce(data.ramble_organization.platform.members, []) : m.username if m.role == "owner"
  ]
}
