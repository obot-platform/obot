package llmengine

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	legacyModelMigrationCompleteAnnotation = "obot.ai/llm-gateway-model-migration-complete"
)

// MigrateLegacyModels prepares new gateway models, publishes through normal discovery,
// then moves stored references. Existing gateway settings always win.
func MigrateLegacyModels(ctx context.Context, c kclient.Client, provider *v1.ModelProvider, desired []kclient.Object, publish func() error) error {
	// After all supported sources complete, normal discovery needs no legacy inventory or alias scans.
	if provider.Annotations[legacyModelMigrationCompleteAnnotation] == "true" {
		return publish()
	}
	pending := false
	var models v1.ModelList
	if err := c.List(ctx, &models, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}
	existing := map[string]bool{}
	byTarget := map[string]*v1.Model{}
	for _, obj := range desired {
		if m, ok := obj.(*v1.Model); ok && m.Spec.Manifest.ModelProvider == system.LLMGatewayModelProvider {
			byTarget[m.Spec.Manifest.TargetModel] = m
		}
	}
	groups := map[string][]*v1.Model{}
	for i := range models.Items {
		source := &models.Items[i]
		if source.Spec.Manifest.ModelProvider == system.LLMGatewayModelProvider {
			existing[source.Name] = true
			continue
		}
		if system.IsLegacyModelProvider(source.Spec.Manifest.ModelProvider) {
			// Retained sources are recovery data; their bindings must not shadow native gateway names.
			if err := releaseLegacyModelAlias(ctx, c, source); err != nil {
				return err
			}
		}
		plan, ok := legacyImportPlans[source.Spec.Manifest.ModelProvider]
		if !ok || !source.DeletionTimestamp.IsZero() || source.Annotations[system.LegacyModelMigrationAnnotation] != "" {
			continue
		}
		target, ok := gatewayModelFor(byTarget, plan.provider, source.Spec.Manifest.TargetModel)
		if !ok {
			// Discovery can omit a model during an outage or before its deployment appears.
			// Keep its references intact and the global completion gate open for a later sync.
			pending = true
			slog.Warn("legacy model has no gateway match; review manually", "model", source.Name, "target", source.Spec.Manifest.TargetModel)
			continue
		}
		groups[target.Spec.Manifest.TargetModel] = append(groups[target.Spec.Manifest.TargetModel], source)
	}
	migrated := map[string]string{}
	var completed []*v1.Model
	for _, id := range slices.Sorted(maps.Keys(groups)) {
		sources, target := groups[id], byTarget[id]
		// An existing gateway model is already administrator-controlled. Legacy sources
		// may disagree with it or each other; migrate their references without changing it.
		if !existing[target.Name] {
			_, native, _ := strings.Cut(id, "/")
			// Several legacy records can collapse onto one destination, such as Azure's
			// authentication variants or an undated alias and its dated model. Prefer an
			// exact native-ID match, then provider/name order, independent of list order.
			slices.SortFunc(sources, func(a, b *v1.Model) int {
				aExact, bExact := a.Spec.Manifest.TargetModel == native, b.Spec.Manifest.TargetModel == native
				if aExact != bExact {
					if aExact {
						return -1
					}
					return 1
				}
				if order := strings.Compare(a.Spec.Manifest.ModelProvider, b.Spec.Manifest.ModelProvider); order != 0 {
					return order
				}
				return strings.Compare(a.Name, b.Name)
			})
			winner := sources[0]
			// Take one source's full settings, including active state and cost override;
			// combining fields could invent a configuration no administrator selected.
			settings := migratedModelSettings(target.Spec.Manifest, winner.Spec.Manifest)
			if slices.ContainsFunc(sources[1:], func(source *v1.Model) bool {
				return !reflect.DeepEqual(settings, migratedModelSettings(target.Spec.Manifest, source.Spec.Manifest))
			}) {
				slog.Warn("legacy model settings differ; using deterministic winner", "target", id, "winner", winner.Name)
			}
			target.Spec.Manifest = settings
		}

		// Settings differences are resolved, not pending. Every matched source moves
		// to this destination and completes after publication and reference updates succeed.
		for _, source := range sources {
			migrated[source.Name] = target.Name
			completed = append(completed, source)
		}
	}
	// Publish the selected settings with the new record so policy checks never see
	// a transient default configuration before those settings are applied.
	if err := publish(); err != nil {
		return err
	}
	if len(completed) > 0 {
		if err := repointModelReferences(ctx, c, migrated); err != nil {
			return err
		}
	}
	for _, source := range completed {
		if source.Annotations == nil {
			source.Annotations = map[string]string{}
		}
		source.Annotations[system.LegacyModelMigrationAnnotation] = migrated[source.Name]
		if err := c.Update(ctx, source); err != nil {
			return err
		}
	}
	if !pending {
		before := provider.DeepCopy()
		updated := provider.DeepCopy()
		if updated.Annotations == nil {
			updated.Annotations = map[string]string{}
		}
		updated.Annotations[legacyModelMigrationCompleteAnnotation] = "true"
		if err := c.Patch(ctx, updated, kclient.MergeFrom(before)); err != nil {
			return err
		}
		// Preserve status edits made earlier in the controller chain while advancing its version.
		provider.ResourceVersion = updated.ResourceVersion
		provider.Annotations = updated.Annotations
	}
	// shortcut: publication and completion are separate writes; deletion during an interrupted
	// migration can replay settings, so recovery must finish before administrators delete targets.
	return nil
}

func migratedModelSettings(target, source types.ModelManifest) types.ModelManifest {
	target.Active = source.Active
	target.Alias = source.Alias
	target.OverrideCost = source.OverrideCost
	// Legacy discovery used the native ID as its default display name; only copy a custom label.
	if source.DisplayName != "" && source.DisplayName != source.TargetModel {
		target.DisplayName = source.DisplayName
	}
	if source.Usage != "" {
		target.Usage = source.Usage
	}
	return target
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

// repointModelReferences changes only explicit record IDs. Native wildcard patterns
// already match qualified targets on main; narrowing them would change their meaning.
func repointModelReferences(ctx context.Context, c kclient.Client, migrated map[string]string) error {
	var aliases v1.DefaultModelAliasList
	if err := c.List(ctx, &aliases, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}
	for i := range aliases.Items {
		a := &aliases.Items[i]
		if target, ok := migrated[a.Spec.Manifest.Model]; ok {
			a.Spec.Manifest.Model = target
			if err := c.Update(ctx, a); err != nil {
				return fmt.Errorf("migrate default model alias %s: %w", a.Name, err)
			}
		}
	}
	var policies v1.ModelAccessPolicyList
	if err := c.List(ctx, &policies, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}
	for i := range policies.Items {
		p := &policies.Items[i]
		changed := false
		var resources []types.ModelResource
		for _, resource := range p.Spec.Manifest.Models {
			if target, ok := migrated[resource.ID]; ok {
				resource.ID = target
				changed = true
			}
			if !slices.Contains(resources, resource) {
				resources = append(resources, resource)
			}
		}
		if changed {
			p.Spec.Manifest.Models = resources
			if err := c.Update(ctx, p); err != nil {
				return fmt.Errorf("migrate model access policy %s: %w", p.Name, err)
			}
		}
	}
	return nil
}

func releaseLegacyModelAlias(ctx context.Context, c kclient.Client, source *v1.Model) error {
	if source.Spec.Manifest.Alias == "" {
		return nil
	}
	var binding v1.Alias
	if err := c.Get(ctx, kclient.ObjectKey{Name: alias.KeyFromScopeID("Model", source.Spec.Manifest.Alias)}, &binding); err != nil {
		return kclient.IgnoreNotFound(err)
	}
	if binding.Spec.TargetKind != "Model" || binding.Spec.TargetName != source.Name || binding.Spec.TargetNamespace != source.Namespace {
		return nil
	}
	// Delete only this source's binding; the gateway alias controller recreates it
	// if the destination uses that alias. An unrelated administrator binding wins.
	return kclient.IgnoreNotFound(c.Delete(ctx, &binding, kclient.Preconditions{UID: &binding.UID, ResourceVersion: &binding.ResourceVersion}))
}
