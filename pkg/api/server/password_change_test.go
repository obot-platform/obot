package server

import (
	"net/http"
	"testing"
)

func TestPasswordChangeRequestAllowed(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodGet, "/change-password", true},
		{http.MethodGet, "/change-password/__data.json", true},
		{http.MethodGet, "/_app/immutable/entry/app.js", true},
		{http.MethodGet, "/api/me", true},
		{http.MethodGet, "/api/version", true},
		{http.MethodGet, "/api/license", true},
		{http.MethodGet, "/api/app-preferences", true},
		{http.MethodPost, "/api/local-auth/change-password", true},
		{http.MethodGet, "/oauth2/sign_out", true},
		{http.MethodGet, "/favicon.ico", true},
		{http.MethodGet, "/admin/dashboard", false},
		{http.MethodGet, "/api/users", false},
		{http.MethodPost, "/api/me", false},
		{http.MethodGet, "/api/local-auth/change-password", false},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatalf("creating request: %v", err)
			}
			if got := passwordChangeRequestAllowed(req); got != tt.want {
				t.Fatalf("passwordChangeRequestAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}
