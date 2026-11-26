package provider

import (
	"strings"
	"testing"
)

func TestAPIErrorDiag(t *testing.T) {
	long := strings.Repeat("x", 500)
	tests := []struct {
		name      string
		status    int
		body      string
		want      []string
		wantNot   []string
		maxDetail int
	}{
		{
			name: "code and message", status: 422,
			body: `{"error":{"code":"invalid_tags","message":"tag \"Db\" must be lowercase"}}`,
			want: []string{"Ramble answered 422 invalid_tags: tag \"Db\" must be lowercase"},
		},
		{
			name: "401 hint", status: 401,
			body: `{"error":{"code":"unauthorized","message":"invalid token"}}`,
			want: []string{"Ramble answered 401 unauthorized: invalid token", "RAMBLE_TOKEN"},
		},
		{
			name: "403 hint", status: 403,
			body: `{"error":{"code":"forbidden","message":"no"}}`,
			want: []string{"write scope", "organisation"},
		},
		{
			name: "409 hint", status: 409,
			body:    `{"error":{"code":"conflict","message":"exists"}}`,
			want:    []string{"already exists or is in a conflicting state"},
			wantNot: []string{"ramble_resource"},
		},
		{
			name: "org_not_empty hint", status: 409,
			body: `{"error":{"code":"org_not_empty","message":"organization still has 3 resources"}}`,
			want: []string{"organization still has 3 resources", "Delete or move", "ramble_resource.namespace = ramble_organization"},
		},
		{
			name: "last_owner hint", status: 409,
			body:    `{"error":{"code":"last_owner","message":"cannot remove the last owner"}}`,
			want:    []string{"must always keep an owner", "make another member owner first"},
			wantNot: []string{"terraform import"},
		},
		{
			name: "invalid_role hint", status: 422,
			body: `{"error":{"code":"invalid_role","message":"role must be owner or member"}}`,
			want: []string{"owner", "member", "role"},
		},
		{
			name: "invalid_name hint", status: 422,
			body: `{"error":{"code":"invalid_name","message":"organization name is reserved"}}`,
			want: []string{"1-64", "reserved"},
		},
		{
			name: "429 hint", status: 429,
			body: `{"error":{"code":"rate_limited","message":"slow down"}}`,
			want: []string{"rate limit"},
		},
		{
			name: "no hint on 404", status: 404,
			body:    `{"error":{"code":"not_found","message":"pack not found"}}`,
			want:    []string{"Ramble answered 404 not_found: pack not found"},
			wantNot: []string{"RAMBLE_TOKEN", "write scope", "import"},
		},
		{
			name: "non-JSON body is not echoed", status: 502,
			body:    `<html>secret-internal-page</html>`,
			want:    []string{"Ramble answered 502 Bad Gateway"},
			wantNot: []string{"secret-internal-page"},
		},
		{
			name: "empty body", status: 500, body: ``,
			want: []string{"Ramble answered 500 Internal Server Error"},
		},
		{
			name: "long message is truncated", status: 422,
			body:      `{"error":{"code":"bad","message":"` + long + `"}}`,
			want:      []string{"..."},
			wantNot:   []string{long},
			maxDetail: 400,
		},
		{
			name: "token in a message is redacted", status: 400,
			body:    `{"error":{"code":"bad_request","message":"got ` + testToken + ` here"}}`,
			wantNot: []string{testToken},
			want:    []string{"[redacted]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := apiErrorDiag("Unable to do a thing", tt.status, []byte(tt.body))
			if d.Summary() != "Unable to do a thing" {
				t.Fatalf("summary = %q", d.Summary())
			}
			for _, w := range tt.want {
				if !strings.Contains(d.Detail(), w) {
					t.Errorf("detail %q does not contain %q", d.Detail(), w)
				}
			}
			for _, w := range tt.wantNot {
				if strings.Contains(d.Detail(), w) {
					t.Errorf("detail %q contains %q", d.Detail(), w)
				}
			}
			if tt.maxDetail > 0 && len(d.Detail()) > tt.maxDetail {
				t.Errorf("detail is %d bytes, want at most %d", len(d.Detail()), tt.maxDetail)
			}
		})
	}
}

func TestIsDuplicateVersion(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"duplicate", 409, `{"error":{"code":"conflict","message":"version already exists"}}`, true},
		{"other conflict", 409, `{"error":{"code":"conflict","message":"version is not failed"}}`, false},
		{"wrong status", 422, `{"error":{"code":"conflict","message":"version already exists"}}`, false},
		{"not JSON", 409, `nope`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDuplicateVersion(tt.status, []byte(tt.body)); got != tt.want {
				t.Fatalf("isDuplicateVersion = %v, want %v", got, tt.want)
			}
		})
	}
}
