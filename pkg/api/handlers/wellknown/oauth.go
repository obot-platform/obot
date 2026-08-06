package wellknown

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp/connectroute"
)

// oauthAuthorization handles the /.well-known/oauth-authorization-server and /.well-known/oauth-authorization-server/{mcp_id} endpoints
func (h *handler) oauthAuthorization(req api.Context) error {
	config := h.config
	if entryID := req.PathValue("entry_id"); entryID != "" {
		version, err := strconv.Atoi(req.PathValue("version"))
		if err != nil || version < 0 {
			return types.NewErrBadRequest("invalid MCP catalog entry version")
		}
		segment := connectroute.Versioned{EntryID: entryID, Version: version}.Path()[1:]
		config.Issuer = appendPathSegment(config.Issuer, segment)
		config.AuthorizationEndpoint = appendPathSegment(config.AuthorizationEndpoint, segment)
		config.RegistrationEndpoint = appendPathSegment(config.RegistrationEndpoint, segment)
		config.TokenEndpoint = appendPathSegment(config.TokenEndpoint, segment)
	} else if mcpID := req.PathValue("mcp_id"); mcpID != "" {
		config.Issuer = appendPathSegment(config.Issuer, mcpID)
		config.AuthorizationEndpoint = appendPathSegment(config.AuthorizationEndpoint, mcpID)
		config.RegistrationEndpoint = appendPathSegment(config.RegistrationEndpoint, mcpID)
		config.TokenEndpoint = appendPathSegment(config.TokenEndpoint, mcpID)
	}
	return req.Write(config)
}

func appendPathSegment(rawURL, segment string) string {
	if rawURL == "" || segment == "" {
		return rawURL
	}

	joined, err := url.JoinPath(rawURL, segment)
	if err != nil {
		return rawURL
	}
	return joined
}

func (h *handler) oauthProtectedResource(req api.Context) error {
	if entryID := req.PathValue("entry_id"); entryID != "" {
		version, err := strconv.Atoi(req.PathValue("version"))
		if err != nil || version < 0 {
			return types.NewErrBadRequest("invalid MCP catalog entry version")
		}
		route := connectroute.Versioned{EntryID: entryID, Version: version}
		return req.Write(map[string]any{
			"resource_name":            "Obot Versioned MCP Gateway",
			"resource":                 route.Resource(h.baseURL),
			"authorization_servers":    []string{strings.TrimSuffix(h.baseURL, "/") + route.Path()},
			"bearer_methods_supported": []string{"header"},
		})
	}
	mcpID := req.PathValue("mcp_id")
	if mcpID != "" {
		return req.Write(map[string]any{
			"resource_name":            "Obot MCP Gateway",
			"resource":                 fmt.Sprintf("%s/mcp-connect/%s", h.baseURL, mcpID),
			"authorization_servers":    []string{h.baseURL + "/" + mcpID},
			"bearer_methods_supported": []string{"header"},
		})
	}

	// The client is hitting the "generic" metadata endpoint and is not supplying an MCP ID. Serve the generic metadata.
	return req.Write(map[string]any{
		"resource_name":            "Obot MCP Gateway",
		"resource":                 fmt.Sprintf("%s/mcp-connect", h.baseURL),
		"authorization_servers":    []string{h.baseURL},
		"bearer_methods_supported": []string{"header"},
	})
}

func (h *handler) registryOAuthProtectedResource(req api.Context) error {
	// Return 404 if registry is in no-auth mode
	if h.registryNoAuth {
		return &types.ErrHTTP{
			Code:    http.StatusNotFound,
			Message: "Registry OAuth is not available when registry authentication is disabled",
		}
	}

	return req.Write(map[string]any{
		"resource":                 h.baseURL,
		"authorization_servers":    []string{h.baseURL},
		"bearer_methods_supported": []string{"header"},
	})
}
