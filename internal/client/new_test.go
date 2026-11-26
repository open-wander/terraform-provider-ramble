package rambleapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testToken = "rmbl_0123456789012345678901234567890123456789"

func TestTokenURLAllowed(t *testing.T) {
	tests := []struct {
		url     string
		allowed bool
	}{
		{"https://ramble.openwander.org", true},
		{"https://example.com:8443", true},
		{"http://localhost:3000", true},
		{"http://127.0.0.1:3000", true},
		{"http://127.1.2.3", true},
		{"http://[::1]:3000", true},
		{"http://example.com", false},
		{"http://localhost.evil.com", false},
		{"http://10.0.0.1", false},
		{"http://0.0.0.0", false},
		{"http://localhost@evil.com", false},
		{"https://user:pass@example.com", false},
		{"http://user:pass@localhost", false},
		{"ftp://example.com", false},
		{"https://", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := TokenURLAllowed(tt.url)
			if (err == nil) != tt.allowed {
				t.Fatalf("TokenURLAllowed(%q) = %v, want allowed=%v", tt.url, err, tt.allowed)
			}
		})
	}
}

func TestNewRefusals(t *testing.T) {
	tests := []struct {
		name, address, token string
	}{
		{"empty token", "https://example.com", ""},
		{"http remote", "http://example.com", testToken},
		{"empty address", "", testToken},
		{"userinfo", "https://user:pass@example.com", testToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.address, tt.token, "test")
			if err == nil || c != nil {
				t.Fatalf("New(%q) = %v, %v; want an error", tt.address, c, err)
			}
			if strings.Contains(err.Error(), testToken) {
				t.Fatalf("error contains the token: %v", err)
			}
		})
	}
}

func TestNewAccepts(t *testing.T) {
	for _, addr := range []string{
		"http://127.0.0.1:3000", "http://[::1]:3000", "http://localhost:3000",
		"https://ramble.openwander.org/", "ramble.openwander.org",
	} {
		t.Run(addr, func(t *testing.T) {
			if _, err := New(addr, testToken, "test"); err != nil {
				t.Fatalf("New(%q): %v", addr, err)
			}
		})
	}
}

func TestNewBareHostGetsHTTPS(t *testing.T) {
	c, err := New("ramble.openwander.org/", testToken, "test")
	if err != nil {
		t.Fatal(err)
	}
	cl, ok := c.ClientInterface.(*Client)
	if !ok {
		t.Fatalf("unexpected client type %T", c.ClientInterface)
	}
	if strings.TrimSuffix(cl.Server, "/") != "https://ramble.openwander.org" {
		t.Fatalf("server = %q", cl.Server)
	}
}

func TestNewSendsHeaders(t *testing.T) {
	var gotAuth, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotUA = r.Header.Get("Authorization"), r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"u","token_scope":"write","token_org":null,"token_expires_at":"2030-01-01T00:00:00Z"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, testToken, "terraform-provider-ramble/1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.GetCurrentUserWithResponse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.JSON200 == nil || resp.JSON200.Username != "u" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if gotAuth != "Bearer "+testToken {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotUA != "terraform-provider-ramble/1.2.3" {
		t.Fatalf("User-Agent = %q", gotUA)
	}
}

func TestNewDoesNotFollowRedirects(t *testing.T) {
	var secondHits atomic.Int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits.Add(1)
	}))
	defer second.Close()

	var firstAuth string
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstAuth = r.Header.Get("Authorization")
		http.Redirect(w, r, second.URL+"/v1/user", http.StatusFound)
	}))
	defer first.Close()

	c, err := New(first.URL, testToken, "test")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.GetCurrentUserWithResponse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode() != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode())
	}
	if n := secondHits.Load(); n != 0 {
		t.Fatalf("redirect target was hit %d times", n)
	}
	if firstAuth != "Bearer "+testToken {
		t.Fatalf("first server Authorization = %q", firstAuth)
	}
}

func TestNewBoundsResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("a", 2*maxResponseBody)))
	}))
	defer srv.Close()

	c, err := New(srv.URL, testToken, "test")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.GetCurrentUserWithResponse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Body) != maxResponseBody {
		t.Fatalf("body length = %d, want %d", len(resp.Body), maxResponseBody)
	}
}
