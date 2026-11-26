package provider

import (
	"context"
	"net/url"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

const (
	defaultAddress = "https://ramble.openwander.org"
	envAddress     = "RAMBLE_ADDR"
	envToken       = "RAMBLE_TOKEN"
)

// Ensure RambleProvider satisfies the provider interface.
var _ provider.Provider = &RambleProvider{}

// RambleProvider defines the provider implementation.
type RambleProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// RambleProviderModel describes the provider data model.
type RambleProviderModel struct {
	Address types.String `tfsdk:"address"`
	Token   types.String `tfsdk:"token"`
}

func (p *RambleProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ramble"
	resp.Version = p.version
}

func (p *RambleProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage resources on a [Ramble](https://ramble.openwander.org) server, a registry for Nomad packs and jobs.",
		Attributes: map[string]schema.Attribute{
			"address": schema.StringAttribute{
				MarkdownDescription: "Base URL of the Ramble server. May also be set with the `" + envAddress + "` environment variable. Defaults to `" + defaultAddress + "`. Must be `https`; plain `http` is accepted only for `localhost` and loopback addresses.",
				Optional:            true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Ramble API token (`rmbl_...`) with write scope. May also be set with the `" + envToken + "` environment variable. Required: the provider reports an error when it is configured without a token.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

// resolve returns the configured value, else the environment variable envKey,
// else def.
func resolve(cfg types.String, envKey, def string) string {
	if !cfg.IsNull() && cfg.ValueString() != "" {
		return cfg.ValueString()
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return def
}

// redactURL hides any password in raw so an address can be named in a
// diagnostic.
func redactURL(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Redacted()
	}
	return "(unparsable address)"
}

func (p *RambleProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data RambleProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Values that are not known yet (for example computed by another
	// resource) cannot configure a client, so they are an error rather than
	// a deferred configuration.
	if data.Address.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("address"),
			"Unknown Ramble address",
			"The provider cannot create the Ramble client as there is an unknown configuration value for the address. "+
				"Set the value statically in the configuration or use the "+envAddress+" environment variable.",
		)
	}
	if data.Token.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Unknown Ramble API token",
			"The provider cannot create the Ramble client as there is an unknown configuration value for the API token. "+
				"Set the value statically in the configuration or use the "+envToken+" environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	address := resolve(data.Address, envAddress, defaultAddress)
	token := resolve(data.Token, envToken, "")
	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Missing Ramble API token",
			"The provider needs an API token to talk to Ramble. Set the token attribute in the provider "+
				"configuration or the "+envToken+" environment variable.",
		)
		return
	}

	client, err := rambleapi.New(address, token, "terraform-provider-ramble/"+p.version)
	if err != nil {
		// The error never contains the token, but is not echoed in full
		// either: it names the address and the reason only.
		resp.Diagnostics.AddAttributeError(
			path.Root("address"),
			"Invalid Ramble address",
			"Cannot create a Ramble client for "+redactURL(address)+": "+err.Error(),
		)
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *RambleProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}

func (p *RambleProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &RambleProvider{
			version: version,
		}
	}
}
