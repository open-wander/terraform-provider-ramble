package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

var (
	_ resource.Resource                = &OrganizationMemberResource{}
	_ resource.ResourceWithConfigure   = &OrganizationMemberResource{}
	_ resource.ResourceWithImportState = &OrganizationMemberResource{}
)

// NewOrganizationMemberResource returns the ramble_organization_member resource.
func NewOrganizationMemberResource() resource.Resource {
	return &OrganizationMemberResource{}
}

// OrganizationMemberResource manages a user's membership of an organisation.
type OrganizationMemberResource struct {
	client *rambleapi.ClientWithResponses
}

func (r *OrganizationMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_member"
}

func (r *OrganizationMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A user's membership of a Ramble organisation, with a role.\n\n" +
			"Creating this resource for a user who is already a member adopts the membership and sets its role.\n\n" +
			"The user who owns the API token is the first owner of every organisation they create, without any " +
			"`ramble_organization_member`. Managing that membership with this resource is possible but risky: " +
			"an organisation must keep at least one owner, so setting the role to `member` or destroying the resource " +
			"fails with `last_owner` while the token's user is the only owner. Removing the token's user while another " +
			"owner exists succeeds, but the token then loses its rights over the organisation. " +
			"Do not manage the membership of the token's own user unless another owner exists.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "`organization/username`, as given when the resource was created or imported.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"organization": schema.StringAttribute{
				MarkdownDescription: "Name of the organisation. Changing it replaces the resource; a change of letter case only does not.",
				Required:            true,
				Validators:          []validator.String{orgNameValidator{}},
				PlanModifiers:       []planmodifier.String{requiresReplaceUnlessCaseOnly()},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Username of the member. The user must exist. Changing it replaces the resource; a change of letter case only does not.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers:       []planmodifier.String{requiresReplaceUnlessCaseOnly()},
			},
			"role": schema.StringAttribute{
				MarkdownDescription: "Either `owner` or `member`. Changed in place.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf("owner", "member")},
			},
		},
	}
}

func (r *OrganizationMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*rambleapi.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("Expected *ClientWithResponses, got %T. This is a provider bug.", req.ProviderData))
		return
	}
	r.client = c
}

// put sets the role of the member named in plan and records what Ramble
// answers. The id is only set when it is not known yet.
func (r *OrganizationMemberResource) put(ctx context.Context, plan *organizationMemberModel, summary string) (d diag.Diagnostics) {
	res, err := r.client.PutOrgMemberWithResponse(ctx, plan.Organization.ValueString(), plan.Username.ValueString(),
		rambleapi.HandlersPutOrgMemberRequest{Role: rambleapi.HandlersPutOrgMemberRequestRole(plan.Role.ValueString())})
	if err != nil {
		d.AddError(summary, "The request to Ramble failed: "+err.Error())
		return d
	}
	if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
		d.Append(apiErrorDiag(summary, res.StatusCode(), res.Body))
		return d
	}
	plan.Username = keepSpelling(plan.Username, res.JSON200.Username)
	plan.Role = types.StringValue(string(res.JSON200.Role))
	if plan.ID.IsNull() || plan.ID.IsUnknown() {
		plan.ID = types.StringValue(plan.Organization.ValueString() + "/" + plan.Username.ValueString())
	}
	return d
}

func (r *OrganizationMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan organizationMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, &plan, "Unable to add the organisation member")...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrganizationMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state organizationMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.client.ListOrgMembersWithResponse(ctx, state.Organization.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the organisation members", "The request to Ramble failed: "+err.Error())
		return
	}
	switch {
	case res.StatusCode() == http.StatusNotFound:
		resp.State.RemoveResource(ctx)
		return
	case res.StatusCode() == http.StatusForbidden:
		// Not treated as gone: a permission problem must not plan a re-create.
		d := apiErrorDiag("Unable to read the organisation members", res.StatusCode(), res.Body)
		resp.Diagnostics.AddError(d.Summary(), d.Detail()+
			"\n\nOnly a member of the organisation can list its members. The token's user may have been removed from "+
			state.Organization.ValueString()+".")
		return
	case res.StatusCode() != http.StatusOK || res.JSON200 == nil:
		resp.Diagnostics.Append(apiErrorDiag("Unable to read the organisation members", res.StatusCode(), res.Body))
		return
	}

	for _, m := range *res.JSON200 {
		if strings.EqualFold(m.Username, state.Username.ValueString()) {
			state.Username = keepSpelling(state.Username, m.Username)
			state.Role = types.StringValue(string(m.Role))
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *OrganizationMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state organizationMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The id never changes, even when only the case of a name did.
	plan.ID = state.ID
	resp.Diagnostics.Append(r.put(ctx, &plan, "Unable to change the organisation member")...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrganizationMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.client.RemoveOrgMemberWithResponse(ctx, state.Organization.ValueString(), state.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to remove the organisation member", "The request to Ramble failed: "+err.Error())
		return
	}
	if res.StatusCode() != http.StatusNoContent && res.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.Append(apiErrorDiag("Unable to remove the organisation member", res.StatusCode(), res.Body))
	}
}

func (r *OrganizationMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	org, username, err := parseMemberImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import id", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization"), org)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("username"), username)...)
}
