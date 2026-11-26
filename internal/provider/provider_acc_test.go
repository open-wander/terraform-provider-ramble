package provider

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

// testAccPreCheck fails clearly when the acceptance environment is missing.
// Start it with 'make testacc-env-up' and load it with '. ./.testacc.env'.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	for _, k := range []string{"RAMBLE_ADDR", "RAMBLE_TOKEN"} {
		if os.Getenv(k) == "" {
			t.Fatalf("%s must be set for acceptance tests (see 'make testacc-env-up')", k)
		}
	}
}

// TestAccProvider_environment proves the acceptance environment end to end:
// the seeded token authenticates against the running Ramble and carries write
// scope. A provider block with no resources may never be configured by
// Terraform, so the client call is the real proof.
func TestAccProvider_environment(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless TF_ACC is set")
	}
	testAccPreCheck(t)

	c, err := rambleapi.New(os.Getenv("RAMBLE_ADDR"), os.Getenv("RAMBLE_TOKEN"), "test")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.GetCurrentUserWithResponse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		t.Fatalf("GET /v1/user status = %d", resp.StatusCode())
	}
	if resp.JSON200.Username != "tfacc" {
		t.Fatalf("username = %q, want the seeded user", resp.JSON200.Username)
	}
	if string(resp.JSON200.TokenScope) != "write" {
		t.Fatalf("token scope = %q, want write", resp.JSON200.TokenScope)
	}
}

// TestAccProvider_configure checks that Terraform accepts the provider block
// against the real environment.
func TestAccProvider_configure(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:   `provider "ramble" {}`,
				PlanOnly: true,
			},
		},
	})
}
