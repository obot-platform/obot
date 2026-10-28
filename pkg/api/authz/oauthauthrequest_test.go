package authz

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestOAuthAuthRequestRouteAuthorization(t *testing.T) {
	routes := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "consent",
			method: http.MethodGet,
			path:   "/oauth/consent/%s",
		},
		{
			name:   "approve",
			method: http.MethodPost,
			path:   "/oauth/consent/%s/approve",
		},
		{
			name:   "cancel",
			method: http.MethodPost,
			path:   "/oauth/consent/%s/cancel",
		},
		{
			name:   "complete",
			method: http.MethodGet,
			path:   "/oauth/complete/%s",
		},
	}

	for _, tc := range []struct {
		name          string
		requestUserID uint
		vmcpUserID    string
		missing       bool
		allowed       bool
	}{
		{
			name:          "owner with MCP access",
			requestUserID: 7,
			vmcpUserID:    "7",
			allowed:       true,
		},
		{
			name:          "another users request despite MCP access",
			requestUserID: 8,
			vmcpUserID:    "7",
			allowed:       false,
		},
		{
			name:          "missing request",
			requestUserID: 7,
			vmcpUserID:    "7",
			missing:       true,
			allowed:       false,
		},
		{
			name:          "owner without MCP access",
			requestUserID: 7,
			vmcpUserID:    "8",
			allowed:       false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vmcp := &v1.VMCP{
				Name:      "vmcp1oauth",
				Namespace: system.DefaultNamespace,
				Spec: v1.VMCPSpec{
					UserID: tc.vmcpUserID,
					Manifest: types.VMCPManifest{
						Components: []types.VMCPComponent{{ID: "component"}},
					},
				},
			}
			authRequest := &v1.OAuthAuthRequest{
				Name:      "oar1test",
				Namespace: system.DefaultNamespace,
				Spec: v1.OAuthAuthRequestSpec{
					UserID: tc.requestUserID,
					MCPID:  vmcp.Name,
				},
			}
			builder := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(vmcp)
			if !tc.missing {
				builder.WithObjects(authRequest)
			}

			storage := builder.Build()
			authorizer := NewAuthorizer(nil, storage, storage, false, nil, nil, nil)
			caller := &user.DefaultInfo{
				UID:    "7",
				Groups: []string{types.GroupAPI, types.GroupAuthenticated},
			}

			for _, route := range routes {
				t.Run(route.name, func(t *testing.T) {
					req := httptest.NewRequest(route.method, fmt.Sprintf(route.path, authRequest.Name), nil)

					if got := authorizer.Authorize(req, caller); got != tc.allowed {
						t.Fatalf("Authorize() = %v, want %v", got, tc.allowed)
					}
				})
			}
		})
	}
}
