package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

// resourceModel is the state of a ramble_resource.
type resourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Type          types.String `tfsdk:"type"`
	Namespace     types.String `tfsdk:"namespace"`
	RepositoryURL types.String `tfsdk:"repository_url"`
	FilePath      types.String `tfsdk:"file_path"`
	Description   types.String `tfsdk:"description"`
	License       types.String `tfsdk:"license"`
	Tags          types.Set    `tfsdk:"tags"`
	Version       types.String `tfsdk:"version"`
	LatestVersion types.String `tfsdk:"latest_version"`
	WebhookURL    types.String `tfsdk:"webhook_url"`
	WebhookSecret types.String `tfsdk:"webhook_secret"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

// applyResource copies an API resource into m. It is shared by create, read,
// update, import and the data source.
//
//   - name and namespace keep the configured spelling when Ramble returns the
//     same name in a different case, since Ramble matches them
//     case-insensitively.
//   - version is the version the configuration asks for, so it is kept; only
//     when it is not set yet (after import) does it take latest_version.
//   - webhook_secret is never returned by the API: a known value is kept and
//     an unknown one becomes null.
func applyResource(_ context.Context, m *resourceModel, r rambleapi.HandlersResourceResponse) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ID = types.StringValue(strconv.Itoa(r.Id))
	m.Name = keepSpelling(m.Name, r.Name)
	m.Namespace = keepSpelling(m.Namespace, r.Namespace)
	m.Type = types.StringValue(string(r.Type))
	m.RepositoryURL = types.StringValue(r.RepositoryUrl)
	m.FilePath = types.StringValue(r.FilePath)
	m.Description = types.StringValue(r.Description)
	m.License = types.StringValue(r.License)

	elems := make([]attr.Value, 0, len(r.Tags))
	for _, t := range r.Tags {
		elems = append(elems, types.StringValue(t))
	}
	tags, d := types.SetValue(types.StringType, elems)
	diags.Append(d...)
	m.Tags = tags

	m.LatestVersion = types.StringValue(r.LatestVersion)
	if m.Version.IsNull() || m.Version.IsUnknown() {
		m.Version = types.StringValue(r.LatestVersion)
	}
	m.WebhookURL = types.StringValue(r.WebhookUrl)
	m.CreatedAt = types.StringValue(r.CreatedAt.UTC().Format(time.RFC3339Nano))
	if m.WebhookSecret.IsUnknown() {
		m.WebhookSecret = types.StringNull()
	}
	return diags
}

func keepSpelling(current types.String, api string) types.String {
	if !current.IsNull() && !current.IsUnknown() && strings.EqualFold(current.ValueString(), api) {
		return current
	}
	return types.StringValue(api)
}

// fromCreateResponse views a create response as a plain resource response.
func fromCreateResponse(c rambleapi.HandlersCreateResourceResponse) rambleapi.HandlersResourceResponse {
	return rambleapi.HandlersResourceResponse{
		CreatedAt:     c.CreatedAt,
		Description:   c.Description,
		FilePath:      c.FilePath,
		Id:            c.Id,
		LatestVersion: c.LatestVersion,
		License:       c.License,
		Name:          c.Name,
		Namespace:     c.Namespace,
		RepositoryUrl: c.RepositoryUrl,
		Tags:          c.Tags,
		Type:          rambleapi.HandlersResourceResponseType(c.Type),
		UpdatedAt:     c.UpdatedAt,
		WebhookUrl:    c.WebhookUrl,
	}
}

// parseStateID parses the id kept in state.
func parseStateID(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%q is not a Ramble resource id", s)
	}
	return id, nil
}

// parseImportID parses an import id: "namespace/name", or a numeric id.
func parseImportID(s string) (namespace, name string, id int, err error) {
	if n, convErr := strconv.Atoi(s); convErr == nil {
		if n <= 0 {
			return "", "", 0, fmt.Errorf("%q is not a valid resource id", s)
		}
		return "", "", n, nil
	}
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", 0, fmt.Errorf("expected an import id of the form namespace/name (for example alice/mysql) or a numeric id, got %q", s)
	}
	return parts[0], parts[1], 0, nil
}
