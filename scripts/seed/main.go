// Command seed creates a verified test user and a write-scope API token
// directly in a Ramble database, for the provider's acceptance tests. It is
// test tooling and is never imported by the provider.
//
// It reads DATABASE_URL, and RAMBLE_ADDR for the address it records, then
// writes RAMBLE_ADDR and RAMBLE_TOKEN as shell exports to the file named by
// -out (mode 0600). It never prints the token.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	tokenLead = "rmbl_"
	randomLen = 40
	prefixLen = 12
	base62    = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	testUsername = "tfacc"
	testEmail    = "tfacc@example.test"
	tokenName    = "tfacc"
)

// newToken returns "rmbl_" plus 40 base62 characters from crypto/rand, with
// rejection sampling to avoid modulo bias (the format Ramble generates).
func newToken() (string, error) {
	var sb strings.Builder
	sb.WriteString(tokenLead)
	buf := make([]byte, 64)
	n := 0
	for n < randomLen {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate token: %w", err)
		}
		for _, b := range buf {
			if b >= 248 { // 248 = 62*4
				continue
			}
			sb.WriteByte(base62[b%62])
			n++
			if n == randomLen {
				break
			}
		}
	}
	return sb.String(), nil
}

// hashToken returns the hex SHA-256 of the whole plaintext token, as Ramble
// stores it.
func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func tokenPrefix(plain string) string { return plain[:prefixLen] }

func envFile(addr, token string) string {
	return fmt.Sprintf("export RAMBLE_ADDR='%s'\nexport RAMBLE_TOKEN='%s'\n", addr, token)
}

func seed(ctx context.Context, db *sql.DB, token string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// A verified local account: API authentication rejects unverified ones.
	var userID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO users (created_at, updated_at, username, email, name, provider, email_verified, is_admin, password_hash)
VALUES (now(), now(), $1, $2, 'Acceptance test user', '', true, false, '')
ON CONFLICT (username) DO UPDATE
SET email_verified = true, is_admin = false, provider = '', deleted_at = NULL, updated_at = now()
RETURNING id`, testUsername, testEmail).Scan(&userID)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM api_tokens WHERE user_id = $1 AND name = $2`, userID, tokenName); err != nil {
		return fmt.Errorf("remove old token: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO api_tokens (user_id, name, prefix, hash, scope, organization_id, expires_at, created_at)
VALUES ($1, $2, $3, $4, 'write', NULL, $5, now())`,
		userID, tokenName, tokenPrefix(token), hashToken(token), time.Now().Add(30*24*time.Hour))
	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}
	return tx.Commit()
}

func main() {
	out := flag.String("out", ".testacc.env", "file to write the RAMBLE_ADDR and RAMBLE_TOKEN exports to")
	flag.Parse()

	dsn, addr := os.Getenv("DATABASE_URL"), os.Getenv("RAMBLE_ADDR")
	if dsn == "" || addr == "" {
		log.Fatal("DATABASE_URL and RAMBLE_ADDR must be set")
	}
	token, err := newToken()
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal("open database: ", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := seed(ctx, db, token); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, []byte(envFile(addr, token)), 0o600); err != nil {
		log.Fatal(err)
	}
	if err := os.Chmod(*out, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("seeded user %q and wrote %s", testUsername, *out)
}
