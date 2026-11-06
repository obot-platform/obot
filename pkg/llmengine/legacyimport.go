package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"

	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	legacyImportPlans = map[string]legacyImportPlan{
		system.OpenAIModelProvider: {
			provider: "openai",
			build: func(s map[string]string) (map[string]any, map[string]any) {
				return map[string]any{}, map[string]any{"value": secretWithSuffix(s, "_MODEL_PROVIDER_API_KEY")}
			},
		},
		system.AnthropicModelProvider: {
			provider: "anthropic",
			build: func(s map[string]string) (map[string]any, map[string]any) {
				return map[string]any{}, map[string]any{"value": secretWithSuffix(s, "_MODEL_PROVIDER_API_KEY")}
			},
		},
		system.GenericResponsesModelProvider: {
			provider: "generic-responses",
			build: func(s map[string]string) (map[string]any, map[string]any) {
				return map[string]any{
					"custom_provider_config": map[string]any{"base_provider_type": "openai"},
					"network_config":         map[string]any{"base_url": gatewayBaseURL(secretWithSuffix(s, "_MODEL_PROVIDER_BASE_URL")), "default_request_timeout_in_seconds": 300},
				}, map[string]any{
					"value": secretWithSuffix(s, "_MODEL_PROVIDER_API_KEY"),
				}
			},
		},
		system.AmazonBedrockModelProvider: {
			provider: "bedrock",
			build: func(s map[string]string) (map[string]any, map[string]any) {
				region := secretWithSuffix(s, "_MODEL_PROVIDER_REGION")
				if region == "" {
					region = "us-east-1"
				}
				cfg := map[string]any{
					"access_key": secretWithSuffix(s, "_MODEL_PROVIDER_ACCESS_KEY_ID"),
					"secret_key": secretWithSuffix(s, "_MODEL_PROVIDER_SECRET_ACCESS_KEY"),
					"region":     region,
				}
				if token := secretWithSuffix(s, "_MODEL_PROVIDER_SESSION_TOKEN"); token != "" {
					cfg["session_token"] = token
				}
				return map[string]any{}, map[string]any{"value": "", "bedrock_key_config": cfg}
			},
		},
		system.AmazonBedrockAPIKeyModelProvider: {
			provider: "bedrock",
			build: func(s map[string]string) (map[string]any, map[string]any) {
				region := secretWithSuffix(s, "_MODEL_PROVIDER_REGION")
				if region == "" {
					region = "us-east-1"
				}
				return map[string]any{}, map[string]any{
					"value":              secretWithSuffix(s, "_MODEL_PROVIDER_API_KEY"),
					"bedrock_key_config": map[string]any{"region": region},
				}
			},
		},
		system.AzureModelProvider: {
			provider: "azure",
			build: func(s map[string]string) (map[string]any, map[string]any) {
				return map[string]any{}, map[string]any{
					"value":            secretWithSuffix(s, "_MODEL_PROVIDER_API_KEY"),
					"azure_key_config": map[string]any{"endpoint": secretWithSuffix(s, "_MODEL_PROVIDER_ENDPOINT")},
				}
			},
		},
		system.AzureEntraModelProvider: {
			provider: "azure",
			build: func(s map[string]string) (map[string]any, map[string]any) {
				return map[string]any{}, map[string]any{
					"value": "",
					"azure_key_config": map[string]any{
						"endpoint":      secretWithSuffix(s, "_MODEL_PROVIDER_ENDPOINT"),
						"client_id":     secretWithSuffix(s, "_MODEL_PROVIDER_CLIENT_ID"),
						"client_secret": secretWithSuffix(s, "_MODEL_PROVIDER_CLIENT_SECRET"),
						"tenant_id":     secretWithSuffix(s, "_MODEL_PROVIDER_TENANT_ID"),
					},
				}
			},
		},
	}
)

// LegacyImportResult reports what ImportLegacyProviders did for each legacy provider.
type LegacyImportResult struct {
	ModelProvider   string `json:"modelProvider"`
	GatewayProvider string `json:"gatewayProvider,omitempty"`
	Status          string `json:"status"`
	Message         string `json:"message,omitempty"`
}

type legacyImportPlan struct {
	provider string
	build    func(secrets map[string]string) (provider map[string]any, key map[string]any)
}

// ImportLegacyModelProviders copies the credentials of configured legacy model providers
// into the gateway, so an existing installation can switch over without re-entering keys.
// Providers the gateway already has are left alone, and so are those configured from the
// environment (see importEnvProviders). Two legacy providers that map to the same
// gateway provider (the Bedrock or Azure variants) become two keys of one provider.
func (engine *Engine) ImportLegacyModelProviders(ctx context.Context, _ kclient.Client, gatewayClient *gateway.Client) ([]LegacyImportResult, error) {
	existing, err := engine.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	// Import callers may discover models immediately; a failed apply is retried by controllers.
	defer func() {
		if _, err := engine.Sync(ctx); err != nil {
			slog.Warn("gateway import saved; reconciliation will retry", "error", err)
		}
	}()

	have := map[string]bool{}
	for _, p := range existing {
		have[string(p.Name)] = true
	}
	// Gateway providers this import created, which more legacy providers may add keys to.
	created := map[string]bool{}

	var results []LegacyImportResult
	for _, name := range slices.Sorted(maps.Keys(legacyImportPlans)) {
		plan := legacyImportPlans[name]
		result := LegacyImportResult{ModelProvider: name, GatewayProvider: plan.provider}

		cred, err := gatewayClient.RevealCredential(ctx, []string{name, system.GenericModelProviderCredentialContext}, name)
		if errors.As(err, &gateway.CredentialNotFoundError{}) || (err == nil && len(cred.Secrets) == 0) {
			// Not configured; nothing to report.
			continue
		} else if err != nil {
			return nil, fmt.Errorf("failed to read credential for %s: %w", name, err)
		}

		// Earlier versions replaced the provider's stored credential with the variable's value on
		// every start, so the variable's key is the one to keep, and importing the stored
		// credential too would give the provider a second key for it.
		if envVar := envConfigured(plan.provider); envVar != "" {
			result.Status = "skipped"
			result.Message = "the provider is configured from the " + envVar + " environment variable instead"
			results = append(results, result)
			continue
		}
		if have[plan.provider] && !created[plan.provider] {
			result.Status = "skipped"
			result.Message = "a gateway provider with this name already exists"
			results = append(results, result)
			continue
		}

		providerBody, key := plan.build(cred.Secrets)
		// Send credentials as explicit plain-text values so Bifrost never interprets a stored
		// secret that happens to start with "env." or "vault." as a reference to resolve.
		plainTextSecrets(key)
		var keyBody []byte
		if key != nil {
			key["name"] = "imported-" + name
			if _, ok := key["models"]; !ok {
				key["models"] = []string{"*"}
			}
			key["weight"] = 1
			keyBody, _ = json.Marshal(key)
		}
		if !created[plan.provider] {
			providerBody["provider"] = plan.provider
			ApplyProviderDefaults(providerBody)
			raw, _ := json.Marshal(providerBody)
			// Create the validated aggregate once; rollback could delete a concurrent writer's keys.
			_, err = changeProvider(ctx, engine, "", http.MethodPost, raw, false, func(ctx context.Context, cfg *llmcoreclient.Config, body []byte) (ProviderResponse, error) {
				provider, err := createProvider(ctx, cfg, body)
				if err == nil && keyBody != nil {
					_, err = engine.applyKey(ctx, cfg, plan.provider, "", http.MethodPost, keyBody)
					if err != nil {
						err = fmt.Errorf("adding the provider's key failed: %w", err)
					}
				}
				return provider, err
			})
		} else if keyBody != nil {
			if _, err = engine.CreateKey(ctx, plan.provider, keyBody); err != nil {
				err = fmt.Errorf("adding the provider's key failed: %w", err)
			}
		}
		if err != nil {
			result.Status = "error"
			result.Message = err.Error()
			results = append(results, result)
			continue
		}
		have[plan.provider], created[plan.provider] = true, true
		result.Status = "imported"
		results = append(results, result)
	}
	return results, nil
}

// gatewayBaseURL converts a legacy OpenAI-compatible provider's base URL, which included the
// API version ("https://api.openai.com/v1") because the provider appended only the endpoint
// ("/responses"), into the gateway's form, which leaves the version out because the gateway
// appends the whole path ("/v1/responses").
func gatewayBaseURL(legacy string) string {
	return strings.TrimSuffix(strings.TrimRight(legacy, "/"), "/v1")
}

func secretWithSuffix(secrets map[string]string, suffix string) string {
	for k, v := range secrets {
		if strings.HasSuffix(k, suffix) {
			return v
		}
	}
	return ""
}

// plainTextSecrets wraps the string values of a key payload (its value and provider-specific
// key config) in Bifrost's explicit plain-text form.
func plainTextSecrets(key map[string]any) {
	plain := func(v string) map[string]any {
		return map[string]any{"value": v, "type": "plain_text"}
	}
	for k, v := range key {
		switch val := v.(type) {
		case string:
			if k == "value" {
				key[k] = plain(val)
			}
		case map[string]any:
			if strings.HasSuffix(k, "_key_config") {
				for ck, cv := range val {
					if s, ok := cv.(string); ok {
						val[ck] = plain(s)
					}
				}
			}
		}
	}
}
