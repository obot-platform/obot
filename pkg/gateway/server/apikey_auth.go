package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	// HeaderObotAPIKey carries an Obot API key alongside another credential, on LLM proxy
	// requests.
	HeaderObotAPIKey = "X-Obot-Api-Key"

	llmProxyPathPrefix = "/api/llm-proxy/"
)

// APIKeyAuthenticator authenticates requests using API keys.
// API key users have restricted access - they only get GroupAPIKey,
// not the full authenticated user groups.
type APIKeyAuthenticator struct {
	client *client.Client
}

// NewAPIKeyAuthenticator creates a new API key authenticator.
func NewAPIKeyAuthenticator(client *client.Client) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{client: client}
}

// AuthenticateRequest implements authenticator.Request.
func (a *APIKeyAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	// X-Obot-Api-Key lets a client whose Authorization header carries another credential,
	// such as Claude Code signed in with a Claude subscription, send its Obot API key too. It
	// is only for the LLM proxy, which strips it before forwarding; elsewhere, such as on MCP
	// connections, it would travel on to an upstream server.
	var authHeader string
	if strings.HasPrefix(req.URL.Path, llmProxyPathPrefix) {
		authHeader = req.Header.Get(HeaderObotAPIKey)
	}
	if authHeader == "" {
		authHeader = strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	}
	if authHeader == "" {
		authHeader = req.Header.Get("X-API-Key")
		if authHeader == "" {
			return nil, false, nil
		}
	}

	// Check if this value uses the shared API key prefix.
	if !strings.HasPrefix(authHeader, system.APIKeyPrefix+"-") {
		return nil, false, nil
	}

	// Validate the API key
	apiKey, err := a.client.ValidateAPIKey(req.Context(), authHeader)
	if errors.Is(err, client.ErrInvalidAPIKey) {
		// Return false, nil to let other authenticators try
		// This allows the chain to continue if the key is invalid
		return nil, false, nil
	} else if err != nil {
		// The key or its user could not be read, so its user's access is unknown. Fail rather than letting the
		// request continue to another credential or to anonymous access.
		_, userID, _, _, _ := client.ParseAPIKey(authHeader)
		return nil, false, &client.UserAccessLookupError{
			UserID: userID,
			Err:    err,
		}
	}

	// Get the user from the database
	u, authProviderGroups, err := a.client.UserByIDWithEffectiveRole(req.Context(), apiKey.UserID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// The key outlived its user. Deny it rather than letting the request continue as anonymous.
		return nil, false, &client.UserAccessDeniedError{
			UserID: apiKey.UserID,
			Status: types2.UserStatusDeleted,
		}
	} else if err != nil {
		return nil, false, &client.UserAccessLookupError{
			UserID: apiKey.UserID,
			Err:    err,
		}
	}

	attribution := principal.NewAPIKeyAttribution(apiKey.ID, apiKey.UserID, apiKey.Name)
	groups := apiKey.Groups(u)
	if apiKey.ClaudeEnvironment {
		groups = append(groups, types2.GroupClaudeEnvironment)
	}
	extra := map[string][]string{
		"email":                   {u.Email},
		"authorized_mcp_ids":      apiKey.MCPServerIDs,
		"obot_groups":             u.Role.RoleGroups(),
		principal.APIKeyIDExtra:   {fmt.Sprintf("%d", attribution.ID)},
		principal.APIKeyNameExtra: {attribution.Name},
	}

	if authProviderGroups != nil {
		extra["auth_provider_groups"] = authProviderGroups
	}
	principal.RecordUserStatus(extra, u.Status())

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   u.Username,
			UID:    fmt.Sprintf("%d", u.ID),
			Groups: groups,
			Extra:  extra,
		},
	}, true, nil
}
