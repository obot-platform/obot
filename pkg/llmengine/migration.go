package llmengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/net/http/httpproxy"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	migrationSetting = "legacy-provider-migration"
)

// MigrationStatus contains only migration outcomes and compatibility mappings, never credentials.
type MigrationStatus struct {
	Sources  map[string]MigrationSource `json:"sources"`
	Mappings map[string]string          `json:"mappings"`
}

type MigrationSource struct {
	Imported bool   `json:"imported,omitempty"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

type legacyProviderScopeKey struct{}

func decodeMigration(settings map[string]json.RawMessage) (MigrationStatus, error) {
	s := MigrationStatus{}
	if raw := settings[migrationSetting]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &s); err != nil {
			return s, err
		}
	}
	if s.Sources == nil {
		s.Sources = map[string]MigrationSource{}
	}
	if s.Mappings == nil {
		s.Mappings = map[string]string{}
	}
	return s, nil
}

func (e *Engine) MigrationStatus(ctx context.Context) (MigrationStatus, error) {
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return MigrationStatus{}, err
	}
	return decodeMigration(settings)
}

func (e *Engine) mutateMigration(ctx context.Context, change func(*MigrationStatus) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return e.store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		s, err := decodeMigration(settings)
		if err != nil {
			return err
		}
		if err = change(&s); err != nil {
			return err
		}
		raw, err := json.Marshal(s)
		if err == nil {
			settings[migrationSetting] = raw
		}
		return err
	})
}

// ImportLegacyModelProviders reconciles independent provider aggregates. No write
// callback performs I/O, and no cross-resource transaction is assumed.
func (e *Engine) ImportLegacyModelProviders(ctx context.Context, c kclient.Client, g *gateway.Client) ([]LegacyImportResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Startup and leader resource queues share this lock through fresh reads and retries.
	// Keep it separate from syncMu: runtime application must not wait on migration I/O.
	e.importMu.Lock()
	defer e.importMu.Unlock()
	// A call queued by a former leader may be canceled before admission.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var results []LegacyImportResult
	var lastErr error
	// Conflicts across processes still reload outcomes and aggregate provenance.
	err := wait.ExponentialBackoffWithContext(ctx, retry.DefaultRetry, func(ctx context.Context) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		results, lastErr = e.importLegacyModelProviders(ctx, c, g)
		if apierrors.IsConflict(lastErr) || apierrors.IsAlreadyExists(lastErr) {
			return false, nil
		}
		return lastErr == nil, lastErr
	})
	if wait.Interrupted(err) && ctx.Err() == nil {
		err = lastErr
	}
	return results, err
}

func (e *Engine) importLegacyModelProviders(ctx context.Context, c kclient.Client, g *gateway.Client) ([]LegacyImportResult, error) {
	secrets := map[string]map[string]string{}
	entries := map[string]MigrationSource{}
	readErrors := map[string]error{}
	deployments := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(legacyImportPlans)) {
		cred, err := g.RevealCredential(ctx, []string{name, system.GenericModelProviderCredentialContext}, name)
		var missing gateway.CredentialNotFoundError
		if errors.As(err, &missing) || (err == nil && len(cred.Secrets) == 0) {
			continue
		}
		entries[name] = MigrationSource{Name: legacySourceName(name), Provider: legacyImportPlans[name].provider, Status: "pending"}
		if err != nil {
			readErrors[name] = err
			continue
		}
		secrets[name] = cred.Secrets
	}
	if c != nil {
		var models v1.ModelList
		if err := c.List(ctx, &models, kclient.InNamespace(system.DefaultNamespace)); err != nil {
			return nil, err
		}
		var providers v1.ModelProviderList
		if err := c.List(ctx, &providers, kclient.InNamespace(system.DefaultNamespace)); err != nil {
			return nil, err
		}
		for _, provider := range providers.Items {
			if provider.Name == system.LLMGatewayModelProvider || provider.Name == system.ModelProxyModelProvider {
				continue
			}
			if _, supported := legacyImportPlans[provider.Name]; !supported && provider.Status.Configured {
				entries[provider.Name] = MigrationSource{Name: provider.Spec.Name, Status: "unsupported"}
			}
		}
		for _, m := range models.Items {
			source := m.Spec.Manifest.ModelProvider
			if legacyImportPlans[source].provider == "azure" && m.Spec.Manifest.TargetModel != "" {
				deployments[source] = append(deployments[source], m.Spec.Manifest.TargetModel)
			}
			if source == system.LLMGatewayModelProvider || source == system.ModelProxyModelProvider {
				continue
			}
			entry, ok := entries[source]
			if !ok {
				if _, supported := legacyImportPlans[source]; supported {
					continue
				}
				entry = MigrationSource{Name: source, Status: "unsupported"}
			}
			entries[source] = entry
		}
	}
	// Gather every source before importing a family. Retain old outcomes after
	// source resources have been cleaned up (credentials intentionally survive).
	if err := e.mutateMigration(ctx, func(s *MigrationStatus) error {
		for name, entry := range entries {
			if old, ok := s.Sources[name]; ok {
				entry.Status, entry.Message, entry.Imported = old.Status, old.Message, old.Imported
			}
			s.Sources[name] = entry
		}
		return nil
	}); err != nil {
		return nil, err
	}
	state, err := e.MigrationStatus(ctx)
	if err != nil {
		return nil, err
	}
	groups := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		if plan, ok := legacyImportPlans[name]; ok {
			groups[plan.provider] = append(groups[plan.provider], name)
		}
	}
	var results []LegacyImportResult
	var importErrors []error
	reconciled := map[string]LegacyImportResult{}
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		if result, ok := reconciled[name]; ok {
			results = append(results, result)
			continue
		}
		if err := ctx.Err(); err != nil {
			return results, err
		}
		plan := legacyImportPlans[name]
		names := groups[plan.provider]
		if entryUnsupported(name) {
			names = []string{name}
		}
		result := LegacyImportResult{ModelProvider: name, GatewayProvider: plan.provider}
		switch {
		case entryUnsupported(name):
			result.Status = "unsupported"
			result.Message = "this legacy provider is not imported automatically; configure a gateway provider manually"
		case envConfigured(plan.provider) != "":
			result.Status = "environment"
			result.Message = "configured from the server environment"
		case slices.ContainsFunc(groups[plan.provider], func(source string) bool { return readErrors[source] != nil }):
			// A family is atomic, but an unreadable family must not block independent imports.
			result.Status = "error"
			result.Message = "legacy credential could not be read; check server logs; reconciliation will retry"
			for _, source := range groups[plan.provider] {
				if err := readErrors[source]; err != nil {
					importErrors = append(importErrors, fmt.Errorf("read legacy credential %s: %w", source, err))
				}
			}
		default:
			completed := slices.ContainsFunc(names, func(source string) bool { return state.Sources[source].Imported })
			var err error
			result.Status, result.Message, err = e.importLegacySources(ctx, names, secrets, deployments, completed)
			if err != nil {
				importErrors = append(importErrors, fmt.Errorf("import legacy provider %s: %w", name, err))
			}
		}
		// Commit the whole family's outcomes together, before any later source can
		// observe completion. Imported stays sticky even when a deleted pool is blocked.
		if err := e.mutateMigration(ctx, func(s *MigrationStatus) error {
			for _, source := range names {
				current := s.Sources[source]
				current.Status, current.Message = result.Status, result.Message
				if result.Status == "imported" {
					current.Imported = true
				}
				s.Sources[source] = current
			}
			return nil
		}); err != nil {
			return results, err
		}
		for _, source := range names {
			reconciled[source] = LegacyImportResult{ModelProvider: source, GatewayProvider: plan.provider, Status: result.Status, Message: result.Message}
		}
		results = append(results, result)
	}
	return results, errors.Join(importErrors...)
}

func legacyAggregate(ctx context.Context, name string, secrets map[string]string) (llmcoreclient.Provider, error) {
	plan := legacyImportPlans[name]
	body, key := plan.build(secrets)
	// Validate required legacy fields rather than accidentally falling back to
	// ambient cloud credentials when a stored credential is incomplete.
	required := []string{"_MODEL_PROVIDER_API_KEY"}
	switch name {
	case system.AmazonBedrockModelProvider:
		required = []string{"_MODEL_PROVIDER_ACCESS_KEY_ID", "_MODEL_PROVIDER_SECRET_ACCESS_KEY"}
	case system.AzureEntraModelProvider:
		required = []string{"_MODEL_PROVIDER_ENDPOINT", "_MODEL_PROVIDER_CLIENT_ID", "_MODEL_PROVIDER_CLIENT_SECRET", "_MODEL_PROVIDER_TENANT_ID"}
	case system.AzureModelProvider:
		required = append(required, "_MODEL_PROVIDER_ENDPOINT")
	case system.GenericResponsesModelProvider:
		required = []string{"_MODEL_PROVIDER_BASE_URL"}
	}
	for _, suffix := range required {
		if secretWithSuffix(secrets, suffix) == "" {
			return llmcoreclient.Provider{}, fmt.Errorf("missing %s", suffix)
		}
	}
	var keyPayloads []map[string]any
	if key != nil {
		plainTextSecrets(key)
		key["id"], key["name"], key["models"], key["weight"] = "imported-"+name, "imported-"+name, []string{"*"}, 1
		keyPayloads = append(keyPayloads, key)
	}
	ApplyProviderDefaults(body)
	var pc schemas.ProviderConfig
	raw, err := json.Marshal(body)
	if err != nil {
		return llmcoreclient.Provider{}, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		return llmcoreclient.Provider{}, err
	}
	// Imports must pass the same destination checks as administrative provider/key writes.
	if err = checkProviderAddresses(ctx, fields, keyPayloads); err != nil {
		return llmcoreclient.Provider{}, err
	}
	if err = json.Unmarshal(raw, &pc); err != nil {
		return llmcoreclient.Provider{}, err
	}
	// Version stripping can turn invalid legacy values such as "/v1" into an empty destination.
	if name == system.GenericResponsesModelProvider && pc.NetworkConfig.BaseURL == "" {
		return llmcoreclient.Provider{}, fmt.Errorf("invalid base URL: empty destination")
	}
	pc.CheckAndSetDefaults()
	if err = validateProvider(schemas.ModelProvider(plan.provider), pc); err != nil {
		return llmcoreclient.Provider{}, err
	}
	var keys []schemas.Key
	if key != nil {
		var k schemas.Key
		raw, err = json.Marshal(key)
		if err != nil {
			return llmcoreclient.Provider{}, err
		}
		if err = json.Unmarshal(raw, &k); err != nil {
			return llmcoreclient.Provider{}, err
		}
		keys = append(keys, k)
	}
	return llmcoreclient.Provider{Config: pc, Keys: keys, Migration: &llmcoreclient.ProviderMigration{Source: name}}, nil
}

// migratedSources verifies aggregates rather than trusting a global completion
// annotation or a separately written outcome. Failed sources stay put.
// A nil map means global migration state could not be read; otherwise verified
// sources remain usable alongside per-source errors.
func (e *Engine) migratedSources(ctx context.Context) (map[string]string, error) {
	s, err := e.MigrationStatus(ctx)
	if err != nil {
		return nil, err
	}
	approved := map[string]string{}
	var sourceErrors []error
	for name, entry := range s.Sources {
		if entry.Status != "imported" && entry.Status != "environment" {
			continue
		}
		p, err := e.store.GetProvider(ctx, entry.Provider)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			sourceErrors = append(sourceErrors, fmt.Errorf("legacy source %s: %w", name, err))
			continue
		}
		// Only the Generic Responses import has an explicit supported keyless form.
		// Other missing-key providers must not authorize model migration or cleanup.
		custom := p.Config.CustomProviderConfig
		keylessGeneric := name == system.GenericResponsesModelProvider && entry.Provider == legacyImportPlans[name].provider &&
			p.Migration != nil && p.Migration.Source == name && custom != nil && custom.IsKeyLess && custom.BaseProviderType == schemas.OpenAI
		if entry.Status == "imported" && p.Migration != nil && (len(p.Keys) > 0 || keylessGeneric) && (p.Migration.Source == name ||
			p.Migration.Source == entry.Provider) {
			approved[name] = entry.Provider
		}
		if entry.Status == "environment" && slices.ContainsFunc(p.Keys, func(k schemas.Key) bool { return k.Name == "env-"+envConfigured(entry.Provider) }) {
			approved[name] = entry.Provider
		}
	}
	return approved, errors.Join(sourceErrors...)
}

func (e *Engine) MigrateLegacyModels(ctx context.Context, c kclient.Client) error {
	core := e.core.Load()
	if !e.Ready() || core == nil {
		return nil
	}
	sources, sourceErr := e.migratedSources(ctx)
	if sources == nil {
		return sourceErr
	}
	// Verify provenance before capturing desired state; configured names and stale
	// discovery alone do not prove this replica can serve an imported provider.
	loaded, err := e.store.Load(ctx)
	if err != nil {
		return errors.Join(sourceErr, err)
	}
	for source, provider := range sources {
		if !core.ProviderApplied(schemas.ModelProvider(provider), loaded.Config) {
			delete(sources, source)
		}
	}
	// Subscription entries and core's failed-poll catalog fallbacks are not
	// evidence that an imported provider serves a model.
	discovered := e.availability.discoveredModels(loaded.Config)
	err = migrateLegacyModels(ctx, c, sources, discovered, func(mapping map[string]string) error {
		if err := e.mutateMigration(ctx, func(s *MigrationStatus) error { maps.Copy(s.Mappings, mapping); return nil }); err != nil {
			return err
		}
		// Apply the committed mappings before cleanup can remove their source models.
		// Reload through serialized Sync so an overlapping reconciliation cannot regress them.
		_, syncErr := e.Sync(ctx)
		// Sync may have read a snapshot before a concurrent write. Reload before
		// taking syncMu so resource I/O does not hold the publication check lock.
		current, err := e.store.Load(ctx)
		if err != nil {
			return errors.Join(syncErr, err)
		}
		e.syncMu.Lock()
		defer e.syncMu.Unlock()
		// Cached entries can survive a failed decode of newer core settings.
		// Validate publication and provider state under the adapter's Sync lock.
		if !bytes.Equal(e.appliedMigration, core.Setting(migrationSetting)) {
			return errors.Join(syncErr, errors.New("legacy model mappings: current snapshot was not published"))
		}
		// Cleanup needs these mappings published, even when an unrelated provider
		// failed Sync. Cached mappings cannot authorize cleanup after a blocking load.
		published := e.migratedMappings.Load()
		for source, target := range mapping {
			if published == nil || (*published)[source] != target {
				return errors.Join(syncErr, fmt.Errorf("legacy model mapping %s to %s was not published", source, target))
			}
		}
		// Recheck evidence before releasing references and source records.
		fresh := e.availability.discoveredModels(current.Config)
		for _, provider := range sources {
			name := schemas.ModelProvider(provider)
			if !core.ProviderApplied(name, loaded.Config) {
				return errors.Join(syncErr, fmt.Errorf("legacy target provider %s is no longer applied", provider))
			}
			if modelDiscoveryConfig(current.Config, name) != modelDiscoveryConfig(loaded.Config, name) {
				return fmt.Errorf("legacy target provider %s configuration changed before cleanup", provider)
			}
			for id := range discovered {
				if strings.HasPrefix(id, provider+"/") {
					if _, ok := fresh[id]; !ok {
						return fmt.Errorf("legacy target model %s is no longer discovered", id)
					}
				}
			}
		}
		return nil
	})
	return errors.Join(sourceErr, err)
}

// LegacyGatewayProvider maps a former route to its current provider independently
// of import provenance, which is only evidence for migration recovery and cleanup.
func LegacyGatewayProvider(source string) string {
	return legacyImportPlans[source].provider
}

// syncMigrationMappings runs under syncMu so older configuration reads cannot
// publish after newer ones. Failed decoding leaves the accepted snapshot intact.
func (e *Engine) syncMigrationMappings() error {
	raw := e.core.Load().Setting(migrationSetting)
	if bytes.Equal(raw, e.appliedMigration) {
		return nil
	}
	s, err := decodeMigration(map[string]json.RawMessage{migrationSetting: raw})
	if err != nil {
		return fmt.Errorf("legacy model mappings: %w", err)
	}
	// Decoding owns this map; published snapshots are never mutated.
	mappings := s.Mappings
	e.migratedMappings.Store(&mappings)
	e.appliedMigration = raw
	return nil
}

func (e *Engine) MigratedReference(_ context.Context, reference string) (string, error) {
	if mappings := e.migratedMappings.Load(); mappings != nil {
		return (*mappings)[strings.TrimSpace(reference)], nil
	}
	return "", nil
}

func WithLegacyProviderScope(ctx context.Context, provider string) context.Context {
	return context.WithValue(ctx, legacyProviderScopeKey{}, provider)
}

func LegacyProviderScope(ctx context.Context) string {
	value, _ := ctx.Value(legacyProviderScopeKey{}).(string)
	return value
}

func entryUnsupported(name string) bool { _, supported := legacyImportPlans[name]; return !supported }

func legacySourceName(name string) string {
	switch name {
	case system.OpenAIModelProvider:
		return "OpenAI"
	case system.AnthropicModelProvider:
		return "Anthropic"
	case system.GenericResponsesModelProvider:
		return "Generic Responses Compatible"
	case system.AzureModelProvider:
		return "Azure"
	case system.AzureEntraModelProvider:
		return "Azure Entra ID"
	case system.AmazonBedrockModelProvider:
		return "Amazon Bedrock (IAM)"
	case system.AmazonBedrockAPIKeyModelProvider:
		return "Amazon Bedrock (API key)"
	default:
		return name
	}
}

// A completed import is never recreated after an administrator removes it.
// A missing outcome, however, can be recovered from the aggregate's provenance.
func (e *Engine) importLegacySources(ctx context.Context, names []string, secrets map[string]map[string]string, deployments map[string][]string, completed bool) (string, string, error) {
	name := names[0]
	plan := legacyImportPlans[name]
	provenance := llmcoreclient.ProviderMigration{Source: name}
	if len(names) > 1 {
		provenance = llmcoreclient.ProviderMigration{Source: plan.provider}
	}
	proxy := httpproxy.FromEnvironment()
	p, err := e.store.GetProvider(ctx, plan.provider)
	if apierrors.IsNotFound(err) {
		if completed {
			return "blocked", "the imported gateway provider was removed; configure it manually to resume", nil
		}
		for i, source := range names {
			var candidate llmcoreclient.Provider
			candidate, err = legacyAggregate(ctx, source, secrets[source])
			if err != nil {
				break
			}
			// Azure lists explicit deployment names; a wildcard cannot discover them.
			if plan.provider == "azure" && len(deployments[source]) > 0 {
				candidate.Keys[0].Models = schemas.WhiteList(slices.Compact(slices.Sorted(slices.Values(deployments[source]))))
			}
			if i == 0 {
				p = candidate
			} else {
				p.Keys = append(p.Keys, candidate.Keys...)
			}
		}
		if err == nil {
			if err := ctx.Err(); err != nil {
				return "error", "provider import canceled", err
			}
			p.Migration = &provenance
			applyProviderEgressProxy(plan.provider, &p, proxy)
			err = e.store.CreateProvider(ctx, plan.provider, p)
		}
		if apierrors.IsAlreadyExists(err) {
			p, err = e.store.GetProvider(ctx, plan.provider)
		}
	}
	if err != nil {
		return "error", "provider import failed; check server logs; reconciliation will retry", err
	}
	if p.Migration == nil || *p.Migration != provenance {
		return "blocked", "a gateway provider already exists; existing configuration was preserved", nil
	}
	// Repair aggregates imported before proxy inheritance was applied atomically.
	// Re-evaluate on the latest version so concurrent administrator edits survive.
	if applyProviderEgressProxy(plan.provider, &p, proxy) {
		if err := e.store.MutateProvider(ctx, plan.provider, func(current *llmcoreclient.Provider) error {
			if current.Migration != nil && *current.Migration == provenance {
				applyProviderEgressProxy(plan.provider, current, proxy)
			}
			return nil
		}); err != nil {
			return "error", "provider import failed; check server logs; reconciliation will retry", err
		}
	}
	return "imported", "", nil
}
