package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccResourceDataSource(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc")
	cfg := testAccResourceConfig{name: name, description: "looked up", license: "MIT", tags: `["database"]`}.String() + `
data "ramble_resource" "test" {
  namespace = ramble_resource.test.namespace
  name      = ramble_resource.test.name
}
`
	const ds = "data.ramble_resource.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourceDestroyed(t),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrPair(ds, "id", "ramble_resource.test", "id"),
				resource.TestCheckResourceAttr(ds, "type", "pack"),
				resource.TestCheckResourceAttr(ds, "description", "looked up"),
				resource.TestCheckResourceAttr(ds, "license", "MIT"),
				resource.TestCheckResourceAttr(ds, "tags.#", "1"),
				resource.TestCheckResourceAttr(ds, "latest_version", "v1.0.0"),
				resource.TestCheckResourceAttr(ds, "repository_url", testAccRepository),
				resource.TestCheckResourceAttrPair(ds, "webhook_url", "ramble_resource.test", "webhook_url"),
				resource.TestCheckNoResourceAttr(ds, "webhook_secret"),
				resource.TestCheckNoResourceAttr(ds, "version"),
			),
		}},
	})
}
