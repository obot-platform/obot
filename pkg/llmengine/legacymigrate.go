package llmengine

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	legacyModelMigrationCompleteAnnotation = "obot.ai/llm-gateway-model-migration-complete"
)

// MigrateLegacyModels rewrites default aliases and Profile references after normal
// discovery publishes gateway models. It never changes source or destination models.
func MigrateLegacyModels(ctx context.Context, c kclient.Client, provider *v1.ModelProvider) error {
	// The controller already loaded this provider; completed migrations need no storage reads.
	if provider.Annotations[legacyModelMigrationCompleteAnnotation] == "true" {
		return nil
	}
	var aliases v1.DefaultModelAliasList
	// Migration retries follow normal model sync; scans must not leave permanent
	// namespace-wide triggers that rerun discovery on every Profile or model edit.
	if err := c.List(ctx, untriggered.List(&aliases), kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}
	var policies v1.ModelAccessPolicyList
	if err := c.List(ctx, untriggered.List(&policies), kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}
	referenced := map[string]bool{}
	for _, a := range aliases.Items {
		referenced[a.Spec.Manifest.Model] = true
	}
	for _, p := range policies.Items {
		for _, resource := range p.Spec.Manifest.Models {
			referenced[resource.ID] = true
		}
	}
	var models v1.ModelList
	if err := c.List(ctx, untriggered.List(&models), kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}
	byTarget := map[string]*v1.Model{}
	for i := range models.Items {
		m := &models.Items[i]
		if m.Spec.Manifest.ModelProvider == system.LLMGatewayModelProvider && m.DeletionTimestamp.IsZero() {
			byTarget[m.Spec.Manifest.TargetModel] = m
		}
	}
	pending := false
	migrated := map[string]string{}
	for _, source := range models.Items {
		if !referenced[source.Name] || !system.IsLegacyModelProvider(source.Spec.Manifest.ModelProvider) {
			continue
		}
		plan, supported := legacyImportPlans[source.Spec.Manifest.ModelProvider]
		if !supported {
			// Discovery cannot supply a match for a provider with no migration plan.
			// Keep its references for manual correction without blocking supported migrations forever.
			slog.Warn("legacy model provider is unsupported by migration; update references manually", "provider", source.Spec.Manifest.ModelProvider, "model", source.Name)
			continue
		}
		target, found := gatewayModelFor(byTarget, plan.provider, source.Spec.Manifest.TargetModel)
		if !found || !source.DeletionTimestamp.IsZero() {
			// Catalog fallback cannot prove a model is gone. Preserve the reference and
			// retry; unreferenced legacy inventory never holds the completion gate open.
			pending = true
			slog.Warn("referenced legacy model has no gateway match; review manually", "model", source.Name, "target", source.Spec.Manifest.TargetModel)
			continue
		}
		migrated[source.Name] = target.Name
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
	for i := range policies.Items {
		p := &policies.Items[i]
		changed := false
		var resources []types.ModelResource
		for _, resource := range p.Spec.Manifest.Models {
			if target, ok := migrated[resource.ID]; ok {
				resource.ID = target
				changed = true
			}
			// Several legacy IDs can resolve to the same gateway record.
			if !slices.Contains(resources, resource) {
				resources = append(resources, resource)
			}
		}
		if changed {
			// Profiles are the existing policy records: subjects and associations stay intact.
			p.Spec.Manifest.Models = resources
			if err := c.Update(ctx, p); err != nil {
				return fmt.Errorf("migrate model access policy %s: %w", p.Name, err)
			}
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
	// Partial writes retry naturally: already rewritten references no longer name a source.
	return nil
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
