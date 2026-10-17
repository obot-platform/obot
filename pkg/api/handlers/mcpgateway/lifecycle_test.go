package mcpgateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"time"

	"github.com/obot-platform/mmmcp"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/stretchr/testify/require"

	"github.com/obot-platform/obot/apiclient/types"
	obotmcp "github.com/obot-platform/obot/pkg/mcp"

	"k8s.io/apiserver/pkg/authentication/user"
)

func TestCompositeLoopbackTokenRecordsCaller(t *testing.T) {
	now := time.Now()

	for _, tt := range []struct {
		name   string
		caller user.Info
	}{
		{
			name: "person",
			caller: &user.DefaultInfo{
				Name: "alice",
				UID:  "7",
				Extra: map[string][]string{
					"email": {"alice@example.com"},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := compositeLoopbackTokenContext(tt.caller, "https://obot.example.com/mcp-connect-composite/vmcp1", "vmcp1", []string{"component"}, now)

			if got.UserID != tt.caller.GetUID() {
				t.Errorf("token user = %q, want the caller %q", got.UserID, tt.caller.GetUID())
			}
		})
	}
}

func TestCompositeAuthorizationDeferredOnlyForClaudeEnvironment(t *testing.T) {
	for _, test := range []struct {
		name   string
		groups []string
		want   bool
	}{
		{
			name:   "external MCP",
			groups: []string{types.GroupMCP},
		},
		{
			name:   "external admin",
			groups: types.RoleAdmin.RoleGroups(),
		},
		{
			name:   "Claude environment",
			groups: []string{types.GroupMCP, types.GroupClaudeEnvironment},
			want:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &user.DefaultInfo{UID: "7", Groups: test.groups}
			if got := compositeConfig(obotmcp.ServerConfig{}, caller).DeferAuthorization; got != test.want {
				t.Fatalf("defer authorization = %v, want %v", got, test.want)
			}
			// The signed identity must carry the opt-in across loopback and replica forwarding.
			token := compositeLoopbackTokenContext(caller, "https://obot.example/mcp-connect-composite/vmcp1", "vmcp1", []string{"component"}, time.Now())
			if got := slices.Contains(token.UserGroups, types.GroupClaudeEnvironment); got != test.want {
				t.Fatalf("loopback Claude group = %v, want %v", got, test.want)
			}
			forwarded := &user.DefaultInfo{UID: token.UserID, Groups: token.UserGroups}
			if got := compositeConfig(obotmcp.ServerConfig{}, forwarded).DeferAuthorization; got != test.want {
				t.Fatalf("forwarded defer authorization = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCompositeAuthorizationLookupUsesAuthenticatedRequest(t *testing.T) {
	called := false
	lookup := compositeAuthorizationURLs(func(req api.Context) ([]mmmcp.AuthorizationURL, error) {
		called = true
		require.Equal(t, "42", req.User.GetUID())
		require.Equal(t, "vmcpi1user", req.PathValue("mcp_id"))
		return nil, nil
	})
	_, err := lookup(t.Context())
	require.Error(t, err)
	request := httptest.NewRequest(http.MethodPost, "/mcp-connect-composite/vmcpi1user", nil)
	request.SetPathValue("mcp_id", "vmcpi1user")
	req := api.Context{Request: request, User: &user.DefaultInfo{UID: "42", Groups: []string{types.GroupMCP}}}
	_, err = lookup(context.WithValue(t.Context(), compositeAPIContextKey{}, req))
	require.Error(t, err)
	require.False(t, called)

	req.User = &user.DefaultInfo{UID: "42", Groups: []string{types.GroupMCP, types.GroupClaudeEnvironment}}
	_, err = lookup(context.WithValue(t.Context(), compositeAPIContextKey{}, req))
	require.NoError(t, err)
	require.True(t, called)
}
