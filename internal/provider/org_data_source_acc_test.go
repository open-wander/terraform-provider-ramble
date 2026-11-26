package provider

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccOrganizationDataSource(t *testing.T) {
	member := os.Getenv("RAMBLE_TEST_MEMBER")
	org := strings.ToLower(acctest.RandomWithPrefix("tfacc-org"))
	cfg := fmt.Sprintf(`
resource "ramble_organization" "test" {
  name        = %q
  description = "looked up"
}

resource "ramble_organization_member" "test" {
  organization = ramble_organization.test.name
  username     = %q
  role         = "member"
}

data "ramble_organization" "test" {
  name       = ramble_organization.test.name
  depends_on = [ramble_organization_member.test]
}
`, org, member)
	const ds = "data.ramble_organization.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckMember(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ds, "id", org),
					resource.TestCheckResourceAttr(ds, "description", "looked up"),
					resource.TestCheckResourceAttrSet(ds, "created_at"),
					resource.TestCheckResourceAttr(ds, "member_count", "2"),
					resource.TestCheckResourceAttr(ds, "members.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs(ds, "members.*", map[string]string{"username": "tfacc", "role": "owner"}),
					resource.TestCheckTypeSetElemNestedAttrs(ds, "members.*", map[string]string{"username": member, "role": "member"}),
				),
			},
		},
	})
}

func TestAccOrganizationDataSource_invalidName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckMember(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      `data "ramble_organization" "test" { name = "bad name" }`,
			PlanOnly:    true,
			ExpectError: regexpMust(`Invalid organisation name`),
		}},
	})
}

func TestAccOrganizationDataSource_notFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckMember(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      `data "ramble_organization" "test" { name = "tfacc-no-such-org-zz" }`,
			ExpectError: regexpMust(`404`),
		}},
	})
}
