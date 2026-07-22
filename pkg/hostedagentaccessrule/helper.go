package hostedagentaccessrule

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

const (
	HostedAgentIDIndex    = "hosted-agent-ids"
	ResourceSelectorIndex = "selectors"
	UserIDIndex           = "user-ids"
	GroupIDIndex          = "group-ids"
	SubjectSelectorIndex  = "subject-selectors"
)

type Helper struct {
	haarIndexer gocache.Indexer
}

func NewHelper(haarIndexer gocache.Indexer) *Helper {
	return &Helper{
		haarIndexer: haarIndexer,
	}
}

func (h *Helper) GetHostedAgentAccessRulesForHostedAgent(namespace, hostedAgentID string) ([]v1.HostedAgentAccessRule, error) {
	return h.getIndexedRules(namespace, HostedAgentIDIndex, hostedAgentID, "hosted agent")
}

func (h *Helper) GetHostedAgentAccessRulesForSelector(namespace, selector string) ([]v1.HostedAgentAccessRule, error) {
	return h.getIndexedRules(namespace, ResourceSelectorIndex, selector, "selector")
}

func (h *Helper) GetHostedAgentAccessRulesForUser(namespace, userID string) ([]v1.HostedAgentAccessRule, error) {
	return h.getIndexedRules(namespace, UserIDIndex, userID, "user")
}

func (h *Helper) GetHostedAgentAccessRulesForGroup(namespace, groupID string) ([]v1.HostedAgentAccessRule, error) {
	return h.getIndexedRules(namespace, GroupIDIndex, groupID, "group")
}

func (h *Helper) GetHostedAgentAccessRulesForSubjectSelector(namespace, selector string) ([]v1.HostedAgentAccessRule, error) {
	return h.getIndexedRules(namespace, SubjectSelectorIndex, selector, "subject selector")
}

func (h *Helper) UserHasAccessToHostedAgent(user kuser.Info, agent *v1.HostedAgent) (bool, error) {
	if agent == nil {
		return false, nil
	}

	return h.UserHasAccessToHostedAgentID(user, agent.Name)
}

func (h *Helper) UserHasAccessToHostedAgentID(user kuser.Info, hostedAgentID string) (bool, error) {
	if hostedAgentID == "" {
		return false, nil
	}

	var (
		userID string
		groups = authGroupSet(user)
	)
	if user != nil {
		userID = user.GetUID()
	}

	selectorRules, err := h.GetHostedAgentAccessRulesForSelector(system.DefaultNamespace, "*")
	if err != nil {
		return false, err
	}
	if hasMatchingSubject(selectorRules, userID, groups) {
		return true, nil
	}

	agentRules, err := h.GetHostedAgentAccessRulesForHostedAgent(system.DefaultNamespace, hostedAgentID)
	if err != nil {
		return false, err
	}
	if hasMatchingSubject(agentRules, userID, groups) {
		return true, nil
	}

	return false, nil
}

// GetUserHostedAgentAccessScope returns whether the user has wildcard access to
// all hosted agents, and if not, the set of hosted agent IDs they can reach.
func (h *Helper) GetUserHostedAgentAccessScope(user kuser.Info) (bool, map[string]struct{}, error) {
	hostedAgentIDs := map[string]struct{}{}

	rules, err := h.getRulesForUser(system.DefaultNamespace, user)
	if err != nil {
		return false, nil, err
	}

	for _, rule := range rules {
		for _, resource := range rule.Spec.Manifest.Resources {
			switch resource.Type {
			case types.HostedAgentResourceTypeSelector:
				if resource.ID == "*" {
					return true, hostedAgentIDs, nil
				}
			case types.HostedAgentResourceTypeHostedAgent:
				if resource.ID != "" {
					hostedAgentIDs[resource.ID] = struct{}{}
				}
			}
		}
	}

	return false, hostedAgentIDs, nil
}

func (h *Helper) getRulesForUser(namespace string, user kuser.Info) ([]v1.HostedAgentAccessRule, error) {
	result := make([]v1.HostedAgentAccessRule, 0)
	seen := map[string]struct{}{}

	addRules := func(rules []v1.HostedAgentAccessRule) {
		for _, rule := range rules {
			if _, ok := seen[rule.Name]; ok {
				continue
			}
			seen[rule.Name] = struct{}{}
			result = append(result, rule)
		}
	}

	selectorRules, err := h.GetHostedAgentAccessRulesForSubjectSelector(namespace, "*")
	if err != nil {
		return nil, err
	}
	addRules(selectorRules)

	if user != nil && user.GetUID() != "" {
		userRules, err := h.GetHostedAgentAccessRulesForUser(namespace, user.GetUID())
		if err != nil {
			return nil, err
		}
		addRules(userRules)
	}

	for groupID := range authGroupSet(user) {
		groupRules, err := h.GetHostedAgentAccessRulesForGroup(namespace, groupID)
		if err != nil {
			return nil, err
		}
		addRules(groupRules)
	}

	return result, nil
}

func (h *Helper) getIndexedRules(namespace, indexName, key, target string) ([]v1.HostedAgentAccessRule, error) {
	rules, err := h.haarIndexer.ByIndex(indexName, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get hosted agent access rules for %s: %w", target, err)
	}

	result := make([]v1.HostedAgentAccessRule, 0, len(rules))
	for _, rule := range rules {
		res, ok := rule.(*v1.HostedAgentAccessRule)
		if ok && res.Namespace == namespace && res.DeletionTimestamp == nil {
			result = append(result, *res)
		}
	}

	return result, nil
}

func hasMatchingSubject(rules []v1.HostedAgentAccessRule, userID string, groups map[string]struct{}) bool {
	for _, rule := range rules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			switch subject.Type {
			case types.SubjectTypeUser:
				if subject.ID == userID {
					return true
				}
			case types.SubjectTypeGroup:
				if _, ok := groups[subject.ID]; ok {
					return true
				}
			case types.SubjectTypeSelector:
				if subject.ID == "*" {
					return true
				}
			}
		}
	}

	return false
}

func authGroupSet(user kuser.Info) map[string]struct{} {
	if user == nil {
		return map[string]struct{}{}
	}
	groups := user.GetExtra()["auth_provider_groups"]
	set := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		set[group] = struct{}{}
	}
	return set
}
