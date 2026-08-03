package modelaccesspolicy

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/backend"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/rest"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	mapUserIndex       = "user-id"
	mapGroupIndex      = "group-id"
	mapSelectorIndex   = "selector-id"
	dmaModelIndex      = "model-id"
	modelProviderIndex = "model-provider"
)

type Helper struct {
	mapIndexer, dmaIndexer, modelIndexer gocache.Indexer
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

	// Create indexer for Model, keyed by provider, so we can resolve external
	// clients' provider-native target models (e.g. "claude-sonnet-4-5") to the
	// internal models the user's access is evaluated against.
	modelGVK, err := backend.GroupVersionKindFor(&v1.Model{})
	if err != nil {
		return nil, err
	}

	modelInformer, err := backend.GetInformerForKind(ctx, modelGVK)
	if err != nil {
		return nil, err
	}

	if err := modelInformer.AddIndexers(gocache.Indexers{
		modelProviderIndex: modelProviderIndexFunc,
	}); err != nil {
		return nil, err
	}

	return &Helper{
		mapIndexer:   mapInformer.GetIndexer(),
		dmaIndexer:   dmaInformer.GetIndexer(),
		modelIndexer: modelInformer.GetIndexer(),
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

// GetUserAllowedModels returns the model IDs that a user can access.
// Model access policies only grant models with an allowed usage, including when
// a policy contains a wildcard or suffix selector.
func (h *Helper) GetUserAllowedModels(user kuser.Info) (map[string]bool, bool, error) {
	var (
		allowedModels  = make(map[string]bool)
		aliasModels    = h.getAliasModels()
		eligibleModels = make(map[string]*v1.Model)
	)

	for _, obj := range h.modelIndexer.List() {
		model, ok := obj.(*v1.Model)
		if !ok || !IsAllowedModelUsage(model.Spec.Manifest.Usage) {
			continue
		}
		eligibleModels[model.Name] = model
	}

	addAllowedModel := func(resource types.ModelResource) {
		if resource.IsWildcard() {
			for modelID := range eligibleModels {
				allowedModels[modelID] = true
			}
			return
		}

		modelID := resource.ID
		if alias, isAlias := resource.IsDefaultModelAliasRef(); isAlias {
			if !IsAllowedDefaultModelAlias(alias) {
				return
			}
			// Resolve allowed aliases to their current model. The eligibility
			// check below also protects against a misconfigured alias.
			modelID = aliasModels[alias]
		} else if _, isPattern := resource.IsWildcardSuffix(); isPattern {
			for id, model := range eligibleModels {
				if resource.MatchesTargetModel(model.Spec.Manifest.TargetModel) {
					allowedModels[id] = true
				}
			}
			return
		}

		if system.IsModelID(modelID) && eligibleModels[modelID] != nil {
			allowedModels[modelID] = true
		}
	}

	addResources := func(resources []types.ModelResource) {
		for _, resource := range resources {
			addAllowedModel(resource)
		}
	}

	// Check policies with wildcard subject selector (*)
	wildcardUserPolicies, err := h.getWildcardUserPolicies()
	if err != nil {
		return nil, false, err
	}
	for _, policy := range wildcardUserPolicies {
		addResources(policy.Spec.Manifest.Models)
	}

	// Check policies that the user is directly included in
	userPolicies, err := h.getUserPolicies(user.GetUID())
	if err != nil {
		return nil, false, err
	}

	for _, policy := range userPolicies {
		addResources(policy.Spec.Manifest.Models)
	}

	// Check policies based on group membership
	for groupID := range authGroupSet(user) {
		groupPolicies, err := h.getGroupPolicies(groupID)
		if err != nil {
			return nil, false, err
		}

		for _, policy := range groupPolicies {
			addResources(policy.Spec.Manifest.Models)
		}
	}

	return allowedModels, false, nil
}

// GetUserAllowedTargetModels returns the set of provider-native target model
// ids (v1.Model.Spec.Manifest.TargetModel) for provider that the user is
// allowed to use. When dialect is non-empty, only models using that dialect are
// returned. A target is included iff a configured, active model maps to it and
// the user is allowed that model. This mirrors the access check enforced by the
// LLM passthrough: a target appears here iff a request for it would succeed.
//
// allowAll reports that the user may use every model without a usage
// restriction. Policy wildcards are enumerated by GetUserAllowedModels instead,
// so they only include eligible usages. When dialect is empty and allowAll is
// true, there is nothing to enumerate, so the returned map is nil and callers
// should skip filtering rather than treat the nil map as "allow nothing". A
// dialect filter always returns an enumerated target set and allowAll=false.
func (h *Helper) GetUserAllowedTargetModels(user kuser.Info, provider, dialect string) (allowed map[string]bool, allowAll bool, _ error) {
	allowedModels, allowAll, err := h.GetUserAllowedModels(user)
	if err != nil {
		return nil, false, err
	}

	if allowAll && dialect == "" {
		// The user may use any model, so there's nothing to filter; skip the
		// provider lookup entirely.
		return nil, true, nil
	}

	// Models served by provider. The provider index already drops deleted,
	// inactive, and unconfigured models, so every entry has a usable target.
	objs, err := h.modelIndexer.ByIndex(modelProviderIndex, provider)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get models for provider %q: %w", provider, err)
	}

	allowed = make(map[string]bool, len(objs))
	for _, obj := range objs {
		m, ok := obj.(*v1.Model)
		if !ok || (dialect != "" && m.Spec.Manifest.Dialect != dialect) {
			continue
		}
		if allowAll || allowedModels[m.Name] {
			allowed[m.Spec.Manifest.TargetModel] = true
		}
	}

	return allowed, false, nil
}

// GetAgentAllowedTargetModels enumerates the targets a hosted agent may use,
// for the same passthrough list endpoint GetUserAllowedTargetModels serves.
//
// An agent's authority was fixed when its instance was created, so it is drawn
// from the models configured on it rather than from access policies, which
// describe people and would not match a principal that is not one. This mirrors
// what the inference path already does; enumerating from policy here instead
// would report a set the agent's own requests do not obey -- an agent told it
// may use nothing, while every call it makes succeeds.
//
// modelIDs holds Model resource names, or "*" for every model.
func (h *Helper) GetAgentAllowedTargetModels(modelIDs []string, provider, dialect string) (allowed map[string]bool, allowAll bool, _ error) {
	allowAll = slices.Contains(modelIDs, "*")
	if allowAll && dialect == "" {
		// Nothing to filter, and the nil map must not be read as "allow
		// nothing" -- the same contract GetUserAllowedTargetModels documents.
		return nil, true, nil
	}

	// The provider index already drops deleted, inactive and unconfigured
	// models, so every entry has a usable target.
	objs, err := h.modelIndexer.ByIndex(modelProviderIndex, provider)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get models for provider %q: %w", provider, err)
	}

	configured := make(map[string]bool, len(modelIDs))
	for _, id := range modelIDs {
		configured[id] = true
	}

	allowed = make(map[string]bool, len(objs))
	for _, obj := range objs {
		m, ok := obj.(*v1.Model)
		if !ok || (dialect != "" && m.Spec.Manifest.Dialect != dialect) {
			continue
		}
		if allowAll || configured[m.Name] {
			allowed[m.Spec.Manifest.TargetModel] = true
		}
	}

	return allowed, false, nil
}

// ResolveModelReference resolves a model reference for provider. References may
// be default model aliases, Model resource names, or provider-native target
// model IDs. Alias and resource references take precedence over target IDs.
func (h *Helper) ResolveModelReference(ctx context.Context, client kclient.Client, namespace, provider, reference string) (*v1.Model, error) {
	if len(rest.IsValidPathSegmentName(reference)) != 0 {
		return h.resolveTargetModel(provider, reference)
	}

	resolved, err := alias.GetFromScope(ctx, client, "Model", namespace, reference)
	if apierrors.IsNotFound(err) {
		return h.resolveTargetModel(provider, reference)
	}
	if err != nil {
		return nil, err
	}

	var model *v1.Model
	switch resolved := resolved.(type) {
	case *v1.DefaultModelAlias:
		if resolved.Spec.Manifest.Model == "" {
			return nil, fmt.Errorf("default model alias %q is not configured", reference)
		}
		model = new(v1.Model)
		if err := alias.Get(ctx, client, model, namespace, resolved.Spec.Manifest.Model); err != nil {
			return nil, err
		}
	case *v1.Model:
		model = resolved
	}

	if model == nil {
		return h.resolveTargetModel(provider, reference)
	}
	if !model.Spec.Manifest.Active {
		return nil, fmt.Errorf("model %q is not active", model.Spec.Manifest.Name)
	}

	return model, nil
}

// resolveTargetModel returns the active Model served by provider whose
// TargetModel matches targetModel, preferring the most recently created when
// more than one matches. The lookup is served directly from the
// (provider, targetModel) index, so it doesn't scan all of a provider's models.
// It is used to resolve external clients' provider-native model ids
// (e.g. "claude-sonnet-4-5") to a configured model. Returns a NotFound error if
// no active model matches. The returned Model is owned by the informer cache;
// treat it as read-only.
func (h *Helper) resolveTargetModel(provider, targetModel string) (*v1.Model, error) {
	objs, err := h.modelIndexer.ByIndex(modelProviderIndex, modelProviderTargetKey(provider, targetModel))
	if err != nil {
		return nil, fmt.Errorf("failed to get models for provider %q target %q: %w", provider, targetModel, err)
	}

	var newest *v1.Model
	for _, obj := range objs {
		m, ok := obj.(*v1.Model)
		if !ok {
			continue
		}
		if newest == nil || m.CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = m
		}
	}

	if newest == nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: v1.SchemeGroupVersion.Group, Resource: "model"}, targetModel)
	}

	return newest, nil
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

// modelProviderIndexFunc indexes a Model by its provider. Deleted, inactive, and
// unconfigured models are dropped from the index so consumers only ever see the
// set of models that are actually usable.
func modelProviderIndexFunc(obj any) ([]string, error) {
	model := obj.(*v1.Model)
	provider, target := model.Spec.Manifest.ModelProvider, model.Spec.Manifest.TargetModel
	if !model.DeletionTimestamp.IsZero() || !model.Spec.Manifest.Active || provider == "" || target == "" {
		return nil, nil
	}

	return []string{provider, modelProviderTargetKey(provider, target)}, nil
}

// modelProviderTargetKey builds the provider/targetModel index key. Provider
// names are Kubernetes resource names and never contain "/", so this is an
// unambiguous encoding even when targetModel itself contains "/".
func modelProviderTargetKey(provider, targetModel string) string {
	return fmt.Sprintf("%s/%s", provider, targetModel)
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
