# Terraform Provider for Ramble

The Terraform provider for [Ramble](https://ramble.openwander.org), the registry for Nomad packs and jobs. It talks to the Ramble `/v1` API with an API token, so a team can manage its registry content as code.

It is built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) and works with Terraform and OpenTofu.

## What it manages

The provider is under construction.

Available:

- `ramble_resource` (resource and data source): a pack or job published to the registry. Import with `terraform import ramble_resource.x namespace/name`.

Planned:

- `ramble_organization`: an organisation
- `ramble_organization_member`: a user's membership of an organisation

## Using the provider

```hcl
terraform {
  required_providers {
    ramble = {
      source = "open-wander/ramble"
    }
  }
}

provider "ramble" {
  # address = "https://ramble.openwander.org"
  # token   = var.ramble_token
}
```

| Attribute | Environment variable | Default |
|-----------|----------------------|---------|
| `address` | `RAMBLE_ADDR` | `https://ramble.openwander.org` |
| `token` (sensitive) | `RAMBLE_TOKEN` | none |

The attribute wins over the environment variable, which wins over the default. Prefer the environment variable for the token so it stays out of configuration files.

Rules the provider enforces:

- The token is required. A provider block without one fails at configure time with a clear error. `terraform validate` does not configure providers, so it does not catch this.
- A value that is not known until apply (for example computed by another resource) is an error rather than a deferred configuration.
- The token is only sent over `https`, or plain `http` to `localhost` and loopback addresses for local testing. An address with credentials in it is refused.
- Redirects are never followed, so the token cannot be forwarded to another host. Response bodies are size-limited.
- Create the token in Ramble with write scope.

## Compatibility

Planned. No release has been made yet.

| Provider | Ramble |
|----------|--------|
| v0.1.x | v0.6.x |

The `/v1` write API this provider uses is not in a Ramble release yet; CI tests against Ramble's `main` branch.

## Developing the provider

Requirements: [Go](https://golang.org/doc/install) (see `go.mod`), [Terraform](https://developer.hashicorp.com/terraform/downloads) or [OpenTofu](https://opentofu.org), and Docker for the acceptance tests.

```shell
go build ./...     # build
go test ./...      # unit tests
make lint          # golangci-lint
make generate      # API client, docs and formatted examples
```

`internal/client/openapi.yaml` is a verbatim copy of Ramble's `api-docs/openapi.yaml`, and `internal/client/client.gen.go` is generated from it with a pinned [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) (`make generate` checks the versions agree with `go.mod`). Build clients with `rambleapi.New`, which enforces the token rules above; do not use the generated constructors. CI fails when `make generate` leaves a difference.

To try a local build, use a [development override](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers) pointing at `go env GOBIN`.

### Acceptance tests

Acceptance tests run against a real Ramble server built from source. The sibling checkout `../ramble` is used by default; set `RAMBLE_SRC` to use another.

```shell
make testacc-env-up           # Postgres in Docker, Ramble from source, a seeded test user and token
. ./.testacc.env              # exports RAMBLE_ADDR and RAMBLE_TOKEN
make testacc                  # TF_ACC=1 go test ./...
make testacc-env-down
```

The seed is test-only code in `scripts/seed`. It inserts a verified user and a write-scope token straight into the database and writes them to `.testacc.env` (mode 0600, git-ignored). With OpenTofu, also set `TF_ACC_TERRAFORM_PATH` to the `tofu` binary and `TF_ACC_PROVIDER_HOST=registry.opentofu.org`.

Ramble limits writes to 120 per minute per token, which matters once resource tests run in parallel.

### CI

`.github/workflows/test.yml` runs `build` (with golangci-lint), `generate`, `unit` and `acceptance`. The acceptance job builds Ramble from `open-wander/ramble` (input `ramble_ref`, default `main`) and runs the tests for two Terraform minor lines and the latest OpenTofu. These four names are required checks (`.github/rulesets/main.json`).

### Releasing

Tags matching `v*` run GoReleaser (`.github/workflows/release.yml`), which signs the checksums and needs the `GPG_PRIVATE_KEY` and `PASSPHRASE` repository secrets.

## Licence

[MPL-2.0](LICENSE).
