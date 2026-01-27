package server

import (
	"testing"
)

func TestServer_AuthCompleteURL(t *testing.T) {
	tests := []struct {
		name   string
		uiURL  string
		want   string
	}{
		{
			name:  "basic URL",
			uiURL: "https://example.com",
			want:  "https://example.com/login_complete",
		},
		{
			name:  "URL with trailing slash",
			uiURL: "https://example.com/",
			want:  "https://example.com//login_complete",
		},
		{
			name:  "localhost URL",
			uiURL: "http://localhost:8080",
			want:  "http://localhost:8080/login_complete",
		},
		{
			name:  "URL with path",
			uiURL: "https://example.com/app",
			want:  "https://example.com/app/login_complete",
		},
		{
			name:  "URL with port",
			uiURL: "https://example.com:9090",
			want:  "https://example.com:9090/login_complete",
		},
		{
			name:  "empty URL",
			uiURL: "",
			want:  "/login_complete",
		},
		{
			name:  "URL with subdomain",
			uiURL: "https://app.example.com",
			want:  "https://app.example.com/login_complete",
		},
		{
			name:  "URL with query parameters (edge case)",
			uiURL: "https://example.com?foo=bar",
			want:  "https://example.com?foo=bar/login_complete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				uiURL: tt.uiURL,
			}
			got := s.authCompleteURL()
			if got != tt.want {
				t.Errorf("authCompleteURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
