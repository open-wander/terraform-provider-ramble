package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func runStringValidator(t *testing.T, v validator.String, val string) string {
	t.Helper()
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("x"),
		ConfigValue: types.StringValue(val),
	}, resp)
	if !resp.Diagnostics.HasError() {
		return ""
	}
	return resp.Diagnostics[0].Summary() + " " + resp.Diagnostics[0].Detail()
}

func TestTagValidator(t *testing.T) {
	tests := []struct {
		tag  string
		want string // substring of the error; empty means valid
	}{
		{"database", ""},
		{"a1", ""},
		{"with-dash", ""},
		{strings.Repeat("a", 20), ""},
		{"a", "2-20"},
		{strings.Repeat("a", 21), "2-20"},
		{"Database", `use "database"`},
		{"Database", "lowercase"},
		{"has space", "a-z, 0-9"},
		{"under_score", "a-z, 0-9"},
		{"", "2-20"},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			got := runStringValidator(t, tagValidator{}, tt.tag)
			if tt.want == "" && got != "" {
				t.Fatalf("unexpected error: %s", got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Fatalf("error %q does not contain %q", got, tt.want)
			}
		})
	}
}

func TestVersionValidator(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"semver", "v1.0.0", ""},
		{"max length", strings.Repeat("v", 64), ""},
		{"empty", "", "1-64"},
		{"too long", strings.Repeat("v", 65), "1-64"},
		// Ramble counts bytes: 33 two-byte characters are 66 bytes.
		{"too many bytes", strings.Repeat("é", 33), "1-64"},
		{"multibyte within limit", strings.Repeat("é", 32), ""},
		{"leading space", " v1", "whitespace"},
		{"trailing newline", "v1\n", "whitespace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runStringValidator(t, versionValidator{}, tt.version)
			if tt.want == "" && got != "" {
				t.Fatalf("unexpected error: %s", got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Fatalf("error %q does not contain %q", got, tt.want)
			}
		})
	}
}

func TestOrgNameValidator(t *testing.T) {
	tests := []struct {
		name string
		want string // substring of the error; empty means valid
	}{
		{"myorg", ""},
		{"MyOrg", ""},
		{"a", ""},
		{"0abc", ""},
		{"with-dash_and.dot", ""},
		{strings.Repeat("a", 64), ""},
		{strings.Repeat("a", 65), "Invalid organisation name"},
		{"", "Invalid organisation name"},
		{".hidden", "Invalid organisation name"},
		{"-dash", "Invalid organisation name"},
		{"_under", "Invalid organisation name"},
		{"has space", "Invalid organisation name"},
		{" padded", "Invalid organisation name"},
		{"a/b", "Invalid organisation name"},
		{"café", "Invalid organisation name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runStringValidator(t, orgNameValidator{}, tt.name)
			if tt.want == "" && got != "" {
				t.Fatalf("unexpected error: %s", got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Fatalf("error %q does not contain %q", got, tt.want)
			}
		})
	}
}

func TestOrgNameValidatorIgnoresUnknownAndNull(t *testing.T) {
	for _, v := range []types.String{types.StringNull(), types.StringUnknown()} {
		resp := &validator.StringResponse{}
		orgNameValidator{}.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("x"), ConfigValue: v}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%v: %v", v, resp.Diagnostics)
		}
	}
}
