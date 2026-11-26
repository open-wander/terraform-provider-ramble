package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

func regexpMust(s string) *regexp.Regexp { return regexp.MustCompile(s) }

func testAccMemberConfig(org, username, role string) string {
	return fmt.Sprintf(`
resource "ramble_organization" "test" {
  name = %q
}

resource "ramble_organization_member" "test" {
  organization = ramble_organization.test.name
  username     = %q
  role         = %q
}
`, org, username, role)
}

// testAccMembers returns the members of org, by lowercase username.
func testAccMembers(ctx context.Context, t *testing.T, org string) (map[string]string, error) {
	t.Helper()
	res, err := testAccClient(t).ListOrgMembersWithResponse(ctx, org)
	if err != nil {
		return nil, err
	}
	if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
		return nil, fmt.Errorf("listing members of %q: status %d", org, res.StatusCode())
	}
	out := map[string]string{}
	for _, m := range *res.JSON200 {
		out[strings.ToLower(m.Username)] = string(m.Role)
	}
	return out, nil
}

// testAccCheckMemberRole checks the role Ramble holds for the member.
func testAccCheckMemberRole(t *testing.T, org, username, role string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		members, err := testAccMembers(context.Background(), t, org)
		if err != nil {
			return err
		}
		if got := members[strings.ToLower(username)]; got != role {
			return fmt.Errorf("role of %q in %q is %q, want %q", username, org, got, role)
		}
		return nil
	}
}

func TestAccOrganizationMember_lifecycle(t *testing.T) {
	member := os.Getenv("RAMBLE_TEST_MEMBER")
	org := strings.ToLower(acctest.RandomWithPrefix("tfacc-org"))
	const addr = "ramble_organization_member.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckMember(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: testAccMemberConfig(org, member, "member"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", org+"/"+member),
					resource.TestCheckResourceAttr(addr, "organization", org),
					resource.TestCheckResourceAttr(addr, "username", member),
					resource.TestCheckResourceAttr(addr, "role", "member"),
					testAccCheckMemberRole(t, org, member, "member"),
				),
			},
			{
				Config:   testAccMemberConfig(org, member, "member"),
				PlanOnly: true,
			},
			{
				Config: testAccMemberConfig(org, member, "owner"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "role", "owner"),
					resource.TestCheckResourceAttr(addr, "id", org+"/"+member),
					testAccCheckMemberRole(t, org, member, "owner"),
				),
			},
			{
				// Case-only changes of the username are in place; the id stays.
				Config: testAccMemberConfig(org, strings.ToUpper(member), "owner"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "username", strings.ToUpper(member)),
					resource.TestCheckResourceAttr(addr, "id", org+"/"+member),
				),
			},
			{
				ResourceName:            addr,
				ImportState:             true,
				ImportStateId:           org + "/" + member,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"username"},
			},
			{
				// The member resource is dropped from the config: the member is
				// removed while the organisation stays.
				Config: testAccOrgConfig(org, ""),
				Check: func(*terraform.State) error {
					members, err := testAccMembers(context.Background(), t, org)
					if err != nil {
						return err
					}
					if _, ok := members[strings.ToLower(member)]; ok {
						return fmt.Errorf("%q is still a member of %q", member, org)
					}
					return nil
				},
			},
		},
	})
}

func TestAccOrganizationMember_drift(t *testing.T) {
	member := os.Getenv("RAMBLE_TEST_MEMBER")
	org := strings.ToLower(acctest.RandomWithPrefix("tfacc-org"))
	const addr = "ramble_organization_member.test"
	cfg := testAccMemberConfig(org, member, "member")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckMember(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed(t),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				PreConfig: func() {
					res, err := testAccClient(t).RemoveOrgMemberWithResponse(context.Background(), org, member)
					if err != nil {
						t.Fatalf("removing the member out of band: %v", err)
					}
					if res.StatusCode() != http.StatusNoContent {
						t.Fatalf("removing the member out of band: status %d", res.StatusCode())
					}
				},
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionCreate)},
				},
				Check: testAccCheckMemberRole(t, org, member, "member"),
			},
		},
	})
}

func TestAccOrganizationMember_adoptsExistingMember(t *testing.T) {
	member := os.Getenv("RAMBLE_TEST_MEMBER")
	org := strings.ToLower(acctest.RandomWithPrefix("tfacc-org"))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckMember(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed(t),
		Steps: []resource.TestStep{
			{Config: testAccOrgConfig(org, "")},
			{
				PreConfig: func() {
					res, err := testAccClient(t).PutOrgMemberWithResponse(context.Background(), org, member,
						rambleapi.HandlersPutOrgMemberRequest{Role: "owner"})
					if err != nil {
						t.Fatalf("adding the member out of band: %v", err)
					}
					if res.StatusCode() != http.StatusOK {
						t.Fatalf("adding the member out of band: status %d", res.StatusCode())
					}
				},
				Config: testAccMemberConfig(org, member, "member"),
				Check:  testAccCheckMemberRole(t, org, member, "member"),
			},
		},
	})
}

func TestAccOrganizationMember_invalidRole(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      testAccMemberConfig("someorg", "someone", "admin"),
			PlanOnly:    true,
			ExpectError: regexpMust(`(?s)role.*owner.*member`),
		}},
	})
}
