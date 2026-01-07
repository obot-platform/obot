package modelaccesspolicy

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/backend"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

const (
	mapUserIndex     = "user-id"
	mapGroupIndex    = "group-id"
	mapSelectorIndex = "selector-id"
	dmaModelIndex    = "model-id"
)

type Helper struct {
	mapIndexer, dmaIndexer gocache.Indexer
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

	// Create indexers for DefaultModelAlias
	dmaGVK, err := backend.GroupVersionKindFor(&v1.DefaultModelAlias{})
	if err != nil {
		return nil, err
	}

	dmaInformer, err := backend.GetInformerForKind(ctx, dmaGVK)
	if err != nil {
		return nil, err
	}

	if err := dmaInformer.AddIndexers(gocache.Indexers{
		dmaModelIndex: dmaModelIndexFunc,
	}); err != nil {
		return nil, err
	}

	return &Helper{
		mapIndexer: mapInformer.GetIndexer(),
		dmaIndexer: dmaInformer.GetIndexer(),
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
	var (
		allowedModels   = make(map[string]bool)
		aliasModels     = h.getAliasModels()
		addAllowedModel = func(model types.ModelResource) bool {
			if model.IsWildcard() {
				return true
			}

			modelID := model.ID
			if alias, isAlias := model.IsDefaultModelAliasRef(); isAlias {
				// The model ID is a default model alias reference (e.g. 'obot://llm')
				// Look up the current model ID and swap it out
				// If we can't find it, modelID will be an empty string, which is handled by the model ID check below
				modelID = aliasModels[alias]
			}

			if system.IsModelID(modelID) {
				allowedModels[modelID] = true
			}

			return false
		}
	)

	// Check policies with wildcard subject selector (*)
	wildcardUserPolicies, err := h.getWildcardUserPolicies()
	if err != nil {
		return nil, false, err
	}
	for _, policy := range wildcardUserPolicies {
		if slices.ContainsFunc(policy.Spec.Manifest.Models, addAllowedModel) {
			return nil, true, nil
		}
	}

	// Check policies that the user is directly included in
	userPolicies, err := h.getUserPolicies(user.GetUID())
	if err != nil {
		return nil, false, err
	}

	for _, policy := range userPolicies {
		if slices.ContainsFunc(policy.Spec.Manifest.Models, addAllowedModel) {
			return nil, true, nil
		}
	}

	// Check policies based on group membership
	for groupID := range authGroupSet(user) {
		groupPolicies, err := h.getGroupPolicies(groupID)
		if err != nil {
			return nil, false, err
		}

		for _, policy := range groupPolicies {
			if slices.ContainsFunc(policy.Spec.Manifest.Models, addAllowedModel) {
				return nil, true, nil
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

// getAliasModels returns a map alias -> model ID for all DefaultModelAliases.
func (h *Helper) getAliasModels() map[string]string {
	var (
		indexed       = h.dmaIndexer.ListIndexFuncValues(dmaModelIndex)
		aliasModelIDs = make(map[string]string, len(indexed))
	)

	for _, v := range indexed {
		alias, model, ok := strings.Cut(v, "/")
		if !ok || !system.IsModelID(model) || types.DefaultModelAliasTypeFromString(alias) == types.DefaultModelAliasTypeUnknown {
			// This is a sanity check since our index function should always generate valid values
			continue
		}

		aliasModelIDs[alias] = model
	}

	return aliasModelIDs
}

// mapSubjectIndexFunc returns a function that ModelAccessPolicies with the given subject type by subject ID.
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

func dmaModelIndexFunc(obj any) ([]string, error) {
	var (
		dma          = obj.(*v1.DefaultModelAlias)
		alias, model = dma.Spec.Manifest.Alias, dma.Spec.Manifest.Model
	)
	if !dma.DeletionTimestamp.IsZero() ||
		!system.IsModelID(model) ||
		types.DefaultModelAliasTypeFromString(alias) == types.DefaultModelAliasTypeUnknown ||
		dma.Name != alias {
		// Drop deleted and invalid objects from the index
		return nil, nil
	}

	return []string{
		fmt.Sprintf("%s/%s", alias, model),
	}, nil
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
