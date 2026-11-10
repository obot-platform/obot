package cleanup

import (
	"github.com/obot-platform/nah/pkg/router"
)

// DeleteAgentIdentityEvent deletes a delivered AgentIdentityChange or AgentIdentityDelete without acting on it, so
// that they do not pile up while nothing handles them. Disabled agents are already denied by authentication, and
// nothing is owned by agents yet. The agent cleanup of agent identities spec 06 replaces this route.
func DeleteAgentIdentityEvent(req router.Request, _ router.Response) error {
	return req.Delete(req.Object)
}
