package llmengine

import (
	"strings"

	"github.com/obot-platform/obot/pkg/system"
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
				custom := map[string]any{"base_provider_type": "openai"}
				body := map[string]any{
					"custom_provider_config": custom,
					"network_config":         map[string]any{"base_url": gatewayBaseURL(secretWithSuffix(s, "_MODEL_PROVIDER_BASE_URL")), "default_request_timeout_in_seconds": 300},
				}
				if value := secretWithSuffix(s, "_MODEL_PROVIDER_API_KEY"); value != "" {
					return body, map[string]any{"value": value}
				}
				// Legacy Generic Responses allowed unauthenticated endpoints; use the supported keyless schema.
				custom["is_key_less"] = true
				return body, nil
			},
		},
		system.AmazonBedrockModelProvider: {
			provider: "bedrock_mantle",
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
				return map[string]any{}, map[string]any{"value": "", "bedrock_mantle_key_config": cfg}
			},
		},
		system.AmazonBedrockAPIKeyModelProvider: {
			provider: "bedrock_mantle",
			build: func(s map[string]string) (map[string]any, map[string]any) {
				region := secretWithSuffix(s, "_MODEL_PROVIDER_REGION")
				if region == "" {
					region = "us-east-1"
				}
				return map[string]any{}, map[string]any{
					"value":                     secretWithSuffix(s, "_MODEL_PROVIDER_API_KEY"),
					"bedrock_mantle_key_config": map[string]any{"region": region},
				}
			},
		},
		system.AzureModelProvider: {
			provider: "azure",
			build: func(s map[string]string) (map[string]any, map[string]any) {
				return map[string]any{}, map[string]any{
					"value":            secretWithSuffix(s, "_MODEL_PROVIDER_API_KEY"),
					"azure_key_config": map[string]any{"endpoint": legacyAzureEndpoint(secretWithSuffix(s, "_MODEL_PROVIDER_ENDPOINT"))},
				}
			},
		},
		system.AzureEntraModelProvider: {
			provider: "azure",
			build: func(s map[string]string) (map[string]any, map[string]any) {
				return map[string]any{}, map[string]any{
					"value": "",
					"azure_key_config": map[string]any{
						"endpoint":      legacyAzureEndpoint(secretWithSuffix(s, "_MODEL_PROVIDER_ENDPOINT")),
						"client_id":     secretWithSuffix(s, "_MODEL_PROVIDER_CLIENT_ID"),
						"client_secret": secretWithSuffix(s, "_MODEL_PROVIDER_CLIENT_SECRET"),
						"tenant_id":     secretWithSuffix(s, "_MODEL_PROVIDER_TENANT_ID"),
						"scopes":        []string{"https://ai.azure.com/.default"},
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

func legacyAzureEndpoint(endpoint string) string {
	endpoint = strings.TrimRight(endpoint, "/")
	for _, suffix := range []string{"/openai/v1", "/anthropic/v1"} {
		endpoint = strings.TrimSuffix(endpoint, suffix)
	}
	return endpoint
}
