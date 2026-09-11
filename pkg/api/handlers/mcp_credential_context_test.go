package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestMCPServerCredentialScopeSelection(t *testing.T) {
	for _, tc := range []struct {
		name           string
		spec           v1.MCPServerSpec
		wantRevealed   string
		wantConfigured string
	}{
		{
			name:           "personal",
			wantRevealed:   "requester-token",
			wantConfigured: "owner-token",
		},
		{
			name: "vMCP from catalog",
			spec: v1.MCPServerSpec{
				MCPCatalogID: "catalog",
				VMCPID:       "vmcp",
			},
			wantRevealed:   "vmcp-token",
			wantConfigured: "vmcp-token",
		},
		{
			name: "vMCP instance from catalog",
			spec: v1.MCPServerSpec{
				MCPCatalogID:   "catalog",
				VMCPInstanceID: "instance",
			},
			wantRevealed:   "instance-token",
			wantConfigured: "instance-token",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := v1.MCPServer{
				Name:      "server",
				Namespace: system.DefaultNamespace,
				Spec:      tc.spec,
			}
			server.Spec.UserID = "owner"
			server.Spec.Manifest.Config = []types.MCPConfig{{Key: "TOKEN"}}
			gatewayClient := newHandlerTestGateway(t)
			for _, owner := range []string{"owner", "requester", "catalog", "vmcp", "instance"} {
				if err := gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
					Context: owner + "-server",
					Name:    server.Name,
					Secrets: map[string]string{"TOKEN": owner + "-token"},
				}); err != nil {
					t.Fatal(err)
				}
			}
			request := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/server/reveal", nil)
			request.SetPathValue("mcp_server_id", server.Name)
			request.SetPathValue("catalog_id", server.Spec.MCPCatalogID)
			recorder := httptest.NewRecorder()
			ctx := api.Context{
				Request:        request,
				ResponseWriter: recorder,
				Storage:        newVMCPTestStorage(&server),
				GatewayClient:  gatewayClient,
				User:           &user.DefaultInfo{UID: "requester"},
			}
			if err := (&MCPHandler{}).Reveal(ctx); err != nil {
				t.Fatal(err)
			}
			var revealed map[string]string
			if err := json.NewDecoder(recorder.Body).Decode(&revealed); err != nil {
				t.Fatal(err)
			}
			if revealed["TOKEN"] != tc.wantRevealed {
				t.Fatalf("Reveal returned %v, want TOKEN=%q", revealed, tc.wantRevealed)
			}
			configured, err := credentialEnvForMCPServer(ctx, server, "")
			if err != nil {
				t.Fatal(err)
			}
			if configured["TOKEN"] != tc.wantConfigured {
				t.Fatalf("server configuration = %v, want TOKEN=%q", configured, tc.wantConfigured)
			}
		})
	}
}
