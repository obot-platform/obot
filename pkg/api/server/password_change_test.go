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
			path:   "/activate",
			want:   true,
		},
		{
			method: http.MethodGet,
			path:   "/activate/__data.json",
			want:   true,
		},
		{
			method: http.MethodPost,
			path:   "/api/local-auth/activate",
			want:   true,
		},

		{
			method: http.MethodGet,
			path:   "/admin/dashboard",
			want:   false,
		},
		{
			method: http.MethodGet,
			path:   "/activated",
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
			if got := passwordChangeRequestAllowed(req, false); got != tt.want {
				t.Fatalf("passwordChangeRequestAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// In dev mode the Vite dev server serves the UI's modules from its own paths, which the
// change-password page needs; outside dev mode they aren't let through.
func TestPasswordChangeRequestAllowsViteModulesInDevMode(t *testing.T) {
	for _, path := range []string{
		"/@fs/repo/ui/user/.svelte-kit/generated/client/app.js",
		"/@vite/client",
		"/@id/__x00__virtual:module",
		"/node_modules/.vite/deps/svelte.js",
		"/src/routes/change-password/+page.svelte",
		"/.svelte-kit/generated/root.svelte",
	} {
		req, err := http.NewRequest(http.MethodGet, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !passwordChangeRequestAllowed(req, true) {
			t.Errorf("%s: blocked in dev mode", path)
		}
		if passwordChangeRequestAllowed(req, false) {
			t.Errorf("%s: allowed outside dev mode", path)
		}
	}
	post, err := http.NewRequest(http.MethodPost, "/@fs/anything", nil)
	if err != nil {
		t.Fatal(err)
	}
	if passwordChangeRequestAllowed(post, true) {
		t.Error("a POST to a dev module path is allowed")
	}
}
