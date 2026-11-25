package mcpgateway

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/obot-platform/mmmcp/component"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
)

const (
	// preconfiguredOAuthInvalidError is the error of the 409 response to a user whose vMCP connection reaches a
	// component whose preconfigured OAuth no longer works (preconfiguredOAuthFailure).
	preconfiguredOAuthInvalidError = "preconfigured_oauth_invalid"
)

// preconfiguredOAuthFailure is the body of the 409 response to a user whose vMCP connection reaches components whose
// preconfigured OAuth no longer works. Only an administrator can fix it, by authorizing the components again (or
// revoking them, so that users authorize them themselves), so the user is told so rather than asked to authorize the
// components with an account of their own, which would not be used.
type preconfiguredOAuthFailure struct {
	Error        string   `json:"error"`
	Message      string   `json:"message"`
	ComponentIDs []string `json:"componentIDs"`
}

// preconfiguredOAuthInvalidResponse is the response to a caller whose connection to a vMCP component would use the
// vMCP's preconfigured OAuth for the component, which no longer works (mcp.PreconfiguredOAuthError, which only agents
// get, or mcp.ErrOAuthGrantInvalid). No challenge is created, and the response never carries WWW-Authenticate.
//
// At the component hop, the caller is the composite loopback acting for a user or an agent, so the response is a bare
// 401, as for a component that challenges an agent (rewriteHeadlessAuthorizationResponse): the composite reports it as
// the component's authorization failure, and, since its configuration names the component's grant
// (mcp.ComponentServer.OAuthGrant), answers the agent with a 409 that names the grant as invalid
// (agentOAuthRequiredResponse), and the user with a 409 that says an administrator must authorize it again
// (userPreconfiguredOAuthResponse). Anywhere else the caller is answered with that 409 directly.
func preconfiguredOAuthInvalidResponse(req api.Context, componentID string) *mcpResponse {
	if principal.IsComponentHop(req.User) {
		return bareUnauthorizedResponse()
	}
	if headlessCaller(req) {
		return agentReadinessResponse(vmcpaccess.AgentReadinessFailure{
			Error:   vmcpaccess.AgentOAuthRequiredError,
			Reasons: []vmcpaccess.ReadinessReason{{ComponentID: componentID, Code: types.AgentReadinessOAuthInvalid}},
		})
	}
	return preconfiguredOAuthFailureResponse([]string{componentID}, []string{componentID})
}

// userPreconfiguredOAuthResponse is the response to a user, through cfg, their vMCP connection, whose components
// answered with challenges, when any challenged component uses the vMCP's preconfigured OAuth
// (mcp.ComponentServer.OAuthGrant): the upstream refused the preconfigured account, or the component hop found that
// the grant no longer works. The user did not authorize those components and cannot: it is a 409 that says an
// administrator must authorize them again, and no challenge is retained, for them or for the other challenged
// components, which the user is asked to authorize once the vMCP's own are fixed. It returns nil when no challenged
// component uses preconfigured OAuth, so that the user is challenged as before. Every challenge must belong to one of
// the connection's components.
func userPreconfiguredOAuthResponse(cfg mcp.ServerConfig, challenges []*component.AuthorizationError) (*mcpResponse, error) {
	challenged, err := challengedComponents(cfg, challenges)
	if err != nil {
		return nil, err
	}
	var ids, names []string
	for _, target := range challenged {
		if target.OAuthGrant != nil && !slices.Contains(ids, target.ComponentID) {
			ids = append(ids, target.ComponentID)
			names = append(names, cmp.Or(target.DisplayName, target.ComponentID))
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return preconfiguredOAuthFailureResponse(ids, names), nil
}

// preconfiguredOAuthFailureResponse is the 409 response to a user whose vMCP components with ids, named names, use
// preconfigured OAuth that no longer works (preconfiguredOAuthFailure).
func preconfiguredOAuthFailureResponse(ids, names []string) *mcpResponse {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, strconv.Quote(name))
	}
	body, err := json.Marshal(preconfiguredOAuthFailure{
		Error: preconfiguredOAuthInvalidError,
		Message: fmt.Sprintf("The OAuth account an administrator preconfigured for %s on this vMCP no longer works. "+
			"An administrator must authorize it again.", strings.Join(quoted, ", ")),
		ComponentIDs: ids,
	})
	if err != nil {
		// A failure is only strings, so this cannot happen; answer with the error code alone if it does.
		body = fmt.Appendf(nil, `{"error":%q,"componentIDs":[]}`, preconfiguredOAuthInvalidError)
	}
	return conflictJSONResponse(body)
}
