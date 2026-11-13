package mcpgateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/obot-platform/mmmcp/component"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
)

// headlessCaller reports whether the caller of req is an agent, or presents an agent's credential. An agent cannot
// complete an interactive OAuth flow, so it is never challenged: it gets no WWW-Authenticate header, and no
// MCPOAuthChallenge row or pending OAuth state is created for it. That holds for a composite loopback that acts for
// an agent, which carries the agent's kind, and for a request whose agent credential was refused, which is not
// authenticated as anyone but must not be sent into a login flow either.
func headlessCaller(req api.Context) bool {
	return principal.ClaimsAgent(req.User) || presentsAgentCredential(req.Request)
}

// presentsAgentCredential reports whether r carries an agent's API key, or a token that claims to be an agent's
// (persistent.ClaimsAgentToken, the classifier the no-auth agent token guard uses), where the API key and token
// authenticators read them. The token is read without verifying it: it only decides that a challenge is withheld,
// never that anything is granted.
func presentsAgentCredential(r *http.Request) bool {
	if r == nil {
		return false
	}
	credential := persistent.RequestToken(r)
	return client.IsAgentAPIKey(credential) || persistent.ClaimsAgentToken(credential)
}

// agentReadinessResponse is the 409 response to an agent that cannot use a vMCP, with the reasons why. It never
// carries WWW-Authenticate.
func agentReadinessResponse(failure vmcpaccess.AgentReadinessFailure) *mcpResponse {
	if failure.Reasons == nil {
		failure.Reasons = []vmcpaccess.ReadinessReason{}
	}
	body, err := json.Marshal(failure)
	if err != nil {
		// A failure is only strings, so this cannot happen; answer with the error code alone if it does.
		body = fmt.Appendf(nil, `{"error":%q,"reasons":[]}`, failure.Error)
	}
	return &mcpResponse{
		status: http.StatusConflict,
		headers: http.Header{
			"Content-Length":         nil,
			"Content-Type":           {"application/json"},
			"X-Content-Type-Options": {"nosniff"},
		},
		body: append(body, '\n'),
	}
}

// agentConfigurationRequiredResponse is the 409 response to an agent whose connection to a vMCP component lacks
// required configuration. Connect-time readiness normally refuses the agent first; this covers configuration that
// goes missing in between.
func agentConfigurationRequiredResponse(componentID string, missing []string) *mcpResponse {
	failure := vmcpaccess.AgentReadinessFailure{Error: vmcpaccess.AgentConfigurationRequiredError}
	for _, field := range missing {
		failure.Reasons = append(failure.Reasons, vmcpaccess.ReadinessReason{
			ComponentID: componentID,
			Code:        types.AgentReadinessConfigurationRequired,
			Field:       field,
		})
	}
	return agentReadinessResponse(failure)
}

// agentOAuthRequiredResponse is the 409 response to an agent whose vMCP components ask for OAuth authorization, in
// place of the challenge a user gets. It creates no MCPOAuthChallenge, and every challenge must belong to one of the
// connection's components, as for a user.
func agentOAuthRequiredResponse(cfg mcp.ServerConfig, challenges []*component.AuthorizationError) (*mcpResponse, error) {
	failure := vmcpaccess.AgentReadinessFailure{Error: vmcpaccess.AgentOAuthRequiredError}
	matched := make([]bool, len(challenges))
	// Walk configuration order, not goroutine completion order, to keep the reasons stable.
	for _, target := range cfg.Components {
		for i, challenge := range challenges {
			// vMCP URLs are Obot loopback routes containing the component connection ID.
			if challenge.URL != target.URL {
				continue
			}
			matched[i] = true
			reason := vmcpaccess.ReadinessReason{ComponentID: target.ComponentID, Code: types.AgentReadinessOAuthRequired}
			if target.OAuthGrant != nil {
				// The connection used the grant of the agent's profile, so the upstream refused that grant, or refused it
				// more scopes than it was authorized with. Only an administrator can authorize it again; the agent never
				// falls back from it to another profile's grant.
				reason = vmcpaccess.ReadinessReason{ComponentID: target.ComponentID, Code: types.AgentReadinessOAuthInvalid, ProfileID: target.OAuthGrant.ProfileID}
			}
			if !slices.Contains(failure.Reasons, reason) {
				failure.Reasons = append(failure.Reasons, reason)
			}
		}
	}
	if slices.Contains(matched, false) {
		return nil, fmt.Errorf("upstream challenge does not belong to this connection")
	}
	return agentReadinessResponse(failure), nil
}

// agentOAuthGrantInvalidResponse is the response to an agent whose connection to a vMCP component would use an OAuth
// grant of its profile that no longer works (mcp.AgentOAuthGrantError, mcp.ErrOAuthGrantInvalid). At the component hop,
// the caller is the composite loopback acting for the agent, so the response is a bare 401, as for a component that
// challenges an agent (rewriteHeadlessAuthorizationResponse): the composite reports it as the component's
// authorization failure, and answers the agent with a 409 that names the component and the grant as invalid
// (agentOAuthRequiredResponse). Anywhere else the agent is answered with that 409 directly. Neither carries
// WWW-Authenticate, and no challenge is created.
func agentOAuthGrantInvalidResponse(req api.Context, componentID, profileID, urlProfileID string) *mcpResponse {
	if principal.IsComponentHop(req.User) {
		return mcpTextResponse(http.StatusUnauthorized, "MCP server requires authentication")
	}
	return agentReadinessResponse(vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentOAuthRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: componentID, Code: types.AgentReadinessOAuthInvalid, ProfileID: profileID, URLProfileID: urlProfileID}},
	})
}

// rewriteHeadlessAuthorizationResponse replaces an authorization challenge in resp, the response to a headless
// caller, so that the caller never receives one.
//
// At a component of a vMCP, the caller is the composite loopback acting for an agent: the component's challenge
// becomes a bare 401, which the composite still reports as the component's authorization failure, and answers with a
// 409 that names the component (compositeAuthorizationResponse). At the outer gateway, a 401 means the loopback was
// refused, and stays a plain 401; a scope challenge, which no composite issues for an agent, becomes a 409.
func rewriteHeadlessAuthorizationResponse(req api.Context, resp *http.Response) {
	response := mcpTextResponse(http.StatusUnauthorized, "MCP server requires authentication")
	if !principal.IsComponentHop(req.User) && resp.StatusCode != http.StatusUnauthorized {
		response = agentReadinessResponse(vmcpaccess.AgentReadinessFailure{Error: vmcpaccess.AgentOAuthRequiredError})
	}
	replaceMCPAuthResponse(resp, response)
}
