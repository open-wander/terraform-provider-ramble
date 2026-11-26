package provider

import (
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"

	rambleapi "github.com/open-wander/terraform-provider-ramble/internal/client"
)

func TestParseMemberImportID(t *testing.T) {
	tests := []struct {
		id        string
		org, user string
		wantErr   bool
	}{
		{"myorg/alice", "myorg", "alice", false},
		{"MyOrg/Alice", "MyOrg", "Alice", false},
		{"myorg", "", "", true},
		{"myorg/", "", "", true},
		{"/alice", "", "", true},
		{"", "", "", true},
		{"a/b/c", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			org, user, err := parseMemberImportID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if got := err.Error(); !containsAll(got, "organization/username", "myorg/alice") {
					t.Fatalf("error %q should show the expected form", got)
				}
				return
			}
			if org != tt.org || user != tt.user {
				t.Fatalf("got %q, %q", org, user)
			}
		})
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestApplyOrganization(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 3600))
	api := rambleapi.HandlersOrgResponse{Name: "MyOrg", Description: "d", CreatedAt: created, MemberCount: 4}

	t.Run("keeps configured spelling, id is the stored name", func(t *testing.T) {
		m := organizationModel{Name: types.StringValue("myorg")}
		applyOrganization(&m, api)
		if m.Name.ValueString() != "myorg" {
			t.Errorf("name = %q, want the configured spelling", m.Name.ValueString())
		}
		if m.ID.ValueString() != "MyOrg" {
			t.Errorf("id = %q, want the stored name", m.ID.ValueString())
		}
		if m.Description.ValueString() != "d" {
			t.Errorf("description = %q", m.Description.ValueString())
		}
		if m.CreatedAt.ValueString() != "2026-01-02T02:04:05Z" {
			t.Errorf("created_at = %q, want RFC 3339 UTC", m.CreatedAt.ValueString())
		}
	})
	t.Run("takes the stored name on import", func(t *testing.T) {
		m := organizationModel{Name: types.StringNull()}
		applyOrganization(&m, api)
		if m.Name.ValueString() != "MyOrg" {
			t.Errorf("name = %q", m.Name.ValueString())
		}
	})
}
