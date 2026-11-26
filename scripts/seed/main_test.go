package main

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
)

var tokenRegex = regexp.MustCompile(`^rmbl_[0-9A-Za-z]{40}$`)

func TestNewToken(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		tok, err := newToken()
		if err != nil {
			t.Fatal(err)
		}
		if !tokenRegex.MatchString(tok) {
			t.Fatalf("token %q does not match the Ramble token format", tok)
		}
		if seen[tok] {
			t.Fatal("duplicate token")
		}
		seen[tok] = true
	}
}

func TestHashAndPrefix(t *testing.T) {
	const tok = "rmbl_0123456789012345678901234567890123456789"
	sum := sha256.Sum256([]byte(tok))
	if got, want := hashToken(tok), hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("hash = %s, want %s", got, want)
	}
	if got := tokenPrefix(tok); got != "rmbl_0123456" || len(got) != 12 {
		t.Fatalf("prefix = %q", got)
	}
}

func TestEnvFile(t *testing.T) {
	got := envFile("http://127.0.0.1:3999", "rmbl_x", "tfacc-member")
	for _, want := range []string{"export RAMBLE_ADDR='http://127.0.0.1:3999'\n", "export RAMBLE_TOKEN='rmbl_x'\n", "export RAMBLE_TEST_MEMBER='tfacc-member'\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("env file %q lacks %q", got, want)
		}
	}
}
