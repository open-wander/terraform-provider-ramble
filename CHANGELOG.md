## 0.1.0 (November 27, 2026)

FEATURES:

* **New Resource:** `ramble_resource`: a pack or job in the registry, with import by `namespace/name`.
* **New Data Source:** `ramble_resource`: look up a pack or job by namespace and name.
* **New Resource:** `ramble_organization`: an organisation, with import by name.
* **New Resource:** `ramble_organization_member`: a user's membership of an organisation and its role, with import by `organization/username`.
* **New Data Source:** `ramble_organization`: look up an organisation by name, with its member count and members.

NOTES:

* `ramble_organization` has no `member_count` attribute, because it changes whenever a member is added and would show as a change made outside Terraform. The `ramble_organization` data source reports it.
* The `members` of the `ramble_organization` data source are `null` when the token's user is not a member of the organisation, because Ramble lists members only to its own members.
* Error hints for 409 responses no longer name `ramble_resource` for every conflict; organisation errors (`org_not_empty`, `last_owner`, `invalid_role`, `invalid_name`) have their own advice.
* The acceptance environment seeds a second user and exports its name as `RAMBLE_TEST_MEMBER`.

* Provider skeleton: provider configuration (`address`, `token`), a generated Ramble API client and the acceptance-test environment.
