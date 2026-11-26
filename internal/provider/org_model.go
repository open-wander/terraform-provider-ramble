package provider

import (
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

// organizationModel is the state of a ramble_organization.
type organizationModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

// organizationMemberModel is the state of a ramble_organization_member.
type organizationMemberModel struct {
	ID           types.String `tfsdk:"id"`
	Organization types.String `tfsdk:"organization"`
	Username     types.String `tfsdk:"username"`
	Role         types.String `tfsdk:"role"`
}

// applyOrganization copies an API organisation into m. The id is the name as
// Ramble stores it and never changes, so a change of letter case in the
// configuration cannot change it. The name keeps the configured spelling.
func applyOrganization(m *organizationModel, o rambleapi.HandlersOrgResponse) {
	m.ID = types.StringValue(o.Name)
	m.Name = keepSpelling(m.Name, o.Name)
	m.Description = types.StringValue(o.Description)
	m.CreatedAt = types.StringValue(o.CreatedAt.UTC().Format(time.RFC3339Nano))
}

// parseMemberImportID parses "organization/username".
func parseMemberImportID(s string) (org, username string, err error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected an import id of the form organization/username (for example myorg/alice), got %q", s)
	}
	return parts[0], parts[1], nil
}
