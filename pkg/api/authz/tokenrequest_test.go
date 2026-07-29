package authz

import (
	"net/http"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestTokenRequestAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, false)
	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		groups  []string
		allowed bool
	}{
		{
			name:    "anonymous can create",
			method:  http.MethodPost,
			path:    "/api/token-request",
			groups:  []string{UnauthenticatedGroup},
			allowed: true,
		},
		{
			name:    "anonymous can poll",
			method:  http.MethodGet,
			path:    "/api/token-request/request-id",
			groups:  []string{UnauthenticatedGroup},
			allowed: true,
		},
		{
			name:    "anonymous cannot verify",
			method:  http.MethodPost,
			path:    "/api/token-request/verify",
			body:    `{"code":"2345-6789","userId":999}`,
			groups:  []string{UnauthenticatedGroup},
			allowed: false,
		},
		{
			name:    "ordinary authenticated user can verify",
			method:  http.MethodPost,
			path:    "/api/token-request/verify",
			body:    `{"code":"2345-6789"}`,
			groups:  []string{types.GroupAuthenticated, types.GroupAPI},
			allowed: true,
		},
		{
			name:    "body cannot change verification authorization",
			method:  http.MethodPost,
			path:    "/api/token-request/verify",
			body:    `{"code":"2345-6789","userId":999}`,
			groups:  []string{types.GroupAuthenticated, types.GroupAPI},
			allowed: true,
		},
		{
			name:    "request redirect pattern matches router shape",
			method:  http.MethodGet,
			path:    "/api/token-request/request-id/default/github",
			groups:  []string{UnauthenticatedGroup},
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			allowed := authorizer.Authorize(req, &user.DefaultInfo{UID: "42", Groups: tt.groups})
			if allowed != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", allowed, tt.allowed)
			}
		})
	}
}
