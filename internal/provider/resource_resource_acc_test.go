package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

const (
	testAccNamespace = "tfacc"
	// A real public repository: creating a resource starts a background fetch
	// from it. The fetch may fail without affecting the resource.
	testAccRepository = "https://github.com/hashicorp/nomad-pack-community-registry"
)

func testAccClient(t *testing.T) *rambleapi.ClientWithResponses {
	t.Helper()
	c, err := rambleapi.New(os.Getenv("RAMBLE_ADDR"), os.Getenv("RAMBLE_TOKEN"), "test")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// testAccCheckResourceDestroyed confirms every ramble_resource in state is
// gone from Ramble.
func testAccCheckResourceDestroyed(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ramble_resource" {
				continue
			}
			id, err := strconv.Atoi(rs.Primary.ID)
			if err != nil {
				return err
			}
			res, err := c.GetResourceWithResponse(context.Background(), id)
			if err != nil {
				return err
			}
			if res.StatusCode() != http.StatusNotFound {
				return fmt.Errorf("resource %d still exists: status %d", id, res.StatusCode())
			}
		}
		return nil
	}
}

type testAccResourceConfig struct {
	name, typ, description, license, version string
	tags                                     string // HCL, for example `["a", "b"]`
}

func (c testAccResourceConfig) String() string {
	typ := c.typ
	if typ == "" {
		typ = "pack"
	}
	version := c.version
	if version == "" {
		version = "v1.0.0"
	}
	out := fmt.Sprintf(`
resource "ramble_resource" "test" {
  namespace      = %q
  name           = %q
  type           = %q
  repository_url = %q
  version        = %q
`, testAccNamespace, c.name, typ, testAccRepository, version)
	if c.description != "" {
		out += fmt.Sprintf("  description    = %q\n", c.description)
	}
	if c.license != "" {
		out += fmt.Sprintf("  license        = %q\n", c.license)
	}
	if c.tags != "" {
		out += fmt.Sprintf("  tags           = %s\n", c.tags)
	}
	return out + "}\n"
}

func TestAccResource_lifecycle(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc")
	const addr = "ramble_resource.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourceDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: testAccResourceConfig{name: name}.String(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckResourceAttr(addr, "namespace", testAccNamespace),
					resource.TestCheckResourceAttr(addr, "name", name),
					resource.TestCheckResourceAttr(addr, "type", "pack"),
					resource.TestCheckResourceAttr(addr, "version", "v1.0.0"),
					resource.TestCheckResourceAttr(addr, "latest_version", "v1.0.0"),
					resource.TestCheckResourceAttr(addr, "description", ""),
					resource.TestCheckResourceAttr(addr, "tags.#", "0"),
					resource.TestCheckResourceAttrSet(addr, "license"),
					resource.TestCheckResourceAttrSet(addr, "webhook_url"),
					resource.TestCheckResourceAttrSet(addr, "webhook_secret"),
					resource.TestCheckResourceAttrSet(addr, "created_at"),
				),
			},
			{
				// The framework already asserts an empty plan after apply;
				// this makes it explicit with a second refresh and plan.
				Config:   testAccResourceConfig{name: name}.String(),
				PlanOnly: true,
			},
			{
				Config: testAccResourceConfig{name: name, description: "first description", license: "MIT", tags: `["database", "sql"]`}.String(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "description", "first description"),
					resource.TestCheckResourceAttr(addr, "license", "MIT"),
					resource.TestCheckResourceAttr(addr, "tags.#", "2"),
					resource.TestCheckTypeSetElemAttr(addr, "tags.*", "database"),
					resource.TestCheckTypeSetElemAttr(addr, "tags.*", "sql"),
					resource.TestCheckResourceAttrSet(addr, "webhook_secret"),
				),
			},
			{
				// Removing description and tags clears them.
				Config: testAccResourceConfig{name: name, license: "MIT"}.String(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "description", ""),
					resource.TestCheckResourceAttr(addr, "tags.#", "0"),
					resource.TestCheckResourceAttr(addr, "license", "MIT"),
				),
			},
			{
				Config: testAccResourceConfig{name: name, license: "MIT", version: "v1.1.0"}.String(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "version", "v1.1.0"),
					resource.TestCheckResourceAttr(addr, "latest_version", "v1.1.0"),
					resource.TestCheckResourceAttrSet(addr, "webhook_secret"),
				),
			},
			{
				Config:            testAccResourceConfig{name: name, license: "MIT", version: "v1.1.0"}.String(),
				ResourceName:      addr,
				ImportState:       true,
				ImportStateId:     testAccNamespace + "/" + name,
				ImportStateVerify: true,
				// Ramble returns the secret only when it creates the resource.
				ImportStateVerifyIgnore: []string{"webhook_secret"},
			},
		},
	})
}

func TestAccResource_importByID(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc")
	const addr = "ramble_resource.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourceDestroyed(t),
		Steps: []resource.TestStep{
			{Config: testAccResourceConfig{name: name}.String()},
			{
				ResourceName:            addr,
				ImportState:             true,
				ImportStateIdFunc:       func(s *terraform.State) (string, error) { return s.RootModule().Resources[addr].Primary.ID, nil },
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"webhook_secret"},
			},
		},
	})
}

// After an import webhook_secret is null; a later update must keep it null
// rather than plan an unknown value.
func TestAccResource_updateAfterImport(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc")
	const addr = "ramble_resource.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourceDestroyed(t),
		Steps: []resource.TestStep{
			{
				// Create outside Terraform, then import into an empty state.
				PreConfig: func() {
					c := testAccClient(t)
					res, err := c.CreateResourceWithResponse(context.Background(), rambleapi.HandlersCreateResourceRequest{
						Namespace:     testAccNamespace,
						Name:          name,
						Type:          "pack",
						RepositoryUrl: testAccRepository,
						Version:       "v1.0.0",
						Tags:          &[]string{},
					})
					if err != nil {
						t.Fatalf("create %s: %v", name, err)
					}
					if res.StatusCode() != http.StatusCreated {
						t.Fatalf("create %s: status %d", name, res.StatusCode())
					}
				},
				Config:             testAccResourceConfig{name: name}.String(),
				ResourceName:       addr,
				ImportState:        true,
				ImportStateId:      testAccNamespace + "/" + name,
				ImportStatePersist: true,
			},
			{
				Config: testAccResourceConfig{name: name, description: "after import"}.String(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "description", "after import"),
					resource.TestCheckNoResourceAttr(addr, "webhook_secret"),
				),
			},
		},
	})
}

func TestAccResource_replace(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc")
	const addr = "ramble_resource.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourceDestroyed(t),
		Steps: []resource.TestStep{
			{Config: testAccResourceConfig{name: name}.String()},
			{
				Config: testAccResourceConfig{name: name + "-b"}.String(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionReplace)},
				},
			},
			{
				Config: testAccResourceConfig{name: name + "-b", typ: "job"}.String(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionReplace)},
				},
				Check: resource.TestCheckResourceAttr(addr, "type", "job"),
			},
		},
	})
}

// A change of letter case in the name is the same Ramble resource: it is
// updated in place, never replaced.
func TestAccResource_caseOnlyRename(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc")
	const addr = "ramble_resource.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourceDestroyed(t),
		Steps: []resource.TestStep{
			{Config: testAccResourceConfig{name: name}.String()},
			{
				Config: testAccResourceConfig{name: strings.ToUpper(name)}.String(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(addr, "name", strings.ToUpper(name)),
			},
			{
				Config: testAccResourceConfig{name: strings.ToUpper(name)}.String(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestAccResource_invalidConfig(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc")
	tests := []struct {
		name   string
		cfg    testAccResourceConfig
		expect string
	}{
		{"uppercase tag", testAccResourceConfig{name: name, tags: `["Database"]`}, "lowercase"},
		{"short tag", testAccResourceConfig{name: name, tags: `["a"]`}, "2-20"},
		{"bad type", testAccResourceConfig{name: name, typ: "module"}, "pack|job"},
		{"padded version", testAccResourceConfig{name: name, version: " v1"}, "whitespace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      tt.cfg.String(),
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(tt.expect),
				}},
			})
		})
	}
}

// A resource deleted outside Terraform is planned for re-creation.
func TestAccResource_drift(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc")
	const addr = "ramble_resource.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourceDestroyed(t),
		Steps: []resource.TestStep{
			{Config: testAccResourceConfig{name: name}.String()},
			{
				PreConfig: func() {
					c := testAccClient(t)
					got, err := c.GetResourceByNameWithResponse(context.Background(), testAccNamespace, name)
					if err != nil || got.JSON200 == nil {
						t.Fatalf("look up %s/%s: %v", testAccNamespace, name, err)
					}
					del, err := c.DeleteResourceWithResponse(context.Background(), got.JSON200.Id)
					if err != nil || del.StatusCode() != http.StatusNoContent {
						t.Fatalf("delete %d: %v", got.JSON200.Id, err)
					}
				},
				Config: testAccResourceConfig{name: name}.String(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionCreate)},
				},
			},
		},
	})
}

// A duplicate of an existing resource is refused with Ramble's message and the
// import hint.
func TestAccResource_conflict(t *testing.T) {
	name := acctest.RandomWithPrefix("tfacc")
	cfg := testAccResourceConfig{name: name}.String() + fmt.Sprintf(`
resource "ramble_resource" "dup" {
  namespace      = %q
  name           = %q
  type           = "pack"
  repository_url = %q
  version        = "v1.0.0"
  depends_on     = [ramble_resource.test]
}
`, testAccNamespace, name, testAccRepository)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckResourceDestroyed(t),
		Steps: []resource.TestStep{{
			Config:      cfg,
			ExpectError: regexp.MustCompile(`terraform\s+import\s+ramble_resource`),
		}},
	})
}
