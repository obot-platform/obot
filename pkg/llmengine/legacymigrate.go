package llmengine

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MigrateLegacyModels moves what refers to legacy model providers' models over to the
// gateway's models: default model aliases and model access policies that name a legacy
// model are re-pointed to the gateway model with the same upstream model, and edits made to
// the legacy model (deactivating it, an alias, a cost override) carry over. It runs after
// every gateway model sync, so references move as soon as the gateway lists the models.
//
// Once legacy provider credentials have been imported (importDone), legacy models that were
// migrated or that nothing refers to are deleted, and then legacy model providers left
// without models. A legacy model that is still referred to but that the gateway doesn't
// serve is kept, so the reference doesn't break, until the gateway lists its model.
func MigrateLegacyModels(ctx context.Context, c kclient.Client, importDone bool) error {
	var models v1.ModelList
	if err := c.List(ctx, &models, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}
	gatewayByTarget := map[string]*v1.Model{}
	var legacy []*v1.Model
	for i := range models.Items {
		m := &models.Items[i]
		switch m.Spec.Manifest.ModelProvider {
		case system.LLMGatewayModelProvider:
			gatewayByTarget[m.Spec.Manifest.TargetModel] = m
		case system.ModelProxyModelProvider:
			// The MCP tester's hosted model service, not a legacy provider.
		default:
			legacy = append(legacy, m)
		}
	}
	if len(legacy) == 0 {
		return deleteEmptyLegacyProviders(ctx, c, importDone)
	}

	// legacy model name -> gateway model name
	migrated := map[string]string{}
	for _, m := range legacy {
		plan, ok := legacyImportPlans[m.Spec.Manifest.ModelProvider]
		if !ok {
			continue
		}
		target, ok := gatewayModelFor(gatewayByTarget, plan.provider, m.Spec.Manifest.TargetModel)
		if !ok {
			continue
		}
		migrated[m.Name] = target.Name
		if err := carryModelEdits(ctx, c, m, target); err != nil {
			return err
		}
	}

	referenced, err := repointModelReferences(ctx, c, migrated, legacy)
	if err != nil {
		return err
	}

	if !importDone {
		return nil
	}
	for _, m := range legacy {
		if _, ok := migrated[m.Name]; !ok {
			if referenced[m.Name] {
				slog.Warn("keeping a legacy model that is still referred to but that the LLM gateway doesn't serve", "model", m.Name, "modelProvider", m.Spec.Manifest.ModelProvider, "targetModel", m.Spec.Manifest.TargetModel)
				continue
			}
			if hasModelEdits(m) {
				// Its edits carry over once the gateway lists the model; deleting it now would
				// re-enable a model an administrator deactivated, or drop a cost override.
				continue
			}
		}
		if err := c.Delete(ctx, m); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete legacy model %s: %w", m.Name, err)
		}
	}
	return deleteEmptyLegacyProviders(ctx, c, importDone)
}

// gatewayModelFor returns the gateway model that serves a legacy model's upstream model ID
// from provider: the one with the same ID, or else, for an undated alias such as Anthropic's
// "claude-sonnet-4-5", its latest dated version, such as "claude-sonnet-4-5-20250929". The
// legacy providers listed aliases that the gateway's providers list only in their dated form;
// the alias named the latest version, so that is the model it moves to.
func gatewayModelFor(gatewayByTarget map[string]*v1.Model, provider, id string) (*v1.Model, bool) {
	if m, ok := gatewayByTarget[provider+"/"+id]; ok {
		return m, true
	}
	var (
		latest     *v1.Model
		latestDate string
	)
	for target, m := range gatewayByTarget {
		upstream, upstreamID, _ := strings.Cut(target, "/")
		if upstream != provider {
			continue
		}
		if date, ok := llm.DatedVersionOf(id, upstreamID); ok && date > latestDate {
			latest, latestDate = m, date
		}
	}
	return latest, latest != nil
}

// hasModelEdits reports whether administrators changed a model in a way carryModelEdits keeps.
func hasModelEdits(m *v1.Model) bool {
	return !m.Spec.Manifest.Active || m.Spec.Manifest.Alias != "" || m.Spec.Manifest.OverrideCost != nil
}

// carryModelEdits copies administrators' edits of a legacy model to its gateway model, once:
// the legacy model is deleted after migration.
func carryModelEdits(ctx context.Context, c kclient.Client, legacy, target *v1.Model) error {
	changed := false
	if !legacy.Spec.Manifest.Active && target.Spec.Manifest.Active {
		target.Spec.Manifest.Active = false
		changed = true
	}
	if legacy.Spec.Manifest.Alias != "" && target.Spec.Manifest.Alias == "" {
		target.Spec.Manifest.Alias = legacy.Spec.Manifest.Alias
		changed = true
	}
	if legacy.Spec.Manifest.OverrideCost != nil && target.Spec.Manifest.OverrideCost == nil {
		target.Spec.Manifest.OverrideCost = legacy.Spec.Manifest.OverrideCost
		changed = true
	}
	if !changed {
		return nil
	}
	if err := c.Update(ctx, target); err != nil {
		return fmt.Errorf("failed to carry edits of legacy model %s over to %s: %w", legacy.Name, target.Name, err)
	}
	return nil
}

// repointModelReferences re-points default model aliases and model access policies from
// migrated legacy models to their gateway models. Wildcard suffix patterns written for legacy
// models' provider-native names, such as "claude-haiku-4-5*", get the gateway's provider
// prefix, such as "anthropic/claude-haiku-4-5*", for each provider whose legacy models they
// matched. It returns the models still referred to.
func repointModelReferences(ctx context.Context, c kclient.Client, migrated map[string]string, legacy []*v1.Model) (map[string]bool, error) {
	referenced := map[string]bool{}

	var aliases v1.DefaultModelAliasList
	if err := c.List(ctx, &aliases, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return nil, err
	}
	for i := range aliases.Items {
		a := &aliases.Items[i]
		target, ok := migrated[a.Spec.Manifest.Model]
		if !ok {
			referenced[a.Spec.Manifest.Model] = true
			continue
		}
		slog.Info("re-pointing a default model alias to the LLM gateway", "alias", a.Spec.Manifest.Alias, "from", a.Spec.Manifest.Model, "to", target)
		a.Spec.Manifest.Model = target
		if err := c.Update(ctx, a); err != nil {
			return nil, fmt.Errorf("failed to re-point default model alias %s: %w", a.Name, err)
		}
	}

	var policies v1.ModelAccessPolicyList
	if err := c.List(ctx, &policies, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return nil, err
	}
	for i := range policies.Items {
		p := &policies.Items[i]
		var (
			models  []types.ModelResource
			changed bool
		)
		add := func(m types.ModelResource) {
			if !slices.ContainsFunc(models, func(existing types.ModelResource) bool { return existing.ID == m.ID }) {
				models = append(models, m)
			}
		}
		for _, m := range p.Spec.Manifest.Models {
			if target, ok := migrated[m.ID]; ok {
				m.ID, changed = target, true
				add(m)
				continue
			}
			if qualified := qualifyLegacyPattern(m, legacy); len(qualified) > 0 {
				for _, q := range qualified {
					add(q)
				}
				changed = true
				continue
			}
			referenced[m.ID] = true
			add(m)
		}
		if !changed {
			continue
		}
		slog.Info("re-pointing a model access policy's models to the LLM gateway", "policy", p.Name)
		p.Spec.Manifest.Models = models
		if err := c.Update(ctx, p); err != nil {
			return nil, fmt.Errorf("failed to re-point model access policy %s: %w", p.Name, err)
		}
	}
	return referenced, nil
}

// qualifyLegacyPattern returns the gateway forms of a wildcard suffix pattern written for
// legacy models' provider-native names: the pattern with the gateway provider's prefix, for
// each provider whose legacy models it matches. It returns nothing for other model resources,
// and for patterns that match no legacy model.
func qualifyLegacyPattern(m types.ModelResource, legacy []*v1.Model) []types.ModelResource {
	prefix, ok := m.IsWildcardSuffix()
	if !ok || strings.Contains(prefix, "/") {
		return nil
	}
	var qualified []types.ModelResource
	for _, l := range legacy {
		plan, ok := legacyImportPlans[l.Spec.Manifest.ModelProvider]
		if !ok || !m.MatchesTargetModel(l.Spec.Manifest.TargetModel) {
			continue
		}
		q := types.ModelResource{ID: plan.provider + "/" + prefix + "*"}
		if !slices.Contains(qualified, q) {
			qualified = append(qualified, q)
		}
	}
	return qualified
}

// deleteEmptyLegacyProviders deletes legacy model providers that have no models left, once
// their credentials have been imported.
func deleteEmptyLegacyProviders(ctx context.Context, c kclient.Client, importDone bool) error {
	if !importDone {
		return nil
	}
	var providers v1.ModelProviderList
	if err := c.List(ctx, &providers, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}
	for i := range providers.Items {
		mp := &providers.Items[i]
		if mp.Name == system.LLMGatewayModelProvider || mp.Name == system.ModelProxyModelProvider || !mp.DeletionTimestamp.IsZero() {
			continue
		}
		var owned v1.ModelList
		if err := c.List(ctx, &owned, kclient.InNamespace(mp.Namespace), kclient.MatchingFields{"spec.manifest.modelProvider": mp.Name}); err != nil {
			return err
		}
		if len(owned.Items) > 0 {
			continue
		}
		slog.Info("deleting a legacy model provider replaced by the LLM gateway", "modelProvider", mp.Name)
		if err := c.Delete(ctx, mp); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete legacy model provider %s: %w", mp.Name, err)
		}
	}
	return nil
}
