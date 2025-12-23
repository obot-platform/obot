package modelpermissionrule

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

type Helper struct {
	mprIndexer gocache.Indexer
}

func NewHelper(mprIndexer gocache.Indexer) *Helper {
	return &Helper{
		mprIndexer: mprIndexer,
	}
}

// GetModelPermissionRulesForUser returns all ModelPermissionRules that apply to a specific user
func (h *Helper) GetModelPermissionRulesForUser(namespace, userID string) ([]v1.ModelPermissionRule, error) {
	mprs, err := h.mprIndexer.ByIndex("user-ids", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model permission rules for user: %w", err)
	}

	result := make([]v1.ModelPermissionRule, 0, len(mprs))
	for _, mpr := range mprs {
		res, ok := mpr.(*v1.ModelPermissionRule)
		if ok && res.Namespace == namespace && res.DeletionTimestamp.IsZero() {
			result = append(result, *res)
		}
	}

	return result, nil
}

// GetModelPermissionRulesForModel returns all ModelPermissionRules that contain the specified model ID
func (h *Helper) GetModelPermissionRulesForModel(namespace, modelID string) ([]v1.ModelPermissionRule, error) {
	mprs, err := h.mprIndexer.ByIndex("model-ids", modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model permission rules for model: %w", err)
	}

	result := make([]v1.ModelPermissionRule, 0, len(mprs))
	for _, mpr := range mprs {
		res, ok := mpr.(*v1.ModelPermissionRule)
		if ok && res.Namespace == namespace && res.DeletionTimestamp.IsZero() {
			result = append(result, *res)
		}
	}

	return result, nil
}

// GetModelPermissionRulesWithWildcardModel returns all ModelPermissionRules that have the wildcard model selector
func (h *Helper) GetModelPermissionRulesWithWildcardModel(namespace string) ([]v1.ModelPermissionRule, error) {
	mprs, err := h.mprIndexer.ByIndex("model-ids", "*")
	if err != nil {
		return nil, fmt.Errorf("failed to get model permission rules with wildcard: %w", err)
	}

	result := make([]v1.ModelPermissionRule, 0, len(mprs))
	for _, mpr := range mprs {
		res, ok := mpr.(*v1.ModelPermissionRule)
		if ok && res.Namespace == namespace && res.DeletionTimestamp.IsZero() {
			result = append(result, *res)
		}
	}

	return result, nil
}

// GetModelPermissionRulesWithWildcardSubject returns all ModelPermissionRules that have the wildcard subject selector
func (h *Helper) GetModelPermissionRulesWithWildcardSubject(namespace string) ([]v1.ModelPermissionRule, error) {
	mprs, err := h.mprIndexer.ByIndex("subject-selectors", "*")
	if err != nil {
		return nil, fmt.Errorf("failed to get model permission rules with wildcard subject: %w", err)
	}

	result := make([]v1.ModelPermissionRule, 0, len(mprs))
	for _, mpr := range mprs {
		res, ok := mpr.(*v1.ModelPermissionRule)
		if ok && res.Namespace == namespace && res.DeletionTimestamp.IsZero() {
			result = append(result, *res)
		}
	}

	return result, nil
}

// UserHasAccessToModel checks if a user has access to a specific model through ModelPermissionRules.
// Returns true if:
// - User is admin or owner (always has access)
// - User is in a rule that has wildcard model selector (*)
// - User is in a rule that explicitly includes the model
// - There is a wildcard subject selector (*) that grants access to the model
func (h *Helper) UserHasAccessToModel(user kuser.Info, modelID string) (bool, error) {
	if userIsAdminOrOwner(user) {
		return true, nil
	}

	var (
		userID = user.GetUID()
		groups = authGroupSet(user)
	)

	// Check rules with wildcard subject selector (*) that include the model
	wildcardSubjectRules, err := h.GetModelPermissionRulesWithWildcardSubject(system.DefaultNamespace)
	if err != nil {
		return false, err
	}

	for _, rule := range wildcardSubjectRules {
		// Check if any subject is a wildcard selector
		for _, subject := range rule.Spec.Manifest.Subjects {
			if subject.Type == types.SubjectTypeSelector && subject.ID == "*" {
				// Check if the rule includes the model or has wildcard model
				for _, model := range rule.Spec.Manifest.Models {
					if model.IsWildcard() || model.ModelID == modelID {
						return true, nil
					}
				}
			}
		}
	}

	// Check rules that the user is directly included in
	userRules, err := h.GetModelPermissionRulesForUser(system.DefaultNamespace, userID)
	if err != nil {
		return false, err
	}

	for _, rule := range userRules {
		// Check if user is a subject in this rule
		for _, subject := range rule.Spec.Manifest.Subjects {
			if subject.Type == types.SubjectTypeUser && subject.ID == userID {
				// Check if the rule includes the model or has wildcard model
				for _, model := range rule.Spec.Manifest.Models {
					if model.IsWildcard() || model.ModelID == modelID {
						return true, nil
					}
				}
			}
		}
	}

	// Check rules based on group membership
	for groupID := range groups {
		groupRules, err := h.getModelPermissionRulesForGroup(system.DefaultNamespace, groupID)
		if err != nil {
			return false, err
		}

		for _, rule := range groupRules {
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == types.SubjectTypeGroup && subject.ID == groupID {
					// Check if the rule includes the model or has wildcard model
					for _, model := range rule.Spec.Manifest.Models {
						if model.IsWildcard() || model.ModelID == modelID {
							return true, nil
						}
					}
				}
			}
		}
	}

	return false, nil
}

// GetAllowedModelsForUser returns all model IDs that a user has access to.
// Returns nil if the user has wildcard access to all models.
// This is useful for filtering model lists in the UI.
func (h *Helper) GetAllowedModelsForUser(user kuser.Info) ([]string, bool, error) {
	if userIsAdminOrOwner(user) {
		// Admin/owner has access to all models
		return nil, true, nil
	}

	var (
		userID       = user.GetUID()
		groups       = authGroupSet(user)
		allowedSet   = make(map[string]struct{})
		hasWildcard  = false
	)

	// Check rules with wildcard subject selector (*)
	wildcardSubjectRules, err := h.GetModelPermissionRulesWithWildcardSubject(system.DefaultNamespace)
	if err != nil {
		return nil, false, err
	}

	for _, rule := range wildcardSubjectRules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			if subject.Type == types.SubjectTypeSelector && subject.ID == "*" {
				for _, model := range rule.Spec.Manifest.Models {
					if model.IsWildcard() {
						return nil, true, nil
					}
					allowedSet[model.ModelID] = struct{}{}
				}
			}
		}
	}

	// Check rules that the user is directly included in
	userRules, err := h.GetModelPermissionRulesForUser(system.DefaultNamespace, userID)
	if err != nil {
		return nil, false, err
	}

	for _, rule := range userRules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			if subject.Type == types.SubjectTypeUser && subject.ID == userID {
				for _, model := range rule.Spec.Manifest.Models {
					if model.IsWildcard() {
						return nil, true, nil
					}
					allowedSet[model.ModelID] = struct{}{}
				}
			}
		}
	}

	// Check rules based on group membership
	for groupID := range groups {
		groupRules, err := h.getModelPermissionRulesForGroup(system.DefaultNamespace, groupID)
		if err != nil {
			return nil, false, err
		}

		for _, rule := range groupRules {
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == types.SubjectTypeGroup && subject.ID == groupID {
					for _, model := range rule.Spec.Manifest.Models {
						if model.IsWildcard() {
							return nil, true, nil
						}
						allowedSet[model.ModelID] = struct{}{}
					}
				}
			}
		}
	}

	// Convert set to slice
	allowed := make([]string, 0, len(allowedSet))
	for modelID := range allowedSet {
		allowed = append(allowed, modelID)
	}

	return allowed, hasWildcard, nil
}

// getModelPermissionRulesForGroup returns all ModelPermissionRules that apply to a specific group
func (h *Helper) getModelPermissionRulesForGroup(namespace, groupID string) ([]v1.ModelPermissionRule, error) {
	mprs, err := h.mprIndexer.ByIndex("group-ids", groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model permission rules for group: %w", err)
	}

	result := make([]v1.ModelPermissionRule, 0, len(mprs))
	for _, mpr := range mprs {
		res, ok := mpr.(*v1.ModelPermissionRule)
		if ok && res.Namespace == namespace && res.DeletionTimestamp.IsZero() {
			result = append(result, *res)
		}
	}

	return result, nil
}

func authGroupSet(user kuser.Info) map[string]struct{} {
	groups := user.GetExtra()["auth_provider_groups"]
	set := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		set[group] = struct{}{}
	}
	return set
}

func userIsAdminOrOwner(user kuser.Info) bool {
	for _, group := range user.GetGroups() {
		switch group {
		case types.GroupAdmin, types.GroupOwner:
			return true
		}
	}
	return false
}
