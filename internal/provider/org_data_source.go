package provider

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

var (
	_ datasource.DataSource              = &OrganizationDataSource{}
	_ datasource.DataSourceWithConfigure = &OrganizationDataSource{}
)

// NewOrganizationDataSource returns the ramble_organization data source.
func NewOrganizationDataSource() datasource.DataSource {
	return &OrganizationDataSource{}
}

// OrganizationDataSource looks up an organisation by name.
type OrganizationDataSource struct {
	client *rambleapi.ClientWithResponses
}

type organizationDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	CreatedAt   types.String `tfsdk:"created_at"`
	MemberCount types.Int64  `tfsdk:"member_count"`
	Members     types.List   `tfsdk:"members"`
}

var memberObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"username": types.StringType,
	"role":     types.StringType,
}}

func (d *OrganizationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (d *OrganizationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an organisation in a Ramble registry by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the organisation.",
				Required:            true,
				Validators:          []validator.String{orgNameValidator{}},
			},
			"id":           schema.StringAttribute{MarkdownDescription: "The name of the organisation as Ramble stores it.", Computed: true},
			"description":  schema.StringAttribute{MarkdownDescription: "Short description.", Computed: true},
			"created_at":   schema.StringAttribute{MarkdownDescription: "Creation time, RFC 3339.", Computed: true},
			"member_count": schema.Int64Attribute{MarkdownDescription: "Number of members.", Computed: true},
			"members": schema.ListNestedAttribute{
				MarkdownDescription: "The members, sorted by username. Ramble lists members only to its own members, " +
					"so this is `null` when the token's user is not a member of the organisation.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"username": schema.StringAttribute{MarkdownDescription: "Username of the member.", Computed: true},
						"role":     schema.StringAttribute{MarkdownDescription: "Either `owner` or `member`.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *OrganizationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *OrganizationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg organizationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := cfg.Name.ValueString()

	res, err := d.client.GetOrgWithResponse(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the organisation", "The request to Ramble failed: "+err.Error())
		return
	}
	if res.StatusCode() != http.StatusOK || res.JSON200 == nil {
		resp.Diagnostics.Append(apiErrorDiag("Unable to read the organisation", res.StatusCode(), res.Body))
		return
	}
	o := res.JSON200

	out := organizationDataSourceModel{
		ID:          types.StringValue(o.Name),
		Name:        keepSpelling(cfg.Name, o.Name),
		Description: types.StringValue(o.Description),
		CreatedAt:   types.StringValue(o.CreatedAt.UTC().Format(time.RFC3339Nano)),
		MemberCount: types.Int64Value(int64(o.MemberCount)),
		Members:     types.ListNull(memberObjectType),
	}

	mres, err := d.client.ListOrgMembersWithResponse(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the organisation members", "The request to Ramble failed: "+err.Error())
		return
	}
	switch {
	case mres.StatusCode() == http.StatusForbidden:
		// The token's user is not a member: the list stays null.
	case mres.StatusCode() != http.StatusOK || mres.JSON200 == nil:
		resp.Diagnostics.Append(apiErrorDiag("Unable to read the organisation members", mres.StatusCode(), mres.Body))
		return
	default:
		members := append([]rambleapi.HandlersOrgMemberResponse(nil), *mres.JSON200...)
		sort.SliceStable(members, func(i, j int) bool {
			return strings.ToLower(members[i].Username) < strings.ToLower(members[j].Username)
		})
		elems := make([]attr.Value, 0, len(members))
		for _, m := range members {
			elems = append(elems, types.ObjectValueMust(memberObjectType.AttrTypes, map[string]attr.Value{
				"username": types.StringValue(m.Username),
				"role":     types.StringValue(string(m.Role)),
			}))
		}
		out.Members = types.ListValueMust(memberObjectType, elems)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}
