package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
)

func TestVMCPStaticOAuthCredentialRotation(t *testing.T) {
	entry := vmcpCatalogEntryForTest("static-source")
	ref := system.MCPOAuthCredentialName(entry.Name)
	vmcp := &v1.VMCP{
		Name:      "vmcp1oauth",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{
			ID:                      "component",
			MCPServerCatalogEntryID: entry.Name,
			OAuthCredentialID:       ref,
		}}}},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1oauth",
		Namespace: system.DefaultNamespace,
		Spec:      v1.VMCPInstanceSpec{Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
	}
	storage := newVMCPTestStorage(vmcp, instance, entry)
	gw := newHandlerTestGateway(t)
	ctx := api.Context{Storage: storage, GatewayClient: gw,
		Request: httptest.NewRequest(http.MethodGet, "/", nil).WithContext(t.Context())}
	servers := []v1.MCPServer{
		{
			Namespace: system.DefaultNamespace,
			Spec:      v1.MCPServerSpec{VMCPID: vmcp.Name, VMCPComponentID: "component"},
		},
		{
			Namespace: system.DefaultNamespace,
			Spec:      v1.MCPServerSpec{VMCPInstanceID: instance.Name, VMCPComponentID: "component"},
		},
		{
			Namespace: system.DefaultNamespace,
			Spec:      v1.MCPServerSpec{MCPServerCatalogEntryName: entry.Name},
		},
	}
	for _, secret := range []string{"original-secret", "rotated-secret"} {
		require.NoError(t, gw.UpsertCredential(t.Context(), gatewaytypes.Credential{
			Context: ref,
			Name:    system.StaticOAuthCredentialName,
			Secrets: map[string]string{"CLIENT_ID": "static-client", "CLIENT_SECRET": secret},
		}))
		for _, server := range servers {
			id, got, err := (*MCPHandler)(nil).lookupStaticOAuthClient(ctx, server)
			require.NoError(t, err)
			require.Equal(t, "static-client", id)
			require.Equal(t, secret, got)
		}
	}
	require.NoError(t, storage.Delete(t.Context(), entry))
	for _, server := range servers[:2] {
		_, secret, err := (*MCPHandler)(nil).lookupStaticOAuthClient(ctx, server)
		require.NoError(t, err)
		require.Equal(t, "rotated-secret", secret)
	}
}
