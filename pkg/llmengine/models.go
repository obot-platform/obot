package llmengine

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/modelcatalog"
	openai "github.com/obot-platform/chat-completion-client"
	"github.com/obot-platform/obot/apiclient/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// Metadata keys set on listed models. The provider back-population controller reads
// "displayName", "usage" and "dialect"; the remaining keys carry Bifrost pricing so the
// caller can derive ModelInfo costs.
const (
	MetadataProvider        = "bifrostProvider"
	MetadataInputCost       = "bifrostInputCostPerToken"
	MetadataOutputCost      = "bifrostOutputCostPerToken"
	MetadataCacheReadCost   = "bifrostCacheReadCostPerToken"
	MetadataCacheWriteCost  = "bifrostCacheWriteCostPerToken"
	MetadataContextLength   = "bifrostContextLength"
	MetadataMaxOutputTokens = "bifrostMaxOutputTokens"

	// anthropicProvider is the built-in provider that forwards Claude subscriptions.
	anthropicProvider = "anthropic"
)

type listedModel struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	NormalizedName string `json:"normalized_name"`
	ContextLength  int    `json:"context_length"`
	MaxOutput      int    `json:"max_output_tokens"`
	OwnedBy        string `json:"owned_by"`
	Architecture   *struct {
		Modality         string   `json:"modality"`
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	Pricing map[string]string `json:"pricing"`
}

// ListedModel is a model the gateway's model listing endpoint doesn't list itself.
type ListedModel struct {
	// ID is the model's provider-qualified ID, such as "anthropic/claude-sonnet-4-5".
	ID   string
	Name string
}

// modelCatalog is the part of the gateway's model catalog that lists a provider's models
// without asking the provider.
type modelCatalog interface {
	GetUnfilteredModelsForProvider(provider schemas.ModelProvider) []string
	GetPricingEntryForModel(model string, provider schemas.ModelProvider) *modelcatalog.PricingEntry
}

// ModelProviderResource is the ModelProvider resource that represents the embedded
// gateway. Unlike registry providers it has no daemon and no credential: providers and
// keys are configured in Bifrost itself.
func ModelProviderResource() *v1.ModelProvider {
	return &v1.ModelProvider{
		Namespace: system.DefaultNamespace,
		Name:      system.LLMGatewayModelProvider,
		Spec: v1.ModelProviderSpec{
			ModelProviderManifest: types.ModelProviderManifest{
				CommonProviderMetadata: types.CommonProviderMetadata{
					Name:        "LLM Gateway",
					Description: "Obot's built-in LLM gateway. Route to 20+ providers with load balancing, fallbacks, budgets, rate limits, caching and analytics.",
					Icon:        "/admin/assets/llm_gateway_icon.svg",
				},
			},
		},
	}
}

// ListModels returns every model Bifrost can currently route to, in the shape the
// provider back-population controller expects.
func (e *Engine) ListModels(ctx context.Context) (*openai.ModelsList, error) {
	var resp struct {
		Data []listedModel `json:"data"`
	}
	if err := e.Do(ctx, http.MethodGet, "/v1/models", nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to list models from LLM gateway: %w", err)
	}

	resp.Data = e.withLastKnownModels(ctx, resp.Data)
	// Added after the last known models are recorded: they aren't a listing of the provider,
	// so they mustn't stand in for one once subscriptions are turned off or keys are added.
	resp.Data = append(resp.Data, e.claudeSubscriptionModels(ctx)...)

	result := &openai.ModelsList{Models: make([]openai.Model, 0, len(resp.Data))}
	seen := make(map[string]struct{}, len(resp.Data))
	for _, m := range resp.Data {
		if _, ok := seen[m.ID]; ok {
			continue
		}
		seen[m.ID] = struct{}{}

		provider, modelID, ok := strings.Cut(m.ID, "/")
		if !ok {
			continue
		}

		displayName := m.NormalizedName
		if displayName == "" {
			displayName = modelID
		}

		metadata := map[string]string{
			"displayName":    fmt.Sprintf("%s (%s)", displayName, provider),
			"usage":          string(modelUsage(modelID, m)),
			"dialect":        string(DialectForModel(provider, modelID)),
			MetadataProvider: provider,
		}
		if m.ContextLength > 0 {
			metadata[MetadataContextLength] = strconv.Itoa(m.ContextLength)
		}
		if m.MaxOutput > 0 {
			metadata[MetadataMaxOutputTokens] = strconv.Itoa(m.MaxOutput)
		}
		for key, meta := range map[string]string{
			"prompt":            MetadataInputCost,
			"completion":        MetadataOutputCost,
			"input_cache_read":  MetadataCacheReadCost,
			"input_cache_write": MetadataCacheWriteCost,
		} {
			if v := m.Pricing[key]; v != "" {
				metadata[meta] = v
			}
		}

		result.Models = append(result.Models, openai.Model{
			ID:       m.ID,
			Object:   "model",
			OwnedBy:  m.OwnedBy,
			Metadata: metadata,
		})
	}
	return result, nil
}

// DialectForModel picks the API dialect Obot's agents should use for a model. Bifrost
// translates every dialect to every provider, so this only selects the most faithful one:
// Anthropic models speak Messages natively, everything else speaks Responses.
func DialectForModel(provider, modelID string) llmtypes.Dialect {
	if provider == "anthropic" || strings.Contains(strings.ToLower(modelID), "claude") {
		return llmtypes.DialectAnthropicMessages
	}
	return llmtypes.DialectOpenAIResponses
}

func modelUsage(modelID string, m listedModel) types.ModelUsage {
	id := strings.ToLower(modelID)
	if m.Architecture != nil {
		for _, out := range m.Architecture.OutputModalities {
			switch out {
			case "embeddings", "embedding":
				return types.ModelUsageEmbedding
			case "image":
				return types.ModelUsageImage
			}
		}
	}
	switch {
	case strings.Contains(id, "embed"):
		return types.ModelUsageEmbedding
	case strings.Contains(id, "dall-e"), strings.Contains(id, "gpt-image"), strings.Contains(id, "imagen"):
		return types.ModelUsageImage
	case strings.Contains(id, "tts"), strings.Contains(id, "whisper"), strings.Contains(id, "transcribe"),
		strings.Contains(id, "moderation"), strings.Contains(id, "rerank"):
		return types.ModelUsageOther
	}
	return types.ModelUsageLLM
}

// CostFromMetadata converts Bifrost per-token pricing into Obot's per-million-token cost.
func CostFromMetadata(metadata map[string]string) (types.ModelCost, bool) {
	perMillion := func(key string) float64 {
		v, err := strconv.ParseFloat(metadata[key], 64)
		if err != nil {
			return 0
		}
		// Round off float error from the conversion: $0.1 per million, not 0.09999999999999999.
		return math.Round(v*1e15) / 1e9
	}
	var cost types.ModelCost
	cost.Input = perMillion(MetadataInputCost)
	cost.Output = perMillion(MetadataOutputCost)
	cost.CacheRead = perMillion(MetadataCacheReadCost)
	cost.CacheWrite = perMillion(MetadataCacheWriteCost)
	return cost, cost.Input > 0 || cost.Output > 0
}

// withLastKnownModels fills in providers missing from a listing. Bifrost returns whatever it
// could list when one provider's list call fails, and syncing that partial list would delete
// the missing provider's models from Obot until the next successful sync. A configured
// provider that lists nothing keeps its previous models; a provider that was removed doesn't.
func (e *Engine) withLastKnownModels(ctx context.Context, listed []listedModel) []listedModel {
	var providers struct {
		Providers []struct {
			Name string `json:"name"`
		} `json:"providers"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers", nil, &providers); err != nil {
		return listed
	}

	byProvider := map[string][]listedModel{}
	for _, m := range listed {
		provider, _, _ := strings.Cut(m.ID, "/")
		byProvider[provider] = append(byProvider[provider], m)
	}

	e.modelsLock.Lock()
	defer e.modelsLock.Unlock()
	current := make(map[string][]listedModel, len(providers.Providers))
	for _, p := range providers.Providers {
		if models, ok := byProvider[p.Name]; ok {
			current[p.Name] = models
		} else if previous, ok := e.lastModels[p.Name]; ok {
			current[p.Name] = previous
			listed = append(listed, previous...)
		}
	}
	e.lastModels = current
	return listed
}

// claudeSubscriptionModels lists Anthropic's models from the gateway's model catalog when
// Claude subscriptions are allowed and the Anthropic provider has no enabled keys. The
// gateway only learns a provider's models by listing them with a key, so an Anthropic
// provider used only for subscriptions would otherwise have no models to grant access to.
func (e *Engine) claudeSubscriptionModels(ctx context.Context) []listedModel {
	if only, err := e.claudeSubscriptionsOnly(ctx); err != nil {
		slog.Warn("failed to check whether the Anthropic provider is only used for Claude subscriptions", "error", err)
		return nil
	} else if !only {
		return nil
	}
	catalog := e.catalog()
	if catalog == nil {
		return nil
	}
	return catalogModels(catalog, anthropicProvider)
}

// ClaudeSubscriptionModels returns the Anthropic models listed from the model catalog while
// the Anthropic provider is used only for Claude subscriptions, which Obot's model list has
// too (see claudeSubscriptionModels). The gateway's own /v1/models endpoint lists a
// provider's models with its keys, so it lists none of them.
func (e *Engine) ClaudeSubscriptionModels(ctx context.Context) []ListedModel {
	listed := e.claudeSubscriptionModels(ctx)
	models := make([]ListedModel, 0, len(listed))
	for _, m := range listed {
		_, id, _ := strings.Cut(m.ID, "/")
		models = append(models, ListedModel{ID: m.ID, Name: cmp.Or(m.Name, m.NormalizedName, id)})
	}
	return models
}

// claudeSubscriptionsOnly reports whether the built-in Anthropic provider has no enabled keys
// and Claude subscriptions are allowed, so its only use is forwarding the subscriptions.
func (e *Engine) claudeSubscriptionsOnly(ctx context.Context) (bool, error) {
	if allowed, err := e.claudeSubscriptionsAllowed(ctx); err != nil || !allowed {
		return false, err
	}
	return e.anthropicWithoutKeys(ctx)
}

func (e *Engine) claudeSubscriptionsAllowed(ctx context.Context) (bool, error) {
	if e.opts.ClaudeSubscriptions == nil {
		return false, nil
	}
	return e.opts.ClaudeSubscriptions(ctx)
}

// anthropicWithoutKeys reports whether the built-in Anthropic provider is configured with no
// enabled keys. Only the built-in provider forwards subscriptions, not a custom one.
func (e *Engine) anthropicWithoutKeys(ctx context.Context) (bool, error) {
	builtin, _, enabled, err := e.anthropicKeys(ctx)
	return builtin && enabled == 0, err
}

// anthropicKeys reports whether the built-in Anthropic provider is configured, and how many
// keys it has, and how many of them are enabled.
func (e *Engine) anthropicKeys(ctx context.Context) (builtin bool, keys, enabled int, err error) {
	var providers struct {
		Providers []struct {
			Name                 string    `json:"name"`
			CustomProviderConfig *struct{} `json:"custom_provider_config"`
		} `json:"providers"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers", nil, &providers); err != nil {
		return false, 0, 0, err
	}
	for _, p := range providers.Providers {
		if p.Name == anthropicProvider && p.CustomProviderConfig == nil {
			builtin = true
		}
	}
	if !builtin {
		return false, 0, 0, nil
	}

	var list struct {
		Keys []struct {
			Enabled *bool `json:"enabled"`
		} `json:"keys"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers/"+anthropicProvider+"/keys", nil, &list); err != nil {
		return false, 0, 0, err
	}
	for _, k := range list.Keys {
		if k.Enabled == nil || *k.Enabled {
			enabled++
		}
	}
	return true, len(list.Keys), enabled, nil
}

// catalogModels lists a provider's current chat models from the gateway's model catalog,
// with the same details and pricing the gateway's own model listing reports for them.
func catalogModels(catalog modelCatalog, provider string) []listedModel {
	var models []listedModel
	for _, id := range catalog.GetUnfilteredModelsForProvider(schemas.ModelProvider(provider)) {
		entry := catalog.GetPricingEntryForModel(id, schemas.ModelProvider(provider))
		if entry == nil || entry.IsDeprecated {
			continue
		}
		switch entry.Mode {
		case "chat", "responses", "completion":
		default:
			continue
		}

		// The gateway's listing fills its models in from the catalog the same way, so the
		// pricing reads the same as a listed model's (per token, as decimal strings).
		model := schemas.Model{ID: provider + "/" + id}
		modelcatalog.ApplyModelInfo(&model, entry)
		data, err := json.Marshal(model)
		if err != nil {
			continue
		}
		var listed listedModel
		if err := json.Unmarshal(data, &listed); err != nil {
			continue
		}
		if modelUsage(id, listed) != types.ModelUsageLLM {
			continue
		}
		models = append(models, listed)
	}
	return models
}

// ConfiguredProviders returns the names of the upstream providers configured in the gateway.
func (e *Engine) ConfiguredProviders(ctx context.Context) ([]string, error) {
	var resp struct {
		Providers []struct {
			Name string `json:"name"`
		} `json:"providers"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers", nil, &resp); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(resp.Providers))
	for _, p := range resp.Providers {
		names = append(names, p.Name)
	}
	// An Anthropic provider with no keys at all lists models only while Claude subscriptions
	// are allowed. Once they aren't, it has none, and the models it listed for them go. One
	// whose keys are only disabled keeps its models, like any other provider.
	if slices.Contains(names, anthropicProvider) {
		if allowed, err := e.claudeSubscriptionsAllowed(ctx); err == nil && !allowed {
			if builtin, keys, _, err := e.anthropicKeys(ctx); err == nil && builtin && keys == 0 {
				names = slices.DeleteFunc(names, func(n string) bool { return n == anthropicProvider })
			}
		}
	}
	return names, nil
}

// HasUpstreamProviders reports whether Bifrost has at least one usable provider. A failed
// check reports the last known answer, so a transient error doesn't unconfigure the gateway.
func (e *Engine) HasUpstreamProviders(ctx context.Context) bool {
	if !e.Ready() {
		return false
	}
	has, err := e.hasUpstreamProviders(ctx)
	e.modelsLock.Lock()
	defer e.modelsLock.Unlock()
	if err != nil {
		return e.lastHasProviders
	}
	e.lastHasProviders = has
	return has
}

func (e *Engine) hasUpstreamProviders(ctx context.Context) (bool, error) {
	var resp struct {
		Providers []struct {
			Name                 string `json:"name"`
			CustomProviderConfig *struct {
				IsKeyLess bool `json:"is_key_less"`
			} `json:"custom_provider_config"`
		} `json:"providers"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers", nil, &resp); err != nil {
		return false, err
	}
	for _, p := range resp.Providers {
		if p.CustomProviderConfig != nil && p.CustomProviderConfig.IsKeyLess {
			return true, nil
		}
		var keys struct {
			Keys []json.RawMessage `json:"keys"`
		}
		if err := e.Do(ctx, http.MethodGet, "/api/providers/"+url.PathEscape(p.Name)+"/keys", nil, &keys); err != nil {
			return false, err
		} else if len(keys.Keys) > 0 {
			return true, nil
		}
		// Keyless built-in providers such as Ollama and vLLM are usable without keys.
		if p.Name == "ollama" || p.Name == "vllm" || p.Name == "sgl" {
			return true, nil
		}
	}
	// An Anthropic provider without keys is usable for Claude subscriptions.
	return e.claudeSubscriptionsOnly(ctx)
}
