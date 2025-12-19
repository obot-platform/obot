package modelaccesspolicy

import (
	"context"
	"fmt"

	"github.com/obot-platform/nah/pkg/backend"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

const (
	mapUserIndex     = "user-id"
	mapGroupIndex    = "group-id"
	mapSelectorIndex = "selector-id"
)

type Helper struct {
	mapIndexer gocache.Indexer
}

func NewHelper(ctx context.Context, backend backend.Backend) (*Helper, error) {
	// Create indexers for ModelAccessPolicy
	mapGVK, err := backend.GroupVersionKindFor(&v1.ModelAccessPolicy{})
	if err != nil {
		return nil, err
	}

	mapInformer, err := backend.GetInformerForKind(ctx, mapGVK)
	if err != nil {
		return nil, err
	}

	if err := mapInformer.AddIndexers(gocache.Indexers{
		mapUserIndex:     mapSubjectIndexFunc(types.SubjectTypeUser),
		mapGroupIndex:    mapSubjectIndexFunc(types.SubjectTypeGroup),
		mapSelectorIndex: mapSubjectIndexFunc(types.SubjectTypeSelector),
	}); err != nil {
		return nil, err
	}

	return &Helper{
		mapIndexer: mapInformer.GetIndexer(),
	}, nil
}

// UserHasAccessToModel returns true if the user has access to the model.
// Access is granted when:
// - The user is an admin or owner
// - A ModelAccessPolicy with wildcard subject selector (*) includes the model (or uses wildcard model selector)
// - A ModelAccessPolicy directly references the user and includes the model (or uses wildcard model selector)
// - A ModelAccessPolicy references a group the user belongs to and includes the model (or uses wildcard model selector)
func (h *Helper) UserHasAccessToModel(user kuser.Info, modelID string) (bool, error) {
	allowedModels, allowAll, err := h.GetUserAllowedModels(user)
	return allowAll || allowedModels[modelID], err
}

// getUserAllowedModels returns a set of model IDs that a user can access.
// If a user is an owner/admin or has been granted access to all models via a wildcard model selector, this method returns nil and true.
func (h *Helper) GetUserAllowedModels(user kuser.Info) (map[string]bool, bool, error) {
	if userIsAdminOrOwner(user) {
		return nil, true, nil
	}

	allowedModels := make(map[string]bool)

	// Check policies with wildcard subject selector (*)
	wildcardUserPolicies, err := h.getWildcardUserPolicies()
	if err != nil {
		return nil, false, err
	}
	for _, rule := range wildcardUserPolicies {
		for _, model := range rule.Spec.Manifest.Models {
			if model.IsWildcard() {
				return nil, true, nil
			}
			allowedModels[model.ID] = true
		}
	}

	// Check policies that the user is directly included in
	userPolicies, err := h.getUserPolicies(user.GetUID())
	if err != nil {
		return nil, false, err
	}

	for _, rule := range userPolicies {
		for _, model := range rule.Spec.Manifest.Models {
			if model.IsWildcard() {
				return nil, true, nil
			}
			allowedModels[model.ID] = true
		}
	}

	// Check policies based on group membership
	for groupID := range authGroupSet(user) {
		groupPolicies, err := h.getGroupPolicies(groupID)
		if err != nil {
			return nil, false, err
		}

		for _, rule := range groupPolicies {
			for _, model := range rule.Spec.Manifest.Models {
				if model.IsWildcard() {
					return nil, true, nil
				}
				allowedModels[model.ID] = true
			}
		}
	}

	return allowedModels, false, nil
}

// GetModelAccessPolicysForUser returns all policies that apply to a specific user.
func (h *Helper) getUserPolicies(userID string) ([]v1.ModelAccessPolicy, error) {
	return h.getIndexedPolicies(mapUserIndex, userID)
}

// getModelAccessPolicysForGroup returns all policies that apply to given group.
func (h *Helper) getGroupPolicies(groupID string) ([]v1.ModelAccessPolicy, error) {
	return h.getIndexedPolicies(mapGroupIndex, groupID)
}

// getAllUserPolicies returns all policies that apply to all users.
func (h *Helper) getWildcardUserPolicies() ([]v1.ModelAccessPolicy, error) {
	return h.getIndexedPolicies(mapSelectorIndex, "*")
}

// getIndexedPolicies returns all indexed policies for a given index and key.
func (h *Helper) getIndexedPolicies(index, key string) ([]v1.ModelAccessPolicy, error) {
	policies, err := h.mapIndexer.ByIndex(index, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get model access policies with wildcard subject: %w", err)
	}

	result := make([]v1.ModelAccessPolicy, 0, len(policies))
	for _, policy := range policies {
		if res, ok := policy.(*v1.ModelAccessPolicy); ok {
			result = append(result, *res)
		}
	}

	return result, nil
}

// mapSubjectIndexFunc returns a function that indexes policies with the given subject type by subject ID.
func mapSubjectIndexFunc(subjectType types.SubjectType) gocache.IndexFunc {
	return func(obj any) ([]string, error) {
		policy := obj.(*v1.ModelAccessPolicy)
		if !policy.DeletionTimestamp.IsZero() {
			// Drop deleted objects from the index
			return nil, nil
		}

		var (
			subjects = policy.Spec.Manifest.Subjects
			keys     = make([]string, 0, len(subjects))
		)
		for _, subject := range subjects {
			if subject.Type == subjectType {
				keys = append(keys, subject.ID)
			}
		}

		return keys, nil
	}
}

// authGroupSet returns a set of auth provider groups for a given user.
func authGroupSet(user kuser.Info) map[string]struct{} {
	var (
		groups = user.GetExtra()["auth_provider_groups"]
		set    = make(map[string]struct{}, len(groups))
	)
	for _, group := range groups {
		set[group] = struct{}{}
	}
	return set
}

// userIsAdminOrOwner checks if the user is an admin or owner.
func userIsAdminOrOwner(user kuser.Info) bool {
	for _, group := range user.GetGroups() {
		switch group {
		case types.GroupAdmin, types.GroupOwner:
			return true
		}
	}
	return false
}
