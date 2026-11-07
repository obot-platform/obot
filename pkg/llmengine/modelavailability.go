package llmengine

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

// modelAvailability preserves provider availability metadata before Bifrost
// aggregates model lists. Bedrock's conversion currently drops lifecycle status
// and the inference types that distinguish ordinary models from capacity variants.
type modelAvailability struct {
	mu          sync.RWMutex
	unavailable map[string]bool
	engine      *Engine
}

type modelAvailabilityProviderKey struct{}

type modelListingConfigKey struct{}

func (*modelAvailability) GetName() string { return "obot-model-availability" }

func (*modelAvailability) Cleanup() error { return nil }

func (m *modelAvailability) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if req.ListModelsRequest != nil {
		ctx.SetValue(modelAvailabilityProviderKey{}, req.ListModelsRequest.Provider)
		if m.engine != nil {
			shared, ok := ctx.Value(modelListingConfigKey{}).(*llmcoreclient.Config)
			if !ok {
				cfg, err := m.engine.config(ctx)
				if err != nil {
					return req, modelListingError(err), nil
				}

				shared = &cfg
			}
			cfg := *shared

			provider := req.ListModelsRequest.Provider
			if isAzureProvider(cfg, provider) {
				models, _ := m.engine.azureDeploymentModels(cfg, provider)

				return req, &schemas.LLMPluginShortCircuit{Response: &schemas.BifrostResponse{
					ListModelsResponse: &schemas.BifrostListModelsResponse{Data: models},
				}}, nil
			}
		}
	}
	return req, nil, nil
}

// Bifrost logs plain hook errors and continues to the provider; terminal failures
// must use its short-circuit contract to prevent Azure offerings from leaking through.
func modelListingError(err error) *schemas.LLMPluginShortCircuit {
	return &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{
		IsBifrostError: true, StatusCode: new(502), AllowFallbacks: new(false),
		Error: &schemas.ErrorField{Message: err.Error()},
	}}
}

func (m *modelAvailability) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	if resp == nil || resp.ListModelsResponse == nil || err != nil {
		return resp, err, nil
	}
	provider, _ := ctx.Value(modelAvailabilityProviderKey{}).(schemas.ModelProvider)
	statuses := map[string]bool{}
	raw, marshalErr := json.Marshal(resp.ListModelsResponse.ExtraFields.RawResponse)
	if marshalErr == nil {
		readBedrockAvailability(raw, statuses)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, unavailable := range statuses {
		m.unavailable[string(provider)+"/"+id] = unavailable
	}
	for _, model := range resp.ListModelsResponse.Data {
		_, id, _ := strings.Cut(model.ID, "/")
		if model.Alias != nil {
			id = *model.Alias
		}
		// A fresh listing can restore a model that was previously deprecated.
		m.unavailable[model.ID] = model.IsDeprecated || statuses[id]
	}
	return resp, err, nil
}

func (m *modelAvailability) isUnavailable(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.unavailable[id]
}

// Bifrost's multi-key listing aggregates raw responses in nested arrays.
func readBedrockAvailability(raw json.RawMessage, statuses map[string]bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return
	}
	switch raw[0] {
	case '[':
		var responses []json.RawMessage
		if json.Unmarshal(raw, &responses) == nil {
			for _, response := range responses {
				readBedrockAvailability(response, statuses)
			}
		}
	case '"':
		var response string
		if json.Unmarshal(raw, &response) == nil {
			readBedrockAvailability(json.RawMessage(response), statuses)
		}
	case '{':
		var response struct {
			Models []struct {
				ID             string   `json:"modelId"`
				ARN            string   `json:"modelArn"`
				InferenceTypes []string `json:"inferenceTypesSupported"`
				Lifecycle      struct {
					Status string `json:"status"`
				} `json:"modelLifecycle"`
			} `json:"modelSummaries"`
		}
		if json.Unmarshal(raw, &response) != nil {
			return
		}
		for _, model := range response.Models {
			status := model.Lifecycle.Status
			provisionedOnly := slices.Contains(model.InferenceTypes, "PROVISIONED") && !slices.Contains(model.InferenceTypes, "ON_DEMAND")
			if status != "ACTIVE" && status != "LEGACY" && !provisionedOnly {
				continue
			}
			unavailable := status == "LEGACY" || provisionedOnly
			for _, id := range []string{model.ID, model.ARN} {
				if id == "" {
					continue
				}
				// A model usable by any configured region/key remains discoverable.
				if _, exists := statuses[id]; !exists || !unavailable {
					statuses[id] = unavailable
				}
			}
		}
	}
}

// Unknown metadata is allowed: listing is discovery, not an inference blocklist.
func (e *Engine) modelIsUnavailable(id string) bool {
	if e.availability != nil && e.availability.isUnavailable(id) {
		return true
	}
	provider, model, ok := strings.Cut(id, "/")
	if !ok || e.prices == nil {
		return false
	}
	if info := e.prices.GetModelInfo(schemas.ModelProvider(provider), model); info != nil && info.IsDeprecated {
		return true
	}
	// LiteLLM catalogs store OpenAI's older completion models under a distinct
	// provider, while the public gateway namespace remains openai/.
	if provider == string(schemas.OpenAI) {
		info := e.prices.GetModelInfo("text-completion-openai", model)
		return info != nil && info.IsDeprecated
	}
	return false
}

func (e *Engine) modelMetadataIsUnavailable(id string, alias *string, base string) bool {
	provider, _, qualified := strings.Cut(id, "/")
	if qualified && base != "" {
		// A deployment or alias can reuse a retired name while its discovered base model is active.
		id = provider + "/" + base
	} else if qualified && alias != nil {
		// An alias name can coincide with a retired model; its target determines availability.
		id = provider + "/" + *alias
	}
	return e.modelIsUnavailable(id)
}

func (e *Engine) modelWithKeysIsUnavailable(id string, keys []schemas.Key) bool {
	provider, model, _ := strings.Cut(id, "/")
	hasEligibleKey := false
	for _, key := range keys {
		if key.Enabled != nil && !*key.Enabled {
			continue
		}
		// Match core's key selection on the requested name before resolving its target.
		if !key.Models.IsAllowed(model) || key.BlacklistedModels.IsBlocked(model) {
			continue
		}
		hasEligibleKey = true
		if !e.modelIsUnavailable(provider + "/" + key.Aliases.Resolve(model)) {
			return false
		}
	}
	// Missing credentials alone must not discard saved models during an outage.
	return hasEligibleKey || e.modelIsUnavailable(id)
}

// UnavailableModels identifies models that resource retention must not restore.
// Unknown models remain eligible for retention during upstream listing failures.
func (e *Engine) UnavailableModels(ctx context.Context, ids []string) (map[string]bool, error) {
	cfg, err := e.config(ctx)
	if err != nil {
		return nil, err
	}
	unavailable := make(map[string]bool)

	// Resolve membership and base-model lifecycle once per provider, including cached IDs.
	azureUnavailable := map[schemas.ModelProvider]map[string]bool{}

	for _, id := range ids {
		provider, _, _ := strings.Cut(id, "/")
		name := schemas.ModelProvider(provider)
		if isAzureProvider(cfg, name) {
			if azureUnavailable[name] == nil {
				models, _ := e.azureDeploymentModels(cfg, name)
				azureUnavailable[name] = make(map[string]bool, len(models))
				for _, model := range models {
					azureUnavailable[name][model.ID] = e.modelMetadataIsUnavailable(model.ID, model.Alias, azureBaseModel(model))
				}
			}

			retired, member := azureUnavailable[name][id]
			if !member || retired {
				unavailable[id] = true
			}
			continue
		}

		if e.modelWithKeysIsUnavailable(id, cfg.Keys[name]) {
			unavailable[id] = true
		}
	}
	return unavailable, nil
}

func (*modelAvailability) PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}
