package provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

var (
	_ resource.Resource                = &ResourceResource{}
	_ resource.ResourceWithConfigure   = &ResourceResource{}
	_ resource.ResourceWithImportState = &ResourceResource{}
)

// NewResourceResource returns the ramble_resource resource.
func NewResourceResource() resource.Resource {
	return &ResourceResource{}
}

// ResourceResource manages a pack or job in the Ramble registry.
type ResourceResource struct {
	client *rambleapi.ClientWithResponses
}

func (r *ResourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource"
}

func (r *ResourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A pack or job published to a Ramble registry.\n\n" +
			"Ramble fetches the content of each version from the repository in the background. A failed fetch does not " +
			"fail the resource: Terraform manages the registry entry, not the fetch.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Ramble's numeric id of the resource.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the pack or job. Changing it replaces the resource; a change of letter case only does not, because Ramble compares names case-insensitively.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{requiresReplaceUnlessCaseOnly()},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Either `pack` or `job`. Changing it replaces the resource.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf("pack", "job")},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"namespace": schema.StringAttribute{
				MarkdownDescription: "Username or organisation that owns the resource. Changing it replaces the resource; a change of letter case only does not, because Ramble compares namespaces case-insensitively.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{requiresReplaceUnlessCaseOnly()},
			},
			"repository_url": schema.StringAttribute{
				MarkdownDescription: "Git repository the content is fetched from. Ramble accepts `https` URLs on its allowed git hosts and answers any other value with an error.",
				Required:            true,
			},
			"file_path": schema.StringAttribute{
				MarkdownDescription: "Path of the pack or job inside the repository. Empty by default.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Short description. Empty by default.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"license": schema.StringAttribute{
				MarkdownDescription: "SPDX licence identifier. When omitted Ramble detects the licence from the repository, or records `Unknown`. " +
					"Removing the attribute later keeps the current value; set it explicitly to change it.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"tags": schema.SetAttribute{
				MarkdownDescription: "Tags. Each tag must be lowercase, 2-20 characters of `a-z`, `0-9` and `-`. Empty by default.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Default:             setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				Validators:          []validator.Set{setvalidator.ValueStringsAre(tagValidator{})},
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "The version to publish, 1-64 characters. On create it is the initial version. " +
					"Changing it later adds that version to the resource; the resource is not replaced and older versions stay. " +
					"Setting a version that already exists is accepted. " +
					"The value is never overwritten from Ramble, so versions added by webhooks do not cause a difference; " +
					"see `latest_version` for what Ramble currently reports. After an import it is set to `latest_version`.",
				Required:   true,
				Validators: []validator.String{versionValidator{}},
			},
			"latest_version": schema.StringAttribute{
				MarkdownDescription: "The latest version Ramble knows of, including versions added by webhooks.",
				Computed:            true,
			},
			"webhook_url": schema.StringAttribute{
				MarkdownDescription: "URL the repository calls to publish new versions.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"webhook_secret": schema.StringAttribute{
				MarkdownDescription: "Secret for the webhook. Ramble returns it only when the resource is created, so it is `null` " +
					"for an imported resource. To get a new secret, replace the resource (`terraform apply -replace`).",
				Computed:      true,
				Sensitive:     true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation time, RFC 3339.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *ResourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func optionalString(s types.String) *string {
	if s.IsNull() || s.IsUnknown() || s.ValueString() == "" {
		return nil
	}
	v := s.ValueString()
	return &v
}

func (r *ResourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tags := make([]string, 0, len(plan.Tags.Elements()))
	resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := rambleapi.HandlersCreateResourceRequest{
		Name:          plan.Name.ValueString(),
		Namespace:     plan.Namespace.ValueString(),
		Type:          rambleapi.HandlersCreateResourceRequestType(plan.Type.ValueString()),
		RepositoryUrl: plan.RepositoryURL.ValueString(),
		Version:       plan.Version.ValueString(),
		Description:   optionalString(plan.Description),
		FilePath:      optionalString(plan.FilePath),
		License:       optionalString(plan.License),
		Tags:          &tags,
	}
	res, err := r.client.CreateResourceWithResponse(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create the resource", "The request to Ramble failed: "+err.Error())
		return
	}
	if res.StatusCode() != http.StatusCreated || res.JSON201 == nil {
		resp.Diagnostics.Append(apiErrorDiag("Unable to create the resource", res.StatusCode(), res.Body))
		return
	}

	plan.WebhookSecret = types.StringValue(res.JSON201.WebhookSecret)
	resp.Diagnostics.Append(applyResource(ctx, &plan, fromCreateResponse(*res.JSON201))...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ResourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseStateID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid resource id in state", err.Error())
		return
	}

	res, err := r.client.GetResourceWithResponse(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the resource", "The request to Ramble failed: "+err.Error())
		return
	}
	if res.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
		resp.Diagnostics.Append(apiErrorDiag("Unable to read the resource", res.StatusCode(), res.Body))
		return
	}

	resp.Diagnostics.Append(applyResource(ctx, &state, *res.JSON200)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ResourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseStateID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid resource id in state", err.Error())
		return
	}

	// Only changed fields are sent. Tags always go as the full set.
	var patch rambleapi.HandlersUpdateResourceRequest
	changed := false
	if !plan.RepositoryURL.Equal(state.RepositoryURL) {
		v := plan.RepositoryURL.ValueString()
		patch.RepositoryUrl, changed = &v, true
	}
	if !plan.Description.Equal(state.Description) {
		v := plan.Description.ValueString()
		patch.Description, changed = &v, true
	}
	if !plan.FilePath.Equal(state.FilePath) {
		v := plan.FilePath.ValueString()
		patch.FilePath, changed = &v, true
	}
	if !plan.License.Equal(state.License) {
		v := plan.License.ValueString()
		patch.License, changed = &v, true
	}
	if !plan.Tags.Equal(state.Tags) {
		tags := make([]string, 0, len(plan.Tags.Elements()))
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		patch.Tags, changed = &tags, true
	}

	patched := false
	var patchedRes *rambleapi.HandlersResourceResponse
	if changed {
		res, err := r.client.UpdateResourceWithResponse(ctx, id, patch)
		if err != nil {
			resp.Diagnostics.AddError("Unable to update the resource", "The request to Ramble failed: "+err.Error())
			return
		}
		if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
			resp.Diagnostics.Append(apiErrorDiag("Unable to update the resource", res.StatusCode(), res.Body))
			return
		}
		patched, patchedRes = true, res.JSON200
	}

	if !plan.Version.Equal(state.Version) {
		res, err := r.client.CreateResourceVersionWithResponse(ctx, id, rambleapi.HandlersCreateVersionRequest{Version: plan.Version.ValueString()})
		switch {
		case err != nil:
			resp.Diagnostics.AddError("Unable to add the version", "The request to Ramble failed: "+err.Error())
		case res.StatusCode() == http.StatusAccepted, isDuplicateVersion(res.StatusCode(), res.Body):
			// The version now exists, newly added or already there.
		default:
			resp.Diagnostics.Append(apiErrorDiag("Unable to add the version", res.StatusCode(), res.Body))
		}
		if resp.Diagnostics.HasError() {
			// Keep state truthful: the PATCH was applied, the version was
			// not, so the next plan retries only the version.
			if patched {
				partial := plan
				partial.Version = state.Version
				resp.Diagnostics.Append(applyResource(ctx, &partial, *patchedRes)...)
				resp.Diagnostics.Append(resp.State.Set(ctx, &partial)...)
			}
			return
		}
	}

	res, err := r.client.GetResourceWithResponse(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the resource", "The request to Ramble failed: "+err.Error())
		return
	}
	if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
		resp.Diagnostics.Append(apiErrorDiag("Unable to read the resource", res.StatusCode(), res.Body))
		return
	}
	// The webhook secret is never returned: keep what state holds.
	plan.WebhookSecret = state.WebhookSecret
	resp.Diagnostics.Append(applyResource(ctx, &plan, *res.JSON200)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ResourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseStateID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid resource id in state", err.Error())
		return
	}

	res, err := r.client.DeleteResourceWithResponse(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete the resource", "The request to Ramble failed: "+err.Error())
		return
	}
	if res.StatusCode() != http.StatusNoContent && res.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.Append(apiErrorDiag("Unable to delete the resource", res.StatusCode(), res.Body))
	}
}

func (r *ResourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	namespace, name, id, err := parseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import id", err.Error())
		return
	}
	if id == 0 {
		res, err := r.client.GetResourceByNameWithResponse(ctx, namespace, name)
		if err != nil {
			resp.Diagnostics.AddError("Unable to import the resource", "The request to Ramble failed: "+err.Error())
			return
		}
		if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
			resp.Diagnostics.Append(apiErrorDiag("Unable to import the resource", res.StatusCode(), res.Body))
			return
		}
		id = res.JSON200.Id
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.Itoa(id))...)
}

// requiresReplaceUnlessCaseOnly replaces the resource when the value changes,
// except for a change of letter case: Ramble resolves names and namespaces
// case-insensitively, so "Alice/MySQL" and "alice/mysql" are one resource.
func requiresReplaceUnlessCaseOnly() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = !strings.EqualFold(req.StateValue.ValueString(), req.PlanValue.ValueString())
		},
		"Changing the value (other than its letter case) replaces the resource.",
		"Changing the value (other than its letter case) replaces the resource.",
	)
}
