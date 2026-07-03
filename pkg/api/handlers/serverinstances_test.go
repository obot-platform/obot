package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// GetOAuthURL must not leak another user's instance: the ownership guard runs
// before any server/credential resolution, so a non-owner gets NotFound and the
// nil mcpOAuthChecker is never reached.
func TestServerInstanceGetOAuthURLRejectsNonOwner(t *testing.T) {
	instance := v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "instance-1",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerInstanceSpec{
			UserID:        "owner-uid",
			MCPServerName: "server-1",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/mcp-server-instances/instance-1/oauth-url", nil)
	req.SetPathValue("mcp_server_instance_id", "instance-1")

	err := (&ServerInstancesHandler{}).GetOAuthURL(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		Storage:        newFakeStorage(t, &instance),
		User:           &kuser.DefaultInfo{UID: "intruder-uid"},
	})

	require.Error(t, err)
	assert.True(t, types.IsNotFound(err), "expected not found error, got %v", err)
}

func TestServerInstanceRedirectOAuthURLRedirectsOwnedInstance(t *testing.T) {
	instance := v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "instance-1",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerInstanceSpec{
			UserID:        "owner-uid",
			MCPServerName: "server-1",
		},
	}
	server := v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "server-1",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerSpec{
			NeedsURL: true,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/mcp-server-instances/instance-1/oauth-redirect", nil)
	req.SetPathValue("mcp_server_instance_id", "instance-1")
	rec := httptest.NewRecorder()

	err := (&ServerInstancesHandler{
		mcpOAuthChecker: fixedOAuthChecker{url: "https://oauth.example.test/authorize?state=state-1"},
	}).RedirectOAuthURL(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        newFakeStorage(t, &instance, &server),
		User:           &kuser.DefaultInfo{UID: "owner-uid"},
	})

	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://oauth.example.test/authorize?state=state-1", rec.Header().Get("Location"))
}

type fixedOAuthChecker struct {
	url string
}

func (f fixedOAuthChecker) CheckForMCPAuth(api.Context, v1.MCPServer, mcp.ServerConfig, string, string, string) (string, error) {
	return f.url, nil
}
