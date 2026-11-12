package cleanup

import (
	"github.com/obot-platform/nah/pkg/router"
)

// DeleteAgentIdentityEvent deletes a delivered AgentIdentityChange without acting on it, so that changes do not pile
// up. Disabling a user deletes their auth tokens, Obot-issued MCP OAuth refresh tokens and browser sessions, none of
// which an agent can hold, so disabling an agent leaves nothing to remove: authentication refuses the disabled agent
// on its next request, and enabling it again needs no repair. Controllers that depend on an agent's lifecycle, such
// as vmcpinstance.DeleteUnauthorized, watch these objects and read the agent's current status.
func DeleteAgentIdentityEvent(req router.Request, _ router.Response) error {
	return req.Delete(req.Object)
}
