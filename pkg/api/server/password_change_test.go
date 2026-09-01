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
		{
			method: http.MethodGet,
			path:   "/change-password",
			want:   true,
		},
		{
			method: http.MethodGet,
			path:   "/change-password/__data.json",
			want:   true,
		},
		{
			method: http.MethodGet,
			path:   "/_app/immutable/entry/app.js",
			want:   true,
		},
		{
			method: http.MethodGet,
			path:   "/favicon.ico",
			want:   true,
		},
		{
			method: http.MethodGet,
			path:   "/oauth2/sign_out",
			want:   true,
		},

		{
			method: http.MethodGet,
			path:   "/api/me",
			want:   true,
		},
		{
			method: http.MethodGet,
			path:   "/api/version",
			want:   true,
		},
		{
			method: http.MethodGet,
			path:   "/api/license",
			want:   true,
		},
		{
			method: http.MethodGet,
			path:   "/api/app-preferences",
			want:   true,
		},
		{
			method: http.MethodPost,
			path:   "/api/local-auth/change-password",
			want:   true,
		},

		{
			method: http.MethodGet,
			path:   "/admin/dashboard",
			want:   false,
		},
		{
			method: http.MethodGet,
			path:   "/api/users",
			want:   false,
		},
		{
			method: http.MethodPost,
			path:   "/api/me",
			want:   false,
		},
		{
			method: http.MethodGet,
			path:   "/api/local-auth/change-password",
			want:   false,
		},
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
