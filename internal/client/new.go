package rambleapi

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxResponseBody caps every response body the generated client reads, so a
// hostile server cannot exhaust memory.
const maxResponseBody = 1 << 20

// TokenURLAllowed reports whether a bearer token may be sent to rawURL: https
// anywhere, plain http only to a loopback host. URLs that carry userinfo are
// refused.
func TokenURLAllowed(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return errors.New("invalid Ramble address")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return fmt.Errorf("refusing to send a token to %s: use https (plain http is allowed only for localhost)", u.Scheme+"://"+u.Host)
}

// tokenHTTPClient returns the only HTTP client that may carry a bearer token:
// TLS 1.2+, a 30 second timeout and no redirect following, so the token cannot
// leave the host it was sent to.
func tokenHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// limitedDoer caps the size of each response body.
type limitedDoer struct {
	hc *http.Client
}

type limitedBody struct {
	io.Reader
	io.Closer
}

func (d limitedDoer) Do(req *http.Request) (*http.Response, error) {
	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body = limitedBody{io.LimitReader(resp.Body, maxResponseBody), resp.Body}
	return resp, nil
}

// New returns a /v1 client for the Ramble server at address that authenticates
// with token. It is the only way the provider builds a client: it refuses an
// empty token and any address the token may not be sent to (plain http beyond
// loopback), and uses an HTTP client that never follows redirects. A bare host
// gets https:// and a trailing slash is dropped. Errors never contain the token.
func New(address, token, userAgent string) (*ClientWithResponses, error) {
	if token == "" {
		return nil, errors.New("no API token")
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, errors.New("no Ramble address")
	}
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = "https://" + address
	}
	address = strings.TrimSuffix(address, "/")
	if err := TokenURLAllowed(address); err != nil {
		return nil, err
	}
	return NewClientWithResponses(address,
		WithHTTPClient(limitedDoer{tokenHTTPClient()}),
		WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("User-Agent", userAgent)
			return nil
		}),
	)
}
