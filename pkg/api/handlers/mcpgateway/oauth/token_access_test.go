package oauth

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An authorization code can outlive the user's access to the vMCP it was issued for. Redeeming it
// must not produce a token that can connect to that vMCP.
func TestAuthorizationCodeForRevokedVMCPAccess(t *testing.T) {
	const (
		baseURL    = "https://obot.example.com"
		clientName = "oauth-client"
		vmcpID     = system.VMCPPrefix + "restricted"
		instanceID = system.VMCPInstancePrefix + "restricted"
		code       = "authorization-code"
	)

	tests := []struct {
		name            string
		profileSubjects []types.Subject
		wantGranted     bool
	}{
		{
			name:            "user still has access",
			profileSubjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "42"}},
			wantGranted:     true,
		},
		{
			name:            "user removed from profile",
			profileSubjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "99"}},
			wantGranted:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vmcp := &v1.VMCP{
				Namespace: system.DefaultNamespace,
				Name:      vmcpID,
				Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
					Components: []types.VMCPComponent{{ID: "component"}},
					Profiles: []types.VMCPProfile{{
						Subjects:    tt.profileSubjects,
						Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
					}},
				}},
			}
			// The user's instance survives the revocation.
			instance := &v1.VMCPInstance{
				Namespace: system.DefaultNamespace,
				Name:      instanceID,
				Spec:      v1.VMCPInstanceSpec{UserID: "42", Manifest: types.VMCPInstanceManifest{VMCPID: vmcpID}},
			}
			authRequest := &v1.OAuthAuthRequest{
				Namespace: system.DefaultNamespace,
				Name:      "oauth-request",
				Spec: v1.OAuthAuthRequestSpec{
					ClientID:       clientName,
					Resource:       baseURL + "/mcp-connect/" + vmcpID,
					Scope:          "profile email",
					HashedAuthCode: fmt.Sprintf("%x", sha256.Sum256([]byte(code))),
					UserID:         42,
					MCPID:          vmcpID,
				},
			}
			storage, gatewayClient, tokenService := newOAuthTokenTestServices(t, vmcp, instance, authRequest)

			recorder := httptest.NewRecorder()
			h := &handler{tokenService: tokenService}
			err := h.doAuthorizationCode(api.Context{
				ResponseWriter: recorder,
				Request:        httptest.NewRequest(http.MethodPost, "/oauth/token", nil),
				Storage:        storage,
				GatewayClient:  gatewayClient,
			}, v1.OAuthClient{Namespace: system.DefaultNamespace, Name: clientName}, code, "")

			t.Run("token exchange", func(t *testing.T) {
				if tt.wantGranted {
					require.NoError(t, err)
				} else {
					require.Error(t, err, "code exchange issued a token to a user without access")
				}
			})
			if err != nil {
				return
			}

			var response types.OAuthToken
			require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
			require.NotEmpty(t, response.AccessToken)

			// Authenticate and authorize the issued token the way the server does for /mcp-connect.
			authorizer := authz.NewAuthorizer(gatewayClient, storage, storage, false, nil, nil, false)
			for _, id := range []string{vmcpID, instanceID} {
				t.Run("mcp-connect "+id, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodPost, "/mcp-connect/"+id, nil)
					req.Header.Set("Authorization", "Bearer "+response.AccessToken)
					resp, ok, err := tokenService.AuthenticateRequest(req)
					require.NoError(t, err)
					require.True(t, ok)
					assert.Equal(t, tt.wantGranted, authorizer.Authorize(req, resp.User))
				})
			}
		})
	}
}
