package provider

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

func TestParseImportID(t *testing.T) {
	tests := []struct {
		in      string
		wantNS  string
		wantN   string
		wantID  int
		wantErr bool
	}{
		{in: "alice/mysql", wantNS: "alice", wantN: "mysql"},
		{in: "42", wantID: 42},
		{in: "", wantErr: true},
		{in: "alice", wantErr: true},
		{in: "alice/", wantErr: true},
		{in: "/mysql", wantErr: true},
		{in: "a/b/c", wantErr: true},
		{in: "0", wantErr: true},
		{in: "-3", wantErr: true},
		{in: "99999999999999999999", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			ns, n, id, err := parseImportID(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if ns != tt.wantNS || n != tt.wantN || id != tt.wantID {
				t.Fatalf("got (%q, %q, %d), want (%q, %q, %d)", ns, n, id, tt.wantNS, tt.wantN, tt.wantID)
			}
		})
	}
}

func TestParseStateID(t *testing.T) {
	tests := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"42", 42, false},
		{"", 0, true},
		{"abc", 0, true},
		{"0", 0, true},
		{"-1", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseStateID(tt.in)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("got (%d, %v), want (%d, err=%v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func apiResource() rambleapi.HandlersResourceResponse {
	return rambleapi.HandlersResourceResponse{
		Id:            42,
		Name:          "mysql",
		Namespace:     "alice",
		Type:          "pack",
		RepositoryUrl: "https://github.com/alice/packs",
		LatestVersion: "v1.2.0",
		License:       "MIT",
		WebhookUrl:    "http://ramble.test/resource/42/webhook",
		CreatedAt:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func TestApplyResource(t *testing.T) {
	ctx := context.Background()
	empty := types.SetValueMust(types.StringType, []attr.Value{})

	t.Run("fresh import takes everything from the API", func(t *testing.T) {
		m := resourceModel{Tags: types.SetNull(types.StringType)}
		if d := applyResource(ctx, &m, apiResource()); d.HasError() {
			t.Fatal(d)
		}
		if m.ID.ValueString() != "42" || m.Name.ValueString() != "mysql" || m.Namespace.ValueString() != "alice" {
			t.Fatalf("identity not applied: %+v", m)
		}
		if m.Version.ValueString() != "v1.2.0" {
			t.Fatalf("version = %q, want latest_version when null", m.Version.ValueString())
		}
		if !m.WebhookSecret.IsNull() {
			t.Fatal("webhook_secret must be null after import")
		}
		if m.CreatedAt.ValueString() != "2026-01-02T03:04:05Z" {
			t.Fatalf("created_at = %q", m.CreatedAt.ValueString())
		}
		if m.Tags.IsNull() || len(m.Tags.Elements()) != 0 {
			t.Fatalf("nil tags must map to the empty set, got %v", m.Tags)
		}
	})

	t.Run("configured version and case are kept", func(t *testing.T) {
		m := resourceModel{
			Name:          types.StringValue("MySQL"),
			Namespace:     types.StringValue("Alice"),
			Version:       types.StringValue("v1.0.0"),
			WebhookSecret: types.StringValue("s3cret"),
			Tags:          empty,
		}
		if d := applyResource(ctx, &m, apiResource()); d.HasError() {
			t.Fatal(d)
		}
		if m.Name.ValueString() != "MySQL" || m.Namespace.ValueString() != "Alice" {
			t.Fatalf("case-insensitive match must keep the configured spelling: %s/%s", m.Namespace, m.Name)
		}
		if m.Version.ValueString() != "v1.0.0" {
			t.Fatalf("version = %q, must not follow latest_version", m.Version.ValueString())
		}
		if m.LatestVersion.ValueString() != "v1.2.0" {
			t.Fatalf("latest_version = %q", m.LatestVersion.ValueString())
		}
		if m.WebhookSecret.ValueString() != "s3cret" {
			t.Fatal("webhook_secret must be kept")
		}
	})

	t.Run("a different name is taken from the API", func(t *testing.T) {
		m := resourceModel{Name: types.StringValue("other"), Namespace: types.StringValue("bob"), Tags: empty}
		if d := applyResource(ctx, &m, apiResource()); d.HasError() {
			t.Fatal(d)
		}
		if m.Name.ValueString() != "mysql" || m.Namespace.ValueString() != "alice" {
			t.Fatalf("got %s/%s", m.Namespace, m.Name)
		}
	})

	t.Run("unknown secret becomes null", func(t *testing.T) {
		m := resourceModel{WebhookSecret: types.StringUnknown(), Tags: empty}
		if d := applyResource(ctx, &m, apiResource()); d.HasError() {
			t.Fatal(d)
		}
		if !m.WebhookSecret.IsNull() {
			t.Fatal("unknown webhook_secret must resolve to null")
		}
	})

	t.Run("tags are mapped", func(t *testing.T) {
		r := apiResource()
		r.Tags = []string{"sql", "database"}
		m := resourceModel{Tags: empty}
		if d := applyResource(ctx, &m, r); d.HasError() {
			t.Fatal(d)
		}
		if len(m.Tags.Elements()) != 2 {
			t.Fatalf("tags = %v", m.Tags)
		}
	})
}

func TestRequiresReplaceUnlessCaseOnly(t *testing.T) {
	tests := []struct {
		state, plan string
		want        bool
	}{
		{"alice", "alice", false},
		{"alice", "Alice", false},
		{"TFACC", "tfacc", false},
		{"alice", "bob", true},
		{"mysql", "mysql2", true},
	}
	for _, tt := range tests {
		req := planmodifier.StringRequest{
			StateValue: types.StringValue(tt.state),
			PlanValue:  types.StringValue(tt.plan),
			State:      tfsdk.State{Raw: tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{})},
			Plan:       tfsdk.Plan{Raw: tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{})},
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		requiresReplaceUnlessCaseOnly().PlanModifyString(context.Background(), req, resp)
		if resp.RequiresReplace != tt.want {
			t.Errorf("%q -> %q: RequiresReplace = %v, want %v", tt.state, tt.plan, resp.RequiresReplace, tt.want)
		}
	}
}
