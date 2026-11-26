package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

// testAccProtoV6ProviderFactories is used to instantiate a provider during acceptance testing.
// The factory function is called for each Terraform CLI command to create a provider
// server that the CLI can connect to and interact with.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"ramble": providerserver.NewProtocol6WithError(New("test")()),
}

const testToken = "rmbl_0123456789012345678901234567890123456789"

func TestResolve(t *testing.T) {
	const envKey = "RAMBLE_TEST_RESOLVE"
	tests := []struct {
		name string
		cfg  types.String
		env  string
		def  string
		want string
	}{
		{"config wins over env and default", types.StringValue("cfg"), "env", "def", "cfg"},
		{"env wins over default", types.StringNull(), "env", "def", "env"},
		{"default when nothing else", types.StringNull(), "", "def", "def"},
		{"nothing set", types.StringNull(), "", "", ""},
		{"empty config falls through to env", types.StringValue(""), "env", "def", "env"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envKey, tt.env)
			if got := resolve(tt.cfg, envKey, tt.def); got != tt.want {
				t.Fatalf("resolve = %q, want %q", got, tt.want)
			}
		})
	}
}

func configValue(t *testing.T, address, token tftypes.Value) tfsdk.Config {
	t.Helper()
	p := New("test")()
	var sresp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &sresp)
	if sresp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", sresp.Diagnostics)
	}
	obj := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"address": tftypes.String,
		"token":   tftypes.String,
	}}
	return tfsdk.Config{
		Schema: sresp.Schema,
		Raw:    tftypes.NewValue(obj, map[string]tftypes.Value{"address": address, "token": token}),
	}
}

func TestConfigure(t *testing.T) {
	str := func(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
	null := tftypes.NewValue(tftypes.String, nil)
	unknown := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

	tests := []struct {
		name      string
		address   tftypes.Value
		token     tftypes.Value
		envAddr   string
		envToken  string
		wantError string // substring of the error summary or detail; empty means success
	}{
		{name: "config values", address: str("http://127.0.0.1:3000"), token: str(testToken)},
		{name: "env values", address: null, token: null, envAddr: "http://localhost:3000", envToken: testToken},
		{name: "default address", address: null, token: str(testToken)},
		{name: "missing token", address: str("https://ramble.example.com"), token: null, wantError: "Missing Ramble API token"},
		{name: "unknown token", address: str("https://ramble.example.com"), token: unknown, wantError: "Unknown Ramble API token"},
		{name: "unknown address", address: unknown, token: str(testToken), wantError: "Unknown Ramble address"},
		{name: "http remote address", address: str("http://ramble.example.com"), token: str(testToken), wantError: "Invalid Ramble address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RAMBLE_ADDR", tt.envAddr)
			t.Setenv("RAMBLE_TOKEN", tt.envToken)

			p := New("1.2.3")()
			req := provider.ConfigureRequest{Config: configValue(t, tt.address, tt.token)}
			var resp provider.ConfigureResponse
			p.Configure(context.Background(), req, &resp)

			for _, d := range resp.Diagnostics {
				if strings.Contains(d.Summary()+d.Detail(), testToken) {
					t.Fatalf("diagnostic contains the token: %s", d.Summary())
				}
			}
			if tt.wantError == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
				}
				c, ok := resp.ResourceData.(*rambleapi.ClientWithResponses)
				if !ok || c == nil {
					t.Fatalf("ResourceData = %T, want *ClientWithResponses", resp.ResourceData)
				}
				if resp.DataSourceData != resp.ResourceData {
					t.Fatal("DataSourceData differs from ResourceData")
				}
				return
			}
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error containing %q, got none", tt.wantError)
			}
			var found bool
			for _, d := range resp.Diagnostics.Errors() {
				if strings.Contains(d.Summary(), tt.wantError) {
					found = true
				}
			}
			if !found {
				t.Fatalf("no error summary contains %q: %v", tt.wantError, resp.Diagnostics)
			}
			if resp.ResourceData != nil {
				t.Fatal("ResourceData set despite error")
			}
		})
	}
}

func TestSchemaTokenIsSensitive(t *testing.T) {
	var resp provider.SchemaResponse
	New("test")().Schema(context.Background(), provider.SchemaRequest{}, &resp)
	attr, ok := resp.Schema.Attributes["token"]
	if !ok || !attr.IsSensitive() {
		t.Fatal("token attribute must exist and be Sensitive")
	}
}
