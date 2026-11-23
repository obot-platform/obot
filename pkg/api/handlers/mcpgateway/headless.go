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
	return conflictJSONResponse(body)
}

// conflictJSONResponse is a 409 response with a JSON body.
func conflictJSONResponse(body []byte) *mcpResponse {
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

// bareUnauthorizedResponse is a 401 without a challenge. At a component hop, the composite reports it as the
// component's authorization failure (compositeAuthorizationResponse).
func bareUnauthorizedResponse() *mcpResponse {
	return mcpTextResponse(http.StatusUnauthorized, "MCP server requires authentication")
}

// agentFailure returns failure, about the vMCP with vmcpID, as the agent that calls req is told it: as it is, or for an
// agent acting on behalf of a user, naming the vMCP when it is known, and with what the user can set up reported as
// needing their approval (vmcpaccess.AgentReadinessFailure.ForUser). At a component of a vMCP, the caller is the
// composite loopback, which acts on behalf of the same user.
func agentFailure(req api.Context, vmcpID string, failure vmcpaccess.AgentReadinessFailure) vmcpaccess.AgentReadinessFailure {
	if _, ok := principal.OnBehalfOf(req.User); ok {
		failure.VMCPID = vmcpID
		return failure.ForUser()
	}
	return failure
}

// agentConfigurationRequiredResponse is the 409 response to an agent whose connection to a vMCP component lacks
// required configuration. Connect-time readiness normally refuses the agent first; this covers configuration that
// goes missing in between.
func agentConfigurationRequiredResponse(req api.Context, componentID string, missing []string) *mcpResponse {
	failure := vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentConfigurationRequiredError,
		Reasons: vmcpaccess.ConfigurationRequiredReasons(componentID, missing),
	}
	return agentReadinessResponse(agentFailure(req, "", failure))
}

// agentOAuthRequiredResponse is the 409 response to an agent whose vMCP components ask for OAuth authorization, in
// place of the challenge a user gets. It creates no MCPOAuthChallenge, and every challenge must belong to one of the
// connection's components, as for a user.
func agentOAuthRequiredResponse(req api.Context, cfg mcp.ServerConfig, challenges []*component.AuthorizationError) (*mcpResponse, error) {
	challenged, err := challengedComponents(cfg, challenges)
	if err != nil {
		return nil, err
	}
	failure := vmcpaccess.AgentReadinessFailure{Error: vmcpaccess.AgentOAuthRequiredError}
	for _, target := range challenged {
		reason := vmcpaccess.ReadinessReason{ComponentID: target.ComponentID, Code: types.AgentReadinessOAuthRequired}
		if target.OAuthGrant != nil {
			// The connection used the vMCP's grant, so the upstream refused that grant, or refused it more scopes than
			// it was authorized with. Only an administrator can authorize it again, on the vMCP.
			reason.Code = types.AgentReadinessOAuthInvalid
		}
		if !slices.Contains(failure.Reasons, reason) {
			failure.Reasons = append(failure.Reasons, reason)
		}
	}
	return agentReadinessResponse(agentFailure(req, cfg.AuditLogMetadata[mcp.AuditLogVMCPID], failure)), nil
}

// challengedComponents returns the components of cfg that challenges came from, in configuration order rather than
// goroutine completion order, to keep responses stable. Every challenge must belong to one of the connection's
// components: vMCP URLs are Obot loopback routes containing the component connection ID.
func challengedComponents(cfg mcp.ServerConfig, challenges []*component.AuthorizationError) ([]mcp.ComponentServer, error) {
	matched := make([]bool, len(challenges))
	var challenged []mcp.ComponentServer
	for _, target := range cfg.Components {
		var hit bool
		for i, challenge := range challenges {
			if challenge.URL == target.URL {
				matched[i], hit = true, true
			}
		}
		if hit {
			challenged = append(challenged, target)
		}
	}
	if slices.Contains(matched, false) {
		return nil, fmt.Errorf("upstream challenge does not belong to this connection")
	}
	return challenged, nil
}

// rewriteHeadlessAuthorizationResponse replaces an authorization challenge in resp, the response to a headless
// caller, so that the caller never receives one.
//
// At a component of a vMCP, the caller is the composite loopback acting for an agent: the component's challenge
// becomes a bare 401, which the composite still reports as the component's authorization failure, and answers with a
// 409 that names the component (compositeAuthorizationResponse). At the outer gateway, a 401 means the loopback was
// refused, and stays a plain 401; a scope challenge, which no composite issues for an agent, becomes a 409.
func rewriteHeadlessAuthorizationResponse(req api.Context, resp *http.Response) {
	response := bareUnauthorizedResponse()
	if !principal.IsComponentHop(req.User) && resp.StatusCode != http.StatusUnauthorized {
		response = agentReadinessResponse(vmcpaccess.AgentReadinessFailure{Error: vmcpaccess.AgentOAuthRequiredError})
	}
	replaceMCPAuthResponse(resp, response)
}
