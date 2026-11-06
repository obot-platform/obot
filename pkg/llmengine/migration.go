package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/net/http/httpproxy"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	migrationSetting      = "legacy-provider-migration"
	sharedMigrationChoice = "both"
)

// MigrationStatus contains no credentials. Version guards administrator decisions;
// background outcome updates do not invalidate an otherwise current choice.
type MigrationStatus struct {
	Version  int                        `json:"version"`
	Choices  map[string]string          `json:"choices"`
	Sources  map[string]MigrationSource `json:"sources"`
	Mappings map[string]string          `json:"mappings"`
}
type MigrationSource struct {
	Imported bool     `json:"imported,omitempty"`
	Name     string   `json:"name"`
	Provider string   `json:"provider"`
	Status   string   `json:"status"`
	Message  string   `json:"message,omitempty"`
	Endpoint string   `json:"endpoint,omitempty"`
	Region   string   `json:"region,omitempty"`
	Models   []string `json:"models"`
}

type legacyProviderScopeKey struct{}

func decodeMigration(settings map[string]json.RawMessage) (MigrationStatus, error) {
	s := MigrationStatus{}
	if raw := settings[migrationSetting]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &s); err != nil {
			return s, err
		}
	}
	if s.Choices == nil {
		s.Choices = map[string]string{}
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
	return e.store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
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

// ResolveMigration records a first-writer-wins decision. Import runs separately:
// a crash after accepting the decision is recovered by resource reconciliation.
func (e *Engine) ResolveMigration(ctx context.Context, family, source string, version int) error {
	err := e.mutateMigration(ctx, func(s *MigrationStatus) error {
		entry, ok := s.Sources[source]
		if (family != "azure" && family != "bedrock_mantle") || (source != sharedMigrationChoice && (!ok || entry.Provider != family)) {
			return &APIError{StatusCode: 400, Message: "invalid legacy provider selection"}
		}
		if s.Version != version || s.Choices[family] != "" {
			return &APIError{StatusCode: 409, Message: "the migration decision changed; refresh and try again"}
		}
		count := 0
		pending := true
		for _, candidate := range s.Sources {
			if candidate.Provider == family {
				count++
				pending = pending && candidate.Status == "pending"
			}
		}
		if count < 2 || (source == sharedMigrationChoice && !pending) || (source != sharedMigrationChoice && entry.Status != "pending") {
			return &APIError{StatusCode: 409, Message: "this provider is no longer awaiting a choice"}
		}
		s.Choices[family] = source
		s.Version++
		return nil
	})
	if apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err) {
		return &APIError{StatusCode: 409, Message: "the migration decision changed; refresh and try again"}
	}
	return err
}

// ImportLegacyModelProviders reconciles independent provider aggregates. No write
// callback performs I/O, and no cross-resource transaction is assumed.
func (e *Engine) ImportLegacyModelProviders(ctx context.Context, c kclient.Client, g *gateway.Client) ([]LegacyImportResult, error) {
	var results []LegacyImportResult
	// Each attempt reloads decisions and provider provenance. Partial imports are
	// idempotent, so concurrent replicas can resume without overwriting manual edits.
	err := retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		results, err = e.importLegacyModelProviders(ctx, c, g)
		return err
	})
	return results, err
}

func (e *Engine) importLegacyModelProviders(ctx context.Context, c kclient.Client, g *gateway.Client) ([]LegacyImportResult, error) {
	secrets := map[string]map[string]string{}
	entries := map[string]MigrationSource{}
	for _, name := range slices.Sorted(maps.Keys(legacyImportPlans)) {
		cred, err := g.RevealCredential(ctx, []string{name, system.GenericModelProviderCredentialContext}, name)
		var missing gateway.CredentialNotFoundError
		if errors.As(err, &missing) || (err == nil && len(cred.Secrets) == 0) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read legacy credential %s: %w", name, err)
		}
		secrets[name] = cred.Secrets
		entry := MigrationSource{Name: legacySourceName(name), Provider: legacyImportPlans[name].provider, Status: "pending", Models: []string{}}
		if endpoint := secretWithSuffix(cred.Secrets, "_MODEL_PROVIDER_ENDPOINT"); endpoint != "" {
			// Userinfo, query strings and fragments may contain credentials.
			if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
				entry.Endpoint = u.Scheme + "://" + u.Host
			}
		}
		entry.Region = secretWithSuffix(cred.Secrets, "_MODEL_PROVIDER_REGION")
		if entry.Provider == "bedrock_mantle" && entry.Region == "" {
			entry.Region = "us-east-1"
		}
		entries[name] = entry
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
				entries[provider.Name] = MigrationSource{Name: provider.Spec.Name, Status: "unsupported", Models: []string{}}
			}
		}
		for _, m := range models.Items {
			source := m.Spec.Manifest.ModelProvider
			if source == system.LLMGatewayModelProvider || source == system.ModelProxyModelProvider {
				continue
			}
			entry, ok := entries[source]
			if !ok {
				if _, supported := legacyImportPlans[source]; supported {
					continue
				}
				entry = MigrationSource{Name: source, Status: "unsupported", Models: []string{}}
			}
			entry.Models = append(entry.Models, m.Spec.Manifest.TargetModel)
			slices.Sort(entry.Models)
			entries[source] = entry
		}
	}
	// Discover all conflicts before importing any source. Retain old outcomes after
	// source resources have been cleaned up (credentials intentionally survive).
	if err := e.mutateMigration(ctx, func(s *MigrationStatus) error {
		for name, entry := range entries {
			if old, ok := s.Sources[name]; ok {
				entry.Status, entry.Message, entry.Imported = old.Status, old.Message, old.Imported
				if len(entry.Models) == 0 {
					entry.Models = old.Models
				}
				if entry.Endpoint != old.Endpoint || entry.Region != old.Region {
					s.Version++
				}
			} else {
				s.Version++
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
	var results []LegacyImportResult
	var importErrors []error
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		plan := legacyImportPlans[name]
		result := LegacyImportResult{ModelProvider: name, GatewayProvider: plan.provider}
		count := 0
		for _, other := range entries {
			if other.Provider == plan.provider {
				count++
			}
		}
		choice := state.Choices[plan.provider]
		switch {
		case entryUnsupported(name):
			result.Status = "unsupported"
			result.Message = "this legacy provider is not imported automatically; configure a gateway provider manually"
		case choice != "" && choice != name && choice != sharedMigrationChoice:
			result.Status = "not-selected"
			result.Message = "another legacy configuration was selected; configure this source manually"
		case count > 1 && choice == "":
			result.Status = "pending"
			result.Message = "choose a legacy configuration in Models / Providers"
		case envConfigured(plan.provider) != "":
			result.Status = "environment"
			result.Message = "configured from the server environment"
		default:
			names := []string{name}
			if choice == sharedMigrationChoice {
				names = nil
				for candidate, candidatePlan := range legacyImportPlans {
					if candidatePlan.provider == plan.provider {
						names = append(names, candidate)
					}
				}
				slices.Sort(names)
			}
			completed := slices.ContainsFunc(names, func(source string) bool { return state.Sources[source].Imported })
			var err error
			result.Status, result.Message, err = e.importLegacySources(ctx, names, secrets, completed)
			if err != nil {
				importErrors = append(importErrors, fmt.Errorf("import legacy provider %s: %w", name, err))
			}
		}
		if err := e.mutateMigration(ctx, func(s *MigrationStatus) error {
			current := s.Sources[name]
			current.Status, current.Message = result.Status, result.Message
			if result.Status == "imported" {
				current.Imported = true
			}
			s.Sources[name] = current
			return nil
		}); err != nil {
			return results, err
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
		required = append(required, "_MODEL_PROVIDER_BASE_URL")
	}
	for _, suffix := range required {
		if secretWithSuffix(secrets, suffix) == "" {
			return llmcoreclient.Provider{}, fmt.Errorf("missing %s", suffix)
		}
	}
	plainTextSecrets(key)
	key["id"], key["name"], key["models"], key["weight"] = "imported-"+name, "imported-"+name, []string{"*"}, 1
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
	if err = checkProviderAddresses(ctx, fields, []map[string]any{key}); err != nil {
		return llmcoreclient.Provider{}, err
	}
	if err = json.Unmarshal(raw, &pc); err != nil {
		return llmcoreclient.Provider{}, err
	}
	pc.CheckAndSetDefaults()
	if err = validateProvider(schemas.ModelProvider(plan.provider), pc); err != nil {
		return llmcoreclient.Provider{}, err
	}
	var k schemas.Key
	raw, err = json.Marshal(key)
	if err != nil {
		return llmcoreclient.Provider{}, err
	}
	if err = json.Unmarshal(raw, &k); err != nil {
		return llmcoreclient.Provider{}, err
	}
	return llmcoreclient.Provider{Config: pc, Keys: []schemas.Key{k}, Migration: &llmcoreclient.ProviderMigration{Source: name, Decision: name}}, nil
}

// migratedSources verifies aggregates rather than trusting a global completion
// annotation or a separately written outcome. Failed or unselected sources stay put.
func (e *Engine) migratedSources(ctx context.Context) (map[string]string, error) {
	s, err := e.MigrationStatus(ctx)
	if err != nil {
		return nil, err
	}
	approved := map[string]string{}
	for name, entry := range s.Sources {
		if entry.Status != "imported" && entry.Status != "environment" {
			continue
		}
		p, err := e.store.GetProvider(ctx, entry.Provider)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if entry.Status == "imported" && p.Migration != nil && len(p.Keys) > 0 && (p.Migration.Source == name ||
			(p.Migration.Source == entry.Provider && p.Migration.Decision == sharedMigrationChoice && s.Choices[entry.Provider] == sharedMigrationChoice)) {
			approved[name] = entry.Provider
		}
		if entry.Status == "environment" && slices.ContainsFunc(p.Keys, func(k schemas.Key) bool { return k.Name == "env-"+envConfigured(entry.Provider) }) {
			approved[name] = entry.Provider
		}
	}
	return approved, nil
}

func (e *Engine) MigrateLegacyModels(ctx context.Context, c kclient.Client) error {
	if e.Err() != nil {
		return nil
	} // Wait until desired resources have applied successfully.
	sources, err := e.migratedSources(ctx)
	if err != nil {
		return err
	}
	// Subscription entries and core's failed-poll catalog fallbacks are not
	// evidence that an imported provider serves a model.
	discovered := e.availability.discoveredModels()
	return migrateLegacyModels(ctx, c, sources, discovered, func(mapping map[string]string) error {
		return e.mutateMigration(ctx, func(s *MigrationStatus) error { maps.Copy(s.Mappings, mapping); return nil })
	})
}

// LegacyGatewayProvider binds a former route to its imported provider or explicitly shared pool.
func (e *Engine) LegacyGatewayProvider(ctx context.Context, source string) (string, error) {
	sources, err := e.migratedSources(ctx)
	if err != nil {
		return "", err
	}
	if p := sources[source]; p != "" {
		return p, nil
	}
	// New installations may use the retained first-party URLs without an import.
	if source == system.OpenAIModelProvider || source == system.AnthropicModelProvider {
		s, err := e.MigrationStatus(ctx)
		if err != nil {
			return "", err
		}
		if _, existed := s.Sources[source]; !existed {
			return legacyImportPlans[source].provider, nil
		}
	}
	return "", &APIError{StatusCode: 410, Message: "this legacy configuration needs attention in Models / Providers; select it for migration or configure a gateway provider manually"}
}

func (e *Engine) MigratedReference(ctx context.Context, reference string) (string, error) {
	s, err := e.MigrationStatus(ctx)
	if err != nil {
		return "", err
	}
	return s.Mappings[strings.TrimSpace(reference)], nil
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
func (e *Engine) importLegacySources(ctx context.Context, names []string, secrets map[string]map[string]string, completed bool) (string, string, error) {
	name := names[0]
	plan := legacyImportPlans[name]
	provenance := llmcoreclient.ProviderMigration{Source: name, Decision: name}
	if len(names) > 1 {
		provenance = llmcoreclient.ProviderMigration{Source: plan.provider, Decision: sharedMigrationChoice}
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
			if i == 0 {
				p = candidate
			} else {
				p.Keys = append(p.Keys, candidate.Keys...)
			}
		}
		if err == nil {
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
