package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginOf(t *testing.T) {
	tests := []struct {
		rawURL string
		want   string
	}{
		{
			rawURL: "https://obot.example.com",
			want:   "https://obot.example.com",
		},
		{
			rawURL: "https://Obot.Example.com/path?q=1",
			want:   "https://obot.example.com",
		},
		{
			rawURL: "HTTPS://obot.example.com",
			want:   "https://obot.example.com",
		},
		{
			rawURL: "https://obot.example.com:443",
			want:   "https://obot.example.com",
		},
		{
			rawURL: "http://localhost:80",
			want:   "http://localhost",
		},
		{
			rawURL: "http://localhost:8080",
			want:   "http://localhost:8080",
		},
		{
			// A default port belongs to its own scheme only.
			rawURL: "http://obot.example.com:443",
			want:   "http://obot.example.com:443",
		},
		{
			rawURL: "https://[::1]:443",
			want:   "https://[::1]",
		},
		{
			rawURL: "https://[::1]:8443",
			want:   "https://[::1]:8443",
		},
		{
			rawURL: "obot.example.com",
			want:   "",
		},
		{
			rawURL: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.rawURL, func(t *testing.T) {
			if got := OriginOf(tt.rawURL); got != tt.want {
				t.Fatalf("OriginOf(%q) = %q, want %q", tt.rawURL, got, tt.want)
			}
		})
	}
}

// A browser sends a canonical Origin, so a UI hostname configured with capital letters or its default port must still
// be trusted. http.CrossOriginProtection compares trusted origins as exact strings.
func TestTrustOriginsMatchesWhatBrowsersSend(t *testing.T) {
	for _, configured := range []string{
		"https://ui.example.com",
		"https://UI.Example.com",
		"https://ui.example.com:443",
	} {
		t.Run(configured, func(t *testing.T) {
			var protection http.CrossOriginProtection
			TrustOrigins(&protection, configured)

			req := httptest.NewRequest(http.MethodPost, "https://obot.example.com/api/example", nil)
			req.Header.Set("Sec-Fetch-Site", "same-site")
			req.Header.Set("Origin", "https://ui.example.com")
			if err := protection.Check(req); err != nil {
				t.Fatalf("request from the configured UI hostname was rejected: %v", err)
			}
		})
	}
}
