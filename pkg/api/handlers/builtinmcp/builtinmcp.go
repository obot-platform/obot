// Package builtinmcp serves the MCP servers Obot provides itself: one for every user, to
// find MCP servers in Obot and request new ones from their agent, and one for
// administrators, to decide on those requests.
package builtinmcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/version"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

type Handler struct {
	requests   *handlers.MCPServerRequestHandler
	acrHelper  *accesscontrolrule.Helper
	supervisor SupervisorAsker
	serverURL  string
}

// New returns the handler for Obot's built-in MCP servers. supervisor may be nil.
func New(requests *handlers.MCPServerRequestHandler, acrHelper *accesscontrolrule.Helper, supervisor SupervisorAsker, serverURL string) *Handler {
	return &Handler{requests: requests, acrHelper: acrHelper, supervisor: supervisor, serverURL: strings.TrimSuffix(serverURL, "/")}
}

// Serve answers an MCP request for one of the built-in servers.
func (h *Handler) Serve(req api.Context) error {
	id := req.PathValue("mcp_id")
	if !req.UserIsAuthenticated() {
		baseURL := strings.TrimSuffix(req.APIBaseURL, "/api")
		req.ResponseWriter.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="Obot MCP Gateway", resource_metadata="%s/.well-known/oauth-protected-resource/mcp-connect/%s"`, baseURL, id))
		http.Error(req.ResponseWriter, "MCP server requires authentication", http.StatusUnauthorized)
		return nil
	}
	// A token issued for another MCP server must not be replayed here.
	if mcpID := req.User.GetExtra()["mcp_id"]; len(mcpID) > 0 && !slices.Contains(mcpID, id) {
		http.Error(req.ResponseWriter, "token was not issued for this MCP server", http.StatusForbidden)
		return nil
	}
	if authorized := req.User.GetExtra()["authorized_mcp_ids"]; len(authorized) > 0 && !slices.Contains(authorized, id) {
		http.Error(req.ResponseWriter, "token is not authorized for this MCP server", http.StatusForbidden)
		return nil
	}

	// MCP OAuth tokens carry the user's Obot role in extra["obot_groups"] rather than in
	// their groups. The tools act with the user's role, as the user would in the UI.
	req.User = EffectiveUser(req.User)

	var server *mcp.Server
	switch id {
	case system.BuiltinUserMCPID:
		server = h.userServer(req)
	case system.BuiltinAdminMCPID:
		if !req.UserIsAdmin() {
			http.Error(req.ResponseWriter, "the Obot admin MCP server is for administrators", http.StatusForbidden)
			return nil
		}
		server = h.adminServer(req)
	default:
		return types.NewErrNotFound("MCP server %q not found", id)
	}

	// Each request gets its own stateless server whose tools act as this request's user.
	mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	}).ServeHTTP(req.ResponseWriter, req.Request)
	return nil
}

// EffectiveUser adds the role groups an MCP token carries in extra["obot_groups"] to the
// user's groups.
func EffectiveUser(u kuser.Info) kuser.Info {
	roleGroups := u.GetExtra()["obot_groups"]
	if len(roleGroups) == 0 {
		return u
	}
	groups := slices.Clone(u.GetGroups())
	for _, g := range roleGroups {
		if !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
	}
	return &kuser.DefaultInfo{
		Name:   u.GetName(),
		UID:    u.GetUID(),
		Groups: groups,
		Extra:  u.GetExtra(),
	}
}

func newServer(name, title, instructions string) *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{Name: name, Title: title, Version: version.Tag}, &mcp.ServerOptions{
		Instructions: instructions,
	})
}

// jsonResult returns v as the tool's text and structured result.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

// toolError reports a failure the model can act on, rather than a protocol error.
func toolError(err error) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}, nil, nil
}

func (h *Handler) connectURL(id string) string {
	return h.serverURL + "/mcp-connect/" + id
}
