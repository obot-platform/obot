package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (a *Authorizer) checkHostedAgent(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.HostedAgentID == "" || u.IsAdmin || u.IsAuditor {
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

	if u.IsAdmin || u.IsAuditor {
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
