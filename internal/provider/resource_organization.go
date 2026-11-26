package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

var (
	_ resource.Resource                = &OrganizationResource{}
	_ resource.ResourceWithConfigure   = &OrganizationResource{}
	_ resource.ResourceWithImportState = &OrganizationResource{}
)

// NewOrganizationResource returns the ramble_organization resource.
func NewOrganizationResource() resource.Resource {
	return &OrganizationResource{}
}

// OrganizationResource manages an organisation in the Ramble registry.
type OrganizationResource struct {
	client *rambleapi.ClientWithResponses
}

func (r *OrganizationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (r *OrganizationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An organisation in a Ramble registry. Packs and jobs can be published under its name.\n\n" +
			"The user who owns the API token becomes the first owner of a new organisation. " +
			"Other members are managed with `ramble_organization_member`.\n\n" +
			"An organisation can be deleted only when it has no packs or jobs. Set `namespace` of each `ramble_resource` to " +
			"`ramble_organization.<name>.name` so that Terraform destroys the resources first.\n\n" +
			"The number of members is not an attribute of this resource, because it changes whenever a member is added; " +
			"the `ramble_organization` data source reports it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The name of the organisation as Ramble stores it.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the organisation: 1-64 characters of letters, digits, dot, dash and underscore, starting with a letter or digit. " +
					"Some names are reserved, and a name cannot be the same as a username. " +
					"Changing it replaces the resource; a change of letter case only does not, because Ramble compares names case-insensitively.",
				Required:      true,
				Validators:    []validator.String{orgNameValidator{}},
				PlanModifiers: []planmodifier.String{requiresReplaceUnlessCaseOnly()},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Short description. Empty by default.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation time, RFC 3339.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *OrganizationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrganizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan organizationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.client.CreateOrgWithResponse(ctx, rambleapi.HandlersCreateOrgRequest{
		Name:        plan.Name.ValueString(),
		Description: optionalString(plan.Description),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create the organisation", "The request to Ramble failed: "+err.Error())
		return
	}
	if res.StatusCode() != http.StatusCreated || res.JSON201 == nil {
		resp.Diagnostics.Append(apiErrorDiag("Unable to create the organisation", res.StatusCode(), res.Body))
		return
	}

	// Read the organisation back: the create response carries the server's
	// in-memory timestamp (nanoseconds), while every later read returns the
	// stored one (microseconds), which would make created_at drift.
	org := *res.JSON201
	if got, err := r.client.GetOrgWithResponse(ctx, org.Name); err == nil && got.StatusCode() == http.StatusOK && got.JSON200 != nil {
		org = *got.JSON200
	}
	applyOrganization(&plan, org)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrganizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state organizationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.client.GetOrgWithResponse(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the organisation", "The request to Ramble failed: "+err.Error())
		return
	}
	if res.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
		resp.Diagnostics.Append(apiErrorDiag("Unable to read the organisation", res.StatusCode(), res.Body))
		return
	}

	applyOrganization(&state, *res.JSON200)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OrganizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state organizationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only the description can change in place. A change of the name's case
	// alone needs no request, only a read.
	var org *rambleapi.HandlersOrgResponse
	if !plan.Description.Equal(state.Description) {
		v := plan.Description.ValueString()
		res, err := r.client.UpdateOrgWithResponse(ctx, state.ID.ValueString(), rambleapi.HandlersUpdateOrgRequest{Description: &v})
		if err != nil {
			resp.Diagnostics.AddError("Unable to update the organisation", "The request to Ramble failed: "+err.Error())
			return
		}
		if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
			resp.Diagnostics.Append(apiErrorDiag("Unable to update the organisation", res.StatusCode(), res.Body))
			return
		}
		org = res.JSON200
	} else {
		res, err := r.client.GetOrgWithResponse(ctx, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to read the organisation", "The request to Ramble failed: "+err.Error())
			return
		}
		if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
			resp.Diagnostics.Append(apiErrorDiag("Unable to read the organisation", res.StatusCode(), res.Body))
			return
		}
		org = res.JSON200
	}

	applyOrganization(&plan, *org)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrganizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.client.DeleteOrgWithResponse(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete the organisation", "The request to Ramble failed: "+err.Error())
		return
	}
	if res.StatusCode() != http.StatusNoContent && res.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.Append(apiErrorDiag("Unable to delete the organisation", res.StatusCode(), res.Body))
	}
}

func (r *OrganizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Read fills in the rest, and the name in its stored spelling.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
