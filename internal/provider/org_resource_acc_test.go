package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccCheckOrganizationDestroyed confirms every ramble_organization in
// state is gone from Ramble.
func testAccCheckOrganizationDestroyed(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ramble_organization" {
				continue
			}
			res, err := c.GetOrgWithResponse(context.Background(), rs.Primary.ID)
			if err != nil {
				return err
			}
			if res.StatusCode() != http.StatusNotFound {
				return fmt.Errorf("organisation %q still exists: status %d", rs.Primary.ID, res.StatusCode())
			}
		}
		return nil
	}
}

// testAccCheckAllDestroyed runs every destroy check of the organisation tests.
func testAccCheckAllDestroyed(t *testing.T) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		testAccCheckResourceDestroyed(t),
		testAccCheckOrganizationDestroyed(t),
	)
}

func testAccOrgConfig(name, description string) string {
	out := fmt.Sprintf("resource \"ramble_organization\" \"test\" {\n  name = %q\n", name)
	if description != "" {
		out += fmt.Sprintf("  description = %q\n", description)
	}
	return out + "}\n"
}

func TestAccOrganization_lifecycle(t *testing.T) {
	name := strings.ToLower(acctest.RandomWithPrefix("tfacc-org"))
	upper := strings.ToUpper(name[:1]) + name[1:]
	const addr = "ramble_organization.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgConfig(name, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", name),
					resource.TestCheckResourceAttr(addr, "name", name),
					resource.TestCheckResourceAttr(addr, "description", ""),
					resource.TestCheckResourceAttrSet(addr, "created_at"),
				),
			},
			{
				Config:   testAccOrgConfig(name, ""),
				PlanOnly: true,
			},
			{
				Config: testAccOrgConfig(name, "first description"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(addr, "description", "first description"),
			},
			{
				// Removing the attribute clears the description in place.
				Config: testAccOrgConfig(name, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(addr, "description", ""),
			},
			{
				// A change of letter case only is in place, and the id stays.
				Config: testAccOrgConfig(upper, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "name", upper),
					resource.TestCheckResourceAttr(addr, "id", name),
				),
			},
			{
				Config:   testAccOrgConfig(upper, ""),
				PlanOnly: true,
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateId:     name,
				ImportStateVerify: true,
				// The imported name has the stored spelling, which the
				// previous step's configuration changed in case only.
				ImportStateVerifyIgnore: []string{"name"},
			},
		},
	})
}

func TestAccOrganization_importKeepsDescription(t *testing.T) {
	name := strings.ToLower(acctest.RandomWithPrefix("tfacc-org"))
	const addr = "ramble_organization.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed(t),
		Steps: []resource.TestStep{
			{Config: testAccOrgConfig(name, "imported")},
			{ResourceName: addr, ImportState: true, ImportStateId: name, ImportStateVerify: true},
		},
	})
}

func TestAccOrganization_invalidName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      testAccOrgConfig("-not valid", ""),
			PlanOnly:    true,
			ExpectError: regexpMust(`Invalid organisation name`),
		}},
	})
}

// TestAccOrganization_destroyOrder proves that a resource in the organisation
// is destroyed before the organisation, which Ramble refuses to delete while
// it holds packs or jobs.
func TestAccOrganization_destroyOrder(t *testing.T) {
	name := strings.ToLower(acctest.RandomWithPrefix("tfacc-org"))
	cfg := fmt.Sprintf(`
resource "ramble_organization" "test" {
  name = %q
}

resource "ramble_resource" "test" {
  namespace      = ramble_organization.test.name
  name           = "pack"
  type           = "pack"
  repository_url = %q
  version        = "v1.0.0"
}
`, name, testAccRepository)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed(t),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check:  resource.TestCheckResourceAttrPair("ramble_resource.test", "namespace", "ramble_organization.test", "name"),
		}},
	})
}

// An organisation deleted outside Terraform is planned for re-creation.
func TestAccOrganization_drift(t *testing.T) {
	name := strings.ToLower(acctest.RandomWithPrefix("tfacc-org"))
	const addr = "ramble_organization.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed(t),
		Steps: []resource.TestStep{
			{Config: testAccOrgConfig(name, "")},
			{
				PreConfig: func() {
					res, err := testAccClient(t).DeleteOrgWithResponse(context.Background(), name)
					if err != nil {
						t.Fatalf("delete %s out of band: %v", name, err)
					}
					if res.StatusCode() != http.StatusNoContent {
						t.Fatalf("delete %s out of band: status %d", name, res.StatusCode())
					}
				},
				Config: testAccOrgConfig(name, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionCreate)},
				},
			},
		},
	})
}
