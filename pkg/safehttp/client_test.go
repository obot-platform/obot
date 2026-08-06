package safehttp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClientBlocksLoopbackLiteralIP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request should have been blocked before reaching server")
	}))
	defer ts.Close()

	_, err := NewClient(ClientOptions{BlockLoopback: true}).Get(ts.URL)
	if err == nil {
		t.Fatal("expected loopback IP to be blocked")
	}
	if !strings.Contains(err.Error(), "blocked loopback IP") {
		t.Fatalf("expected loopback error, got %v", err)
	}
}

func TestClientBlocksLoopbackHostname(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request should have been blocked before reaching server")
	}))
	defer ts.Close()

	_, err := NewClient(ClientOptions{BlockLoopback: true}).Get(strings.Replace(ts.URL, "127.0.0.1", "localhost", 1))
	if err == nil {
		t.Fatal("expected localhost to resolve to a blocked loopback IP")
	}
	if !strings.Contains(err.Error(), "blocked loopback IP") {
		t.Fatalf("expected loopback error, got %v", err)
	}
}

func TestClientBlocksPrivateIP(t *testing.T) {
	_, err := NewClient(ClientOptions{BlockPrivateIP: true}).Get("http://192.168.0.1/.well-known/oauth-protected-resource")
	if err == nil {
		t.Fatal("expected private IP to be blocked")
	}
	if !strings.Contains(err.Error(), "blocked private IP") {
		t.Fatalf("expected private IP error, got %v", err)
	}
}

func TestClientBlocksLinkLocalIP(t *testing.T) {
	_, err := NewClient(ClientOptions{BlockLinkLocal: true}).Get("http://169.254.169.254/latest/meta-data")
	if err == nil {
		t.Fatal("expected link-local IP to be blocked")
	}
	if !strings.Contains(err.Error(), "blocked link-local IP") {
		t.Fatalf("expected link-local IP error, got %v", err)
	}
}

func TestClientAllowsExplicitlyDisabledBlockedRanges(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	resp, err := NewClient(ClientOptions{}).Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestClientAllowsBlockedIPWhenAllowListed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	serverURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := NewClient(ClientOptions{
		BlockLoopback: true,
		AllowList:     []string{serverURL.Host},
	}).Get(ts.URL)
	if err != nil {
		t.Fatalf("expected allow-listed loopback IP to be allowed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestClientAllowsBlockedHostnameWhenAllowListed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	serverURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	localURL := strings.Replace(ts.URL, "127.0.0.1", "localhost", 1)

	resp, err := NewClient(ClientOptions{
		BlockLoopback: true,
		AllowList:     []string{"localhost:" + serverURL.Port()},
	}).Get(localURL)
	if err != nil {
		t.Fatalf("expected allow-listed localhost to be allowed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestClientBlocksAllowListedHostnameWithMismatchedPort(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request should have been blocked before reaching server")
	}))
	defer ts.Close()

	localURL := strings.Replace(ts.URL, "127.0.0.1", "localhost", 1)
	_, err := NewClient(ClientOptions{
		BlockLoopback: true,
		AllowList:     []string{"localhost:1"},
	}).Get(localURL)
	if err == nil {
		t.Fatal("expected localhost to be blocked when allow-list port mismatches")
	}
	if !strings.Contains(err.Error(), "blocked loopback IP") {
		t.Fatalf("expected loopback error, got %v", err)
	}
}

func TestClientAddsConfiguredHeadersWithoutMutatingRequest(t *testing.T) {
	var receivedHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		receivedHeaders = req.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	serverURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	client := NewClient(ClientOptions{
		BlockLoopback: true,
		AllowList:     []string{serverURL.Host},
		Headers: http.Header{
			"X-Injected": {"configured"},
			"X-Override": {"configured"},
		},
	})
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Original", "preserved")
	request.Header.Set("X-Override", "request")

	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if receivedHeaders.Get("X-Original") != "preserved" {
		t.Fatalf("original header = %q, want preserved", receivedHeaders.Get("X-Original"))
	}
	if receivedHeaders.Get("X-Injected") != "configured" {
		t.Fatalf("injected header = %q, want configured", receivedHeaders.Get("X-Injected"))
	}
	if receivedHeaders.Get("X-Override") != "configured" {
		t.Fatalf("overridden header = %q, want configured", receivedHeaders.Get("X-Override"))
	}
	if request.Header.Get("X-Injected") != "" {
		t.Fatal("configured header was added to the original request")
	}
	if request.Header.Get("X-Override") != "request" {
		t.Fatalf("original request header = %q, want request", request.Header.Get("X-Override"))
	}
}

func TestAllowListMatchesExactAndSuffixHosts(t *testing.T) {
	dialer := safeDialer{
		allowList: parseAllowList([]string{
			"api.example.com",
			"*.internal.example.com",
			"db.example.com:8443",
		}),
	}

	tests := []struct {
		name string
		host string
		port string
		want bool
	}{
		{
			name: "exact host",
			host: "api.example.com",
			port: "443",
			want: true,
		},
		{
			name: "exact host is not suffix",
			host: "xapi.example.com",
			port: "443",
			want: false,
		},
		{
			name: "suffix host",
			host: "mcp.internal.example.com",
			port: "443",
			want: true,
		},
		{
			name: "suffix host nested",
			host: "a.b.internal.example.com",
			port: "443",
			want: true,
		},
		{
			name: "suffix does not match apex",
			host: "internal.example.com",
			port: "443",
			want: false,
		},
		{
			name: "port match",
			host: "db.example.com",
			port: "8443",
			want: true,
		},
		{
			name: "port mismatch",
			host: "db.example.com",
			port: "443",
			want: false,
		},
		{
			name: "case insensitive",
			host: "API.EXAMPLE.COM",
			port: "443",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dialer.isAllowed(tt.host, tt.port); got != tt.want {
				t.Fatalf("isAllowed(%q, %q) = %v, want %v", tt.host, tt.port, got, tt.want)
			}
		})
	}
}
