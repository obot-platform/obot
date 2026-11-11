package skillaccessrule

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

const (
	SkillIDIndex          = "skill-ids"
	RepositoryIDIndex     = "repository-ids"
	ResourceSelectorIndex = "selectors"
)

type Helper struct {
	sarIndexer gocache.Indexer
}

func NewHelper(sarIndexer gocache.Indexer) *Helper {
	return &Helper{
		sarIndexer: sarIndexer,
	}
}

// Indexers returns the indexes of skill access rules that Helper looks rules up in, for the informer whose indexer
// it is given: by resource, and by subject (subjects.Indexers).
func Indexers() gocache.Indexers {
	resourceIndex := func(resourceType types.SkillResourceType) gocache.IndexFunc {
		return func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == resourceType {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		}
	}
	indexers := subjects.Indexers(func(obj any) ([]types.Subject, bool) {
		return obj.(*v1.SkillAccessRule).Spec.Manifest.Subjects, true
	})
	indexers[SkillIDIndex] = resourceIndex(types.SkillResourceTypeSkill)
	indexers[RepositoryIDIndex] = resourceIndex(types.SkillResourceTypeSkillRepository)
	indexers[ResourceSelectorIndex] = resourceIndex(types.SkillResourceTypeSelector)
	return indexers
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

func (h *Helper) GetSkillAccessRulesForGroup(namespace, groupID string) ([]v1.SkillAccessRule, error) {
	return h.getIndexedRules(namespace, subjects.GroupIndex, groupID, "group")
}

func (h *Helper) UserHasAccessToSkill(user kuser.Info, skill *v1.Skill) (bool, error) {
	if skill == nil {
		return false, nil
	}

	return h.UserHasAccessToSkillID(user, skill.Name, skill.Spec.RepoID)
}

// UserHasAccessToSkillID reports whether a rule that covers the skill, its repository, or every skill names the
// caller among its subjects (subjects.Caller.Best). Users match through their ID, every user, everyone, and their
// groups; agents only through their ID, every agent, and everyone. A caller that is not a well-formed user or agent
// principal has access to no skill.
func (h *Helper) UserHasAccessToSkillID(user kuser.Info, skillID, repoID string) (bool, error) {
	if skillID == "" && repoID == "" {
		return false, nil
	}

	caller, ok := subjects.CallerFromUserInfo(user)
	if !ok {
		return false, nil
	}

	selectorRules, err := h.GetSkillAccessRulesForSelector(system.DefaultNamespace, "*")
	if err != nil {
		return false, err
	}
	if hasMatchingSubject(selectorRules, caller) {
		return true, nil
	}

	if repoID != "" {
		repoRules, err := h.GetSkillAccessRulesForRepository(system.DefaultNamespace, repoID)
		if err != nil {
			return false, err
		}
		if hasMatchingSubject(repoRules, caller) {
			return true, nil
		}
	}

	if skillID != "" {
		skillRules, err := h.GetSkillAccessRulesForSkill(system.DefaultNamespace, skillID)
		if err != nil {
			return false, err
		}
		if hasMatchingSubject(skillRules, caller) {
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

// getRulesForUser returns the rules whose subjects name the caller, each once, as UserHasAccessToSkillID matches
// them. A caller that is not a well-formed user or agent principal gets none.
func (h *Helper) getRulesForUser(namespace string, user kuser.Info) ([]v1.SkillAccessRule, error) {
	result := make([]v1.SkillAccessRule, 0)
	caller, ok := subjects.CallerFromUserInfo(user)
	if !ok {
		return result, nil
	}

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

	if err := subjects.LookupIndexes(caller, func(index, key string) error {
		rules, err := h.getIndexedRules(namespace, index, key, index+" "+key)
		if err != nil {
			return err
		}
		addRules(rules)
		return nil
	}); err != nil {
		return nil, err
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

// hasMatchingSubject reports whether any of rules has a subject that names caller.
func hasMatchingSubject(rules []v1.SkillAccessRule, caller subjects.Caller) bool {
	for _, rule := range rules {
		if caller.Best(rule.Spec.Manifest.Subjects) != subjects.NoMatch {
			return true
		}
	}
	return false
}
