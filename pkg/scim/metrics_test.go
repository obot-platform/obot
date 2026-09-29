package scim

import (
	"net/http"
	"testing"
)

func TestDescribeRequest(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		segments      []string
		wantResource  string
		wantOperation string
	}{
		{
			name:          "user list",
			method:        http.MethodGet,
			segments:      []string{"Users"},
			wantResource:  "Users",
			wantOperation: "list",
		},
		{
			name:          "user get, case-insensitively",
			method:        http.MethodGet,
			segments:      []string{"users", "abc"},
			wantResource:  "Users",
			wantOperation: "get",
		},
		{
			name:          "group create",
			method:        http.MethodPost,
			segments:      []string{"Groups"},
			wantResource:  "Groups",
			wantOperation: "create",
		},
		{
			name:          "group replace",
			method:        http.MethodPut,
			segments:      []string{"Groups", "abc"},
			wantResource:  "Groups",
			wantOperation: "replace",
		},
		{
			name:          "group patch",
			method:        http.MethodPatch,
			segments:      []string{"Groups", "abc"},
			wantResource:  "Groups",
			wantOperation: "patch",
		},
		{
			name:          "group delete",
			method:        http.MethodDelete,
			segments:      []string{"Groups", "abc"},
			wantResource:  "Groups",
			wantOperation: "delete",
		},
		{
			name:          "discovery",
			method:        http.MethodGet,
			segments:      []string{"Schemas", "urn:ietf:params:scim:schemas:core:2.0:User"},
			wantResource:  "Schemas",
			wantOperation: "discovery",
		},
		{
			name:          "an unknown resource does not add a series",
			method:        http.MethodGet,
			segments:      []string{"Anything-" + "random"},
			wantResource:  "other",
			wantOperation: "get",
		},
		{
			name:          "the base URL",
			method:        http.MethodGet,
			wantResource:  "other",
			wantOperation: "get",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource, operation := describeRequest(tt.method, tt.segments)
			if resource != tt.wantResource || operation != tt.wantOperation {
				t.Fatalf("describeRequest() = %q, %q, want %q, %q", resource, operation, tt.wantResource, tt.wantOperation)
			}
		})
	}
}
