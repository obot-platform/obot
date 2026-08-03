package authz

import (
	"net/http"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// revealsSecrets reports whether a route hands back the configured secret
// values rather than the agent's description of itself.
func revealsSecrets(req *http.Request) bool {
	return strings.HasSuffix(req.Pattern, "/reveal")
}

// entersSandbox reports whether a route reaches into a running sandbox rather
// than managing the record of one. A terminal is a live console and
// /agent-connect proxies straight through to whatever the agent serves; both
// are the agent being used, not administered.
func entersSandbox(req *http.Request) bool {
	return strings.HasSuffix(req.Pattern, "/terminal") ||
		strings.HasPrefix(req.Pattern, "/agent-connect/")
}

func (a *Authorizer) checkHostedAgent(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.HostedAgentID == "" {
		return true, nil
	}

	// An administrator manages the agents an installation offers, so reading and
	// editing one is theirs. An auditor's business is what happened, not what
	// the credentials are, so revealing them is not -- and neither role is
	// granted a way to launch one here: that is decided by an access rule, in
	// the create handler.
	if u.IsAdmin || (u.IsAuditor && !revealsSecrets(req)) {
		return true, nil
	}

	var agent v1.HostedAgent
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.HostedAgentID), &agent); err != nil {
		return false, err
	}

	return a.hostedAgentHelper.UserHasAccessToHostedAgent(u, &agent)
}

func (a *Authorizer) checkHostedAgentInstance(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.HostedAgentInstanceID == "" {
		return true, nil
	}

	var instance v1.HostedAgentInstance
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.HostedAgentInstanceID), &instance); err != nil {
		return false, err
	}

	// Seeing that an instance exists, and deleting one, is administration.
	// Attaching to its console or proxying into what it serves is not: that is
	// someone else's working session, and reading or typing into it is not a
	// power either role is given. Both fall through to the ownership check.
	if (u.IsAdmin || u.IsAuditor) && !entersSandbox(req) {
		resources.Authorizated.HostedAgentInstance = &instance
		return true, nil
	}

	// An instance is only reachable by its owner, and only for as long as they
	// still have access to the agent it was created from.
	if instance.Spec.UserID != u.GetUID() {
		return false, nil
	}

	hasAccess, err := a.hostedAgentHelper.UserHasAccessToHostedAgentID(u, instance.Spec.HostedAgentName)
	if err != nil || !hasAccess {
		return false, err
	}

	resources.Authorizated.HostedAgentInstance = &instance
	return true, nil
}
