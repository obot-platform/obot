package apiclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokenHasScopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		resp   map[string]any
		want   bool
	}{
		{
			name:   "API scope satisfies non-MCP scopes",
			scopes: []string{"skills", "llm", "published-artifacts"},
			resp: map[string]any{
				"allowed": true,
				"scopes": map[string]any{
					"canAccessAPI": true,
				},
			},
			want: true,
		},
		{
			name:   "API scope does not satisfy MCP scope",
			scopes: []string{"all-mcp"},
			resp: map[string]any{
				"allowed": true,
				"scopes": map[string]any{
					"canAccessAPI": true,
				},
			},
			want: false,
		},
		{
			name:   "MCP wildcard satisfies MCP scope",
			scopes: []string{"all-mcp"},
			resp: map[string]any{
				"allowed": true,
				"scopes": map[string]any{
					"mcpServerIds": []string{"*"},
				},
			},
			want: true,
		},
		{
			name:   "specific scope satisfies matching request",
			scopes: []string{"skills"},
			resp: map[string]any{
				"allowed": true,
				"scopes": map[string]any{
					"canAccessSkills": true,
				},
			},
			want: true,
		},
		{
			name:   "missing requested scope fails",
			scopes: []string{"api"},
			resp: map[string]any{
				"allowed": true,
				"scopes":  map[string]any{},
			},
			want: false,
		},
		{
			name:   "disallowed token fails",
			scopes: []string{"api"},
			resp: map[string]any{
				"allowed": false,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api-keys/auth" {
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Fatalf("method = %s, want POST", r.Method)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
					t.Fatalf("Authorization = %q, want bearer token", got)
				}
				_ = json.NewEncoder(w).Encode(tt.resp)
			}))
			defer srv.Close()

			if got := TokenHasScopes(t.Context(), srv.URL+"/", "test-token", tt.scopes); (got == nil) != tt.want {
				t.Fatalf("TokenHasScopes() = %v, want %v", got, tt.want)
			}
		})
	}
}
