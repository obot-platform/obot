package modelaccesspolicy

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/nah/pkg/backend"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjectgroups"
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
	mapObotGroupIndex  = "obot-group-id"
	mapSelectorIndex   = "selector-id"
	dmaModelIndex      = "model-id"
	modelProviderIndex = "model-provider"
)

type Helper struct {
	mapIndexer, dmaIndexer, modelIndexer gocache.Indexer
}

// modelGrants resolves the models that model access policies grant, from one snapshot of the
// eligible models and default model aliases.
type modelGrants struct {
	aliasModels    map[string]string
	eligibleModels map[string]*v1.Model
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
		mapUserIndex:      mapSubjectIndexFunc(types.SubjectTypeUser),
		mapGroupIndex:     mapSubjectIndexFunc(types.SubjectTypeGroup),
		mapObotGroupIndex: mapSubjectIndexFunc(types.SubjectTypeObotGroup),
		mapSelectorIndex:  mapSubjectIndexFunc(types.SubjectTypeSelector),
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
	policies, err := h.UserPolicies(user)
	if err != nil {
		return nil, false, err
	}

	var (
		grants        = h.newModelGrants()
		allowedModels = make(map[string]bool)
	)
	for _, policy := range policies {
		grants.addPolicyModels(policy, allowedModels)
	}

	return allowedModels, false, nil
}

// PolicyAllowedModels returns the model IDs a single model access policy (gateway profile)
// grants, evaluated as GetUserAllowedModels evaluates each of a user's policies.
func (h *Helper) PolicyAllowedModels(policy *v1.ModelAccessPolicy) map[string]bool {
	allowed := make(map[string]bool)
	h.newModelGrants().addPolicyModels(policy, allowed)
	return allowed
}

// UserProfileIDsForModel returns the IDs of the user's gateway profiles that grant modelID.
// A budget covers only its own profile's models, so these are the profiles whose budgets
// apply to a request for the model.
func (h *Helper) UserProfileIDsForModel(user kuser.Info, modelID string) ([]string, error) {
	policies, err := h.UserPolicies(user)
	if err != nil {
		return nil, err
	}

	var (
		grants = h.newModelGrants()
		ids    []string
	)
	for _, policy := range policies {
		allowed := make(map[string]bool)
		grants.addPolicyModels(policy, allowed)
		if allowed[modelID] {
			ids = append(ids, policy.Name)
		}
	}
	return ids, nil
}

func (h *Helper) newModelGrants() *modelGrants {
	g := &modelGrants{
		aliasModels:    h.getAliasModels(),
		eligibleModels: make(map[string]*v1.Model),
	}
	for _, obj := range h.modelIndexer.List() {
		model, ok := obj.(*v1.Model)
		if !ok || !IsAllowedModelUsage(model.Spec.Manifest.Usage) {
			continue
		}
		g.eligibleModels[model.Name] = model
	}
	return g
}

// addPolicyModels adds the models policy grants to allowed.
func (g *modelGrants) addPolicyModels(policy *v1.ModelAccessPolicy, allowed map[string]bool) {
	for _, resource := range policy.Spec.Manifest.Models {
		g.addModel(resource, allowed)
	}
}

func (g *modelGrants) addModel(resource types.ModelResource, allowed map[string]bool) {
	if resource.IsWildcard() {
		for modelID := range g.eligibleModels {
			allowed[modelID] = true
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
		modelID = g.aliasModels[alias]
	} else if _, isPattern := resource.IsWildcardSuffix(); isPattern {
		for id, model := range g.eligibleModels {
			if resource.MatchesTargetModel(model.Spec.Manifest.TargetModel) {
				allowed[id] = true
			}
		}
		return
	}

	if system.IsModelID(modelID) && g.eligibleModels[modelID] != nil {
		allowed[modelID] = true
	}
}

// UserPolicies returns the model access policies (gateway profiles) that apply to the user:
// those naming everyone (*), the user, or one of their auth provider or Obot groups. Each
// policy is returned once.
func (h *Helper) UserPolicies(user kuser.Info) ([]*v1.ModelAccessPolicy, error) {
	var (
		result []*v1.ModelAccessPolicy
		seen   = map[string]bool{}
	)
	add := func(policies []v1.ModelAccessPolicy, err error) error {
		if err != nil {
			return err
		}
		for i := range policies {
			if !seen[policies[i].Name] {
				seen[policies[i].Name] = true
				result = append(result, &policies[i])
			}
		}
		return nil
	}

	if err := add(h.getWildcardUserPolicies()); err != nil {
		return nil, err
	}
	if err := add(h.getUserPolicies(user.GetUID())); err != nil {
		return nil, err
	}
	groups, obotGroups := subjectgroups.Sets(user)
	for groupID := range groups {
		if err := add(h.getGroupPolicies(groupID)); err != nil {
			return nil, err
		}
	}
	for groupID := range obotGroups {
		if err := add(h.getIndexedPolicies(mapObotGroupIndex, groupID)); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// UserProfileIDs returns the IDs of the gateway profiles (model access policies) that apply
// to the user. Guardrail policies apply through all of them; budgets only through those that
// grant the requested model (see UserProfileIDsForModel).
func (h *Helper) UserProfileIDs(user kuser.Info) ([]string, error) {
	policies, err := h.UserPolicies(user)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(policies))
	for _, policy := range policies {
		ids = append(ids, policy.Name)
	}
	return ids, nil
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

// ResolveTargetModel finds the model of a provider whose target model is exactly target,
// such as the LLM gateway's "anthropic/claude-sonnet-4-5". When several match, the most
// recently created one wins.
func (h *Helper) ResolveTargetModel(provider, target string) (*v1.Model, error) {
	objs, err := h.modelIndexer.ByIndex(modelProviderIndex, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get models for provider %q: %w", provider, err)
	}
	var newest *v1.Model
	for _, obj := range objs {
		if m, ok := obj.(*v1.Model); ok && m.Spec.Manifest.TargetModel == target {
			if newest == nil || m.CreationTimestamp.After(newest.CreationTimestamp.Time) {
				newest = m
			}
		}
	}
	if newest == nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: v1.SchemeGroupVersion.Group, Resource: "model"}, target)
	}
	return newest, nil
}

// ResolveUpstreamModelName resolves a model name a client sent to a provider whose target
// models are qualified with an upstream provider prefix ("openai/gpt-4o"), such as the LLM
// gateway, when the name isn't one of its target models. Clients that only know
// provider-native IDs send a bare name, such as Claude Code sending "claude-sonnet-4-5", and
// Anthropic's API accepts undated aliases, such as "claude-sonnet-4-5" for
// "claude-sonnet-4-5-20250929", which providers list only in their dated form. So name is
// matched, with or without its upstream provider prefix, against each model's upstream ID and
// against the dated versions of it.
//
// When several models match, the first of these wins: one the caller may use (usable, when it
// isn't nil); the provider that made the model ("openai", "anthropic") over one that hosts
// copies of it (Azure, Bedrock, a custom provider); an exact ID over a dated version; the
// latest dated version; the most recently created model.
func (h *Helper) ResolveUpstreamModelName(provider, name string, usable func(*v1.Model) bool) (*v1.Model, error) {
	objs, err := h.modelIndexer.ByIndex(modelProviderIndex, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get models for provider %q: %w", provider, err)
	}

	wantUpstream, wantID, qualified := strings.Cut(name, "/")
	if !qualified {
		wantUpstream, wantID = "", name
	}

	type candidate struct {
		model      *v1.Model
		usable     bool
		exact      bool
		firstParty bool
		date       string
	}
	better := func(a, b candidate) bool {
		switch {
		case a.usable != b.usable:
			return a.usable
		case a.firstParty != b.firstParty:
			return a.firstParty
		case a.exact != b.exact:
			return a.exact
		case a.date != b.date:
			return a.date > b.date
		}
		return a.model.CreationTimestamp.After(b.model.CreationTimestamp.Time)
	}

	var best *candidate
	for _, obj := range objs {
		m, ok := obj.(*v1.Model)
		if !ok {
			continue
		}
		upstream, id, found := strings.Cut(m.Spec.Manifest.TargetModel, "/")
		if !found || (qualified && upstream != wantUpstream) {
			continue
		}
		c := candidate{model: m, exact: id == wantID, firstParty: llm.IsFirstPartyProvider(upstream)}
		if !c.exact {
			if c.date, ok = llm.DatedVersionOf(wantID, id); !ok {
				continue
			}
		}
		c.usable = usable == nil || usable(m)
		if best == nil || better(c, *best) {
			best = &c
		}
	}

	if best == nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: v1.SchemeGroupVersion.Group, Resource: "model"}, name)
	}
	return best.model, nil
}
