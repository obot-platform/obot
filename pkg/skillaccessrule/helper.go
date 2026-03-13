package skillaccessrule

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

const (
	SkillIDIndex          = "skill-ids"
	RepositoryIDIndex     = "repository-ids"
	ResourceSelectorIndex = "selectors"
	UserIDIndex           = "user-ids"
	GroupIDIndex          = "group-ids"
	SubjectSelectorIndex  = "subject-selectors"
)

type Helper struct {
	sarIndexer gocache.Indexer
}

func NewHelper(sarIndexer gocache.Indexer) *Helper {
	return &Helper{
		sarIndexer: sarIndexer,
	}
}

func (h *Helper) GetSkillAccessRulesForSkill(namespace, skillID string) ([]v1.SkillAccessRule, error) {
	return h.getIndexedRules(namespace, SkillIDIndex, skillID, "skill")
}

func (h *Helper) GetSkillAccessRulesForRepository(namespace, repoID string) ([]v1.SkillAccessRule, error) {
	return h.getIndexedRules(namespace, RepositoryIDIndex, repoID, "repository")
}

func (h *Helper) GetSkillAccessRulesForSelector(namespace, selector string) ([]v1.SkillAccessRule, error) {
	return h.getIndexedRules(namespace, ResourceSelectorIndex, selector, "selector")
}

func (h *Helper) GetSkillAccessRulesForUser(namespace, userID string) ([]v1.SkillAccessRule, error) {
	return h.getIndexedRules(namespace, UserIDIndex, userID, "user")
}

func (h *Helper) GetSkillAccessRulesForGroup(namespace, groupID string) ([]v1.SkillAccessRule, error) {
	return h.getIndexedRules(namespace, GroupIDIndex, groupID, "group")
}

func (h *Helper) GetSkillAccessRulesForSubjectSelector(namespace, selector string) ([]v1.SkillAccessRule, error) {
	return h.getIndexedRules(namespace, SubjectSelectorIndex, selector, "subject selector")
}

func (h *Helper) UserHasAccessToSkill(user kuser.Info, skill *v1.Skill) (bool, error) {
	if skill == nil {
		return false, nil
	}

	return h.UserHasAccessToSkillID(user, skill.Name, skill.Spec.RepoID)
}

func (h *Helper) UserHasAccessToSkillID(user kuser.Info, skillID, repoID string) (bool, error) {
	if skillID == "" && repoID == "" {
		return false, nil
	}

	var (
		userID string
		groups = authGroupSet(user)
	)
	if user != nil {
		userID = user.GetUID()
	}

	selectorRules, err := h.GetSkillAccessRulesForSelector(system.DefaultNamespace, "*")
	if err != nil {
		return false, err
	}
	if hasMatchingSubject(selectorRules, userID, groups) {
		return true, nil
	}

	if repoID != "" {
		repoRules, err := h.GetSkillAccessRulesForRepository(system.DefaultNamespace, repoID)
		if err != nil {
			return false, err
		}
		if hasMatchingSubject(repoRules, userID, groups) {
			return true, nil
		}
	}

	if skillID != "" {
		skillRules, err := h.GetSkillAccessRulesForSkill(system.DefaultNamespace, skillID)
		if err != nil {
			return false, err
		}
		if hasMatchingSubject(skillRules, userID, groups) {
			return true, nil
		}
	}

	return false, nil
}

func (h *Helper) GetUserSkillAccessScope(user kuser.Info) (bool, map[string]struct{}, map[string]struct{}, error) {
	repoIDs := map[string]struct{}{}
	skillIDs := map[string]struct{}{}

	rules, err := h.getRulesForUser(system.DefaultNamespace, user)
	if err != nil {
		return false, nil, nil, err
	}

	for _, rule := range rules {
		for _, resource := range rule.Spec.Manifest.Resources {
			switch resource.Type {
			case types.SkillResourceTypeSelector:
				if resource.ID == "*" {
					return true, repoIDs, skillIDs, nil
				}
			case types.SkillResourceTypeSkillRepository:
				if resource.ID != "" {
					repoIDs[resource.ID] = struct{}{}
				}
			case types.SkillResourceTypeSkill:
				if resource.ID != "" {
					skillIDs[resource.ID] = struct{}{}
				}
			}
		}
	}

	return false, repoIDs, skillIDs, nil
}

func (h *Helper) getRulesForUser(namespace string, user kuser.Info) ([]v1.SkillAccessRule, error) {
	result := make([]v1.SkillAccessRule, 0)
	seen := map[string]struct{}{}

	addRules := func(rules []v1.SkillAccessRule) {
		for _, rule := range rules {
			if _, ok := seen[rule.Name]; ok {
				continue
			}
			seen[rule.Name] = struct{}{}
			result = append(result, rule)
		}
	}

	selectorRules, err := h.GetSkillAccessRulesForSubjectSelector(namespace, "*")
	if err != nil {
		return nil, err
	}
	addRules(selectorRules)

	if user != nil && user.GetUID() != "" {
		userRules, err := h.GetSkillAccessRulesForUser(namespace, user.GetUID())
		if err != nil {
			return nil, err
		}
		addRules(userRules)
	}

	for groupID := range authGroupSet(user) {
		groupRules, err := h.GetSkillAccessRulesForGroup(namespace, groupID)
		if err != nil {
			return nil, err
		}
		addRules(groupRules)
	}

	return result, nil
}

func (h *Helper) getIndexedRules(namespace, indexName, key, target string) ([]v1.SkillAccessRule, error) {
	rules, err := h.sarIndexer.ByIndex(indexName, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get skill access rules for %s: %w", target, err)
	}

	result := make([]v1.SkillAccessRule, 0, len(rules))
	for _, rule := range rules {
		res, ok := rule.(*v1.SkillAccessRule)
		if ok && res.Namespace == namespace && res.DeletionTimestamp == nil {
			result = append(result, *res)
		}
	}

	return result, nil
}

func hasMatchingSubject(rules []v1.SkillAccessRule, userID string, groups map[string]struct{}) bool {
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
