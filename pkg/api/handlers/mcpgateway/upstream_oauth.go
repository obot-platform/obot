package mcpgateway

import (
	"cmp"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/obot-platform/mmmcp/component"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type upstreamCaptureConfigKey struct{}

func compositeAuthorizationResponse(manager *mcp.SessionManager) func(http.ResponseWriter, *http.Request, []*component.AuthorizationError) {
	return func(w http.ResponseWriter, r *http.Request, challenges []*component.AuthorizationError) {
		req, ok := r.Context().Value(compositeAPIContextKey{}).(api.Context)
		cfg, configured := r.Context().Value(upstreamCaptureConfigKey{}).(mcp.ServerConfig)
		if !ok || !configured {
			http.Error(w, "upstream authorization context unavailable", http.StatusInternalServerError)

			return
		}

		req.Request = req.WithContext(r.Context())
		response, err := upstreamOAuthResponse(req, cfg, manager, challenges)
		if err != nil {
			http.Error(w, "failed to retain upstream authorization challenge", http.StatusInternalServerError)

			return
		}

		response.write(w, nil)
	}
}

func upstreamOAuthResponse(req api.Context, cfg mcp.ServerConfig, manager *mcp.SessionManager, challenges []*component.AuthorizationError) (*mcpResponse, error) {
	connectionID := req.PathValue("mcp_id")
	if slices.Contains(req.User.GetGroups(), types.GroupCompositeMCP) {
		// The original connection comes only from Obot's signed loopback token.
		connectionID = cmp.Or(req.User.GetExtra()["oauth_connection_id"]...)
		if connectionID == "" {
			return nil, fmt.Errorf("missing original vMCP connection")
		}
	}

	if len(challenges) == 0 {
		return nil, fmt.Errorf("invalid upstream challenge count")
	}

	batch := gatewaytypes.MCPOAuthChallenge{
		UserID:    req.User.GetUID(),
		Namespace: req.Namespace(),
		MCPID:     connectionID,
		ClientID:  cmp.Or(req.User.GetExtra()["oauth_client_id"]...),
		Resource:  strings.TrimSuffix(req.APIBaseURL, "/api") + "/mcp-connect/" + connectionID,
	}

	matched := make([]bool, len(challenges))

	// Walk configuration order, not goroutine completion order, to keep the UI stable.
	for _, target := range cfg.Components {
		for i, challenge := range challenges {
			// vMCP URLs are Obot loopback routes containing the component connection ID.
			// Display names can change or collide and are not part of the binding.
			if challenge.URL != target.URL {
				continue
			}

			matched[i] = true
			server, current, err := manager.ServerForAction(req.Context(), target.ConnectID(), req.User)
			if err != nil {
				return nil, err
			}

			if current.Runtime != types.RuntimeRemote {
				return nil, fmt.Errorf("challenged component is not a remote server")
			}

			if len(challenge.ResourceMetadata)+len(challenge.Scope)+len(challenge.ErrorCode)+len(challenge.ErrorDescription)+len(challenge.Realm)+len(challenge.ErrorURI) > 8192 {
				return nil, fmt.Errorf("upstream challenge too large")
			}

			requirement := v1.UpstreamOAuthRequirement{
				Component:        server.Name,
				Realm:            challenge.Realm,
				ErrorURI:         challenge.ErrorURI,
				ConnectID:        target.ConnectID(),
				URL:              current.URL,
				TargetHash:       mcp.OAuthTargetHash(server, current),
				StatusCode:       challenge.StatusCode,
				ResourceMetadata: challenge.ResourceMetadata,
				Scope:            challenge.Scope,
				ErrorCode:        challenge.ErrorCode,
				ErrorDescription: challenge.ErrorDescription,
			}
			if requirement.TargetHash == "" {
				return nil, fmt.Errorf("invalid upstream target configuration")
			}

			duplicate := slices.ContainsFunc(batch.Requirements, func(existing v1.UpstreamOAuthRequirement) bool { return existing == requirement })
			if !duplicate {
				batch.Requirements = append(batch.Requirements, requirement)
			}
		}
	}

	if slices.Contains(matched, false) {
		return nil, fmt.Errorf("upstream challenge does not belong to this connection")
	}

	if len(batch.Requirements) == 0 {
		return nil, fmt.Errorf("upstream challenges do not belong to this connection")
	}

	if err := req.GatewayClient.CreateMCPOAuthChallenge(req.Context(), &batch); err != nil {
		return nil, err
	}

	response := mcpAuthRequiredResponse(req, false)

	// The caller is authenticated to Obot but needs additional upstream authorization.
	// A scope challenge starts a new grant; a 401 may only refresh the existing token.
	response.status = http.StatusForbidden
	response.headers.Set("WWW-Authenticate", downstreamChallenge(req, client.UpstreamOAuthScopePrefix+batch.ID))

	return response, nil
}

func downstreamChallenge(req api.Context, scope string) string {
	result := mcpAuthChallenge(req)
	if scope != "" {
		result += `, error="insufficient_scope", scope=` + strconv.Quote(scope)
	}

	return result
}

func rewriteUpstreamOAuthResponse(req api.Context, resp *http.Response) error {
	challenges := component.AuthorizationChallenges(resp)

	if len(challenges) == 0 {
		return nil
	}

	if slices.Contains(req.User.GetGroups(), types.GroupCompositeMCP) {
		// Alternatives in one response are not cumulative requirements; each component keeps its own selection.
		// shortcut: use the first supported alternative; add provider selection if users need another issuer.
		if len(challenges) > 1 {
			challenge := challenges[0]
			var params []string
			for key, value := range map[string]string{
				"realm":             challenge.Realm,
				"error_uri":         challenge.ErrorURI,
				"resource_metadata": challenge.ResourceMetadata,
				"scope":             challenge.Scope,
				"error":             challenge.ErrorCode,
				"error_description": challenge.ErrorDescription,
			} {
				if value != "" {
					params = append(params, key+"="+strconv.Quote(value))
				}
			}

			slices.Sort(params)
			resp.Header.Set("WWW-Authenticate", strings.TrimSpace("Bearer "+strings.Join(params, ", ")))
		}

		return nil
	}

	// The authenticated Obot loopback may request first-level authentication
	// without a ticket when its token expires; preserve that as an Obot 401.
	scope := challenges[0].Scope
	if scope != "" && (!strings.HasPrefix(scope, client.UpstreamOAuthScopePrefix) || len(scope) > 128) {
		return fmt.Errorf("invalid composite authorization challenge")
	}

	response := mcpAuthRequiredResponse(req, false)
	if scope != "" {
		response.status = http.StatusForbidden
	}

	response.headers.Set("WWW-Authenticate", downstreamChallenge(req, scope))

	replaceMCPAuthResponse(resp, response)

	return nil
}
