package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

var (
	_ datasource.DataSource              = &ResourceDataSource{}
	_ datasource.DataSourceWithConfigure = &ResourceDataSource{}
)

// NewResourceDataSource returns the ramble_resource data source.
func NewResourceDataSource() datasource.DataSource {
	return &ResourceDataSource{}
}

// ResourceDataSource looks up a pack or job by namespace and name.
type ResourceDataSource struct {
	client *rambleapi.ClientWithResponses
}

type resourceDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Type          types.String `tfsdk:"type"`
	Namespace     types.String `tfsdk:"namespace"`
	RepositoryURL types.String `tfsdk:"repository_url"`
	FilePath      types.String `tfsdk:"file_path"`
	Description   types.String `tfsdk:"description"`
	License       types.String `tfsdk:"license"`
	Tags          types.Set    `tfsdk:"tags"`
	LatestVersion types.String `tfsdk:"latest_version"`
	WebhookURL    types.String `tfsdk:"webhook_url"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

func (d *ResourceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource"
}

func (d *ResourceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	str := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{MarkdownDescription: desc, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a pack or job in a Ramble registry by namespace and name.",
		Attributes: map[string]schema.Attribute{
			"namespace":      schema.StringAttribute{MarkdownDescription: "Username or organisation that owns the resource.", Required: true},
			"name":           schema.StringAttribute{MarkdownDescription: "Name of the pack or job.", Required: true},
			"id":             str("Ramble's numeric id of the resource."),
			"type":           str("Either `pack` or `job`."),
			"repository_url": str("Git repository the content is fetched from."),
			"file_path":      str("Path of the pack or job inside the repository."),
			"description":    str("Short description."),
			"license":        str("Licence identifier."),
			"tags": schema.SetAttribute{
				MarkdownDescription: "Tags.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"latest_version": str("The latest version Ramble knows of."),
			"webhook_url":    str("URL the repository calls to publish new versions."),
			"created_at":     str("Creation time, RFC 3339."),
		},
	}
}

func (d *ResourceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*rambleapi.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("Expected *ClientWithResponses, got %T. This is a provider bug.", req.ProviderData))
		return
	}
	d.client = c
}

func (d *ResourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg resourceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := d.client.GetResourceByNameWithResponse(ctx, cfg.Namespace.ValueString(), cfg.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the resource", "The request to Ramble failed: "+err.Error())
		return
	}
	if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
		resp.Diagnostics.Append(apiErrorDiag("Unable to read the resource", res.StatusCode(), res.Body))
		return
	}

	m := resourceModel{Name: cfg.Name, Namespace: cfg.Namespace}
	resp.Diagnostics.Append(applyResource(ctx, &m, *res.JSON200)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out := resourceDataSourceModel{
		ID: m.ID, Name: m.Name, Type: m.Type, Namespace: m.Namespace,
		RepositoryURL: m.RepositoryURL, FilePath: m.FilePath, Description: m.Description,
		License: m.License, Tags: m.Tags, LatestVersion: m.LatestVersion,
		WebhookURL: m.WebhookURL, CreatedAt: m.CreatedAt,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}
