package ui

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	"testing"
)

// The proxy was migrated from the deprecated ReverseProxy.Director to Rewrite.
// Director made ReverseProxy set X-Forwarded-For automatically; Rewrite does not
// unless SetXForwarded is called, so this pins the forwarding behavior.
func TestUIProxyForwardsToLocalhostWithXForwardedFor(t *testing.T) {
	var (
		gotHost  string
		gotXFF   string
		gotProto string
		gotPath  string
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotXFF = r.Header.Get("X-Forwarded-For")
		gotProto = r.Header.Get("X-Forwarded-Proto")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, portStr, err := net.SplitHostPort(upstreamURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://obot.example.com/some/path", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()

	newUIProxy(port).ServeHTTP(rec, req)

	// Reaching the upstream at all is what proves URL.Host routing works, since
	// the upstream only exists on localhost.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from upstream, got %d", rec.Code)
	}
	if gotPath != "/some/path" {
		t.Errorf("expected the path to be preserved, got %q", gotPath)
	}
	// Director only rewrote URL.Host, never the Host header, so the upstream still
	// sees the original host. Pinned here because Rewrite could easily change it.
	if gotHost != "obot.example.com" {
		t.Errorf("expected the inbound Host header to be passed through, got %q", gotHost)
	}
	if gotXFF != "192.0.2.10" {
		t.Errorf("expected X-Forwarded-For to carry the client IP (as Director used to), got %q", gotXFF)
	}
	if gotProto != "http" {
		t.Errorf("expected X-Forwarded-Proto http for a non-TLS hop, got %q", gotProto)
	}
}
