provider "ramble" {
  # Defaults to https://ramble.openwander.org, or the RAMBLE_ADDR environment variable.
  address = "https://ramble.openwander.org"

  # Prefer the RAMBLE_TOKEN environment variable to keeping a token in configuration.
  token = var.ramble_token
}

variable "ramble_token" {
  type      = string
  sensitive = true
}
