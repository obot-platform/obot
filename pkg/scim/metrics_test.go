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
			name:          "user get, case-insensitively",
			method:        http.MethodGet,
			segments:      []string{"users", "abc"},
			wantResource:  "Users",
			wantOperation: "get",
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
