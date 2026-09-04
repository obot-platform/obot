package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	vmcpOAuthID        = "vmcp1oauth"
	vmcpOAuthUserID    = "user-oauth"
	vmcpOAuthInstance  = "vmcpi1-user-oauth-vmcp1oauth"
	vmcpOAuthComponent = "ms1-vmcp-oauth-component"
)

func TestVMCPComponentServersForAuthUsesConfiguredComponents(t *testing.T) {
	fixture := newVMCPOAuthFixture(t)
	unrelatedServer := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ms2-unrelated-component",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Name:    "Unrelated component",
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "unrelated-package",
				},
			},
			UserID:          vmcpOAuthUserID,
			VMCPInstanceID:  vmcpOAuthInstance,
			VMCPComponentID: "unrelated-component",
		},
	}
	require.NoError(t, fixture.storage.Create(t.Context(), unrelatedServer))

	componentServers, err := fixture.factory.componentServersForAuth(
		fixture.request(t.Context()),
		fixture.aggregateServer,
		fixture.aggregateConfig,
	)
	require.NoError(t, err)
	require.Len(t, componentServers, 1)
	require.Equal(t, vmcpOAuthComponent, componentServers[0].Name)
}

func TestCheckForMCPAuthVMCPWithNoRemoteComponentsReturnsEmpty(t *testing.T) {
	fixture := newVMCPOAuthFixture(t)

	authURL, err := fixture.factory.CheckForMCPAuth(
		fixture.request(t.Context()),
		fixture.aggregateServer,
		fixture.aggregateConfig,
		vmcpOAuthUserID,
		vmcpOAuthID,
		"",
	)
	require.NoError(t, err)
	require.Empty(t, authURL)
}

type vmcpOAuthFixture struct {
	storage         kclient.WithWatch
	factory         *MCPOAuthHandlerFactory
	aggregateServer v1.MCPServer
	aggregateConfig mcp.ServerConfig
}

func newVMCPOAuthFixture(t *testing.T) vmcpOAuthFixture {
	t.Helper()

	componentServer := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmcpOAuthComponent,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Name:    "Component",
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "component-package",
				},
			},
			UserID:          vmcpOAuthUserID,
			VMCPInstanceID:  vmcpOAuthInstance,
			VMCPComponentID: "component",
		},
	}

	storage := clientfake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(componentServer).
		Build()

	return vmcpOAuthFixture{
		storage: storage,
		factory: NewMCPOAuthHandlerFactory(
			"https://obot.example.com",
			nil,
			storage,
			nil,
			nil,
			"",
			false,
		),
		aggregateServer: v1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmcpOAuthID,
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerSpec{
				Manifest: types.MCPServerManifest{
					Runtime: types.RuntimeVMCP,
				},
			},
		},
		aggregateConfig: mcp.ServerConfig{
			Runtime:       types.RuntimeVMCP,
			MCPServerName: vmcpOAuthID,
			UserID:        vmcpOAuthUserID,
			Components: []mcp.ComponentServer{
				{
					Name:        vmcpOAuthComponent,
					DisplayName: "component",
				},
			},
		},
	}
}

func (f vmcpOAuthFixture) request(ctx context.Context) api.Context {
	request := httptest.NewRequest(http.MethodGet, "/api/oauth/composite/"+vmcpOAuthID, nil).WithContext(ctx)
	request.SetPathValue("mcp_id", vmcpOAuthID)
	return api.Context{
		Request:        request,
		Storage:        f.storage,
		User:           &user.DefaultInfo{Name: vmcpOAuthUserID, UID: vmcpOAuthUserID},
		ResponseWriter: httptest.NewRecorder(),
	}
}
