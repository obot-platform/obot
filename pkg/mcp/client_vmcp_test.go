package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authz"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestListToolsThroughSharedVMCPComponentConnection(t *testing.T) {
	parent := &v1.VMCP{
		Name: "vmcp1shared", Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{{ID: "one"}},
			Profiles: []types.VMCPProfile{{
				Subjects:      []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}},
				AllowAllTools: true,
			}},
		}},
	}
	instance := &v1.VMCPInstance{
		Name: "vmcpi1one", Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			UserID: "7", Manifest: types.VMCPInstanceManifest{VMCPID: parent.Name},
		},
	}
	backing := &v1.MCPServer{
		Name: "ms1shared", Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			VMCPID: parent.Name, VMCPComponentID: "one",
			Manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://upstream.example.com/mcp"},
			},
		},
	}
	connection := &v1.MCPServerInstance{
		Name: "msi1one", Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerInstanceSpec{
			UserID: "7", VMCPInstanceID: instance.Name,
			VMCPComponentID: "one", MCPServerName: backing.Name,
		},
	}
	storage := newVMCPTestStorage(parent, instance, backing, connection)
	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), db, storage, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, gateway.Close()) })

	mcpServer := gomcp.NewServer(&gomcp.Implementation{Name: "component"}, nil)
	gomcp.AddTool(mcpServer, &gomcp.Tool{Name: "hello"}, func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, struct{}, error) {
		return &gomcp.CallToolResult{}, struct{}{}, nil
	})
	transport := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return mcpServer }, nil)
	var tokens *persistent.TokenService
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := tokens.DecodeToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/mcp-connect/")
		allowed, err := authz.UserCanConnectToMCP(r.Context(), storage, nil, &user.DefaultInfo{
			UID: claims.UserID, Groups: claims.UserGroups,
			Extra: map[string][]string{"authorized_mcp_ids": {claims.MCPID}},
		}, id)
		if err != nil || !allowed {
			http.Error(w, "component connection denied", http.StatusForbidden)
			return
		}
		transport.ServeHTTP(w, r)
	}))
	defer endpoint.Close()
	tokens, err = persistent.NewTokenService(endpoint.URL, gateway)
	require.NoError(t, err)
	require.NoError(t, tokens.EnsureJWK(t.Context()))
	sm := &SessionManager{
		backend: &kubernetesBackend{},
		baseURL: endpoint.URL, storageClient: storage, gatewayClient: gateway, tokenService: tokens,
		remoteURLValidationConfig: RemoteMCPURLValidationConfig{AllowLocalhostMCP: true},
	}
	t.Cleanup(sm.Close)
	server, config, err := sm.ServerForAction(t.Context(), connection.Name, "7")
	require.NoError(t, err)
	require.Equal(t, backing.Name, server.Name)
	require.Equal(t, backing.Name, config.MCPServerName)
	require.Equal(t, connection.Name, config.MCPServerInstanceID)
	tools, err := sm.ListTools(t.Context(), config)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "hello", tools[0].Name)
}
