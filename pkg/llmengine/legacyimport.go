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

	"github.com/maximhq/bifrost/core/schemas"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/net/http/httpproxy"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

type legacyImportPlan struct {
	provider string
	build    func(secrets map[string]string) (provider map[string]any, key map[string]any)
}

// ImportLegacyModelProviders imports each provider family once, without changing models or references.
func (e *Engine) ImportLegacyModelProviders(ctx context.Context, c kclient.Client, gatewayClient *gateway.Client, allowPrivateNetwork bool) error {
	families := map[string][]string{}
	for source, plan := range legacyImportPlans {
		families[plan.provider] = append(families[plan.provider], source)
	}
	var failures []error
	for _, family := range slices.Sorted(maps.Keys(families)) {
		sources := families[family]
		slices.Sort(sources)
		err := gatewayClient.MigrateKinmIfNotRun(ctx, "llm_gateway_legacy_provider_"+family, func() error {
			if envConfigured(family) != "" {
				return nil
			}
			existing := []string{family}
			if family == "bedrock" {
				existing = append(existing, "bedrock_mantle")
			}
			for _, name := range existing {
				if _, err := e.store.GetProvider(ctx, name); err == nil {
					// Existing configuration wins: complete this family without merging legacy keys; other families still run.
					return nil
				} else if !apierrors.IsNotFound(err) {
					return fmt.Errorf("read gateway provider %s: %w", name, err)
				}
			}
			secrets := map[string]map[string]string{}
			for _, source := range sources {
				cred, err := gatewayClient.RevealCredential(ctx, []string{source, system.GenericModelProviderCredentialContext}, source)
				if errors.As(err, &gateway.CredentialNotFoundError{}) {
					continue
				}
				if err != nil {
					return fmt.Errorf("read legacy credentials for %s: %w", source, err)
				}
				if len(cred.Secrets) > 0 {
					secrets[source] = cred.Secrets
				}
			}
			if len(secrets) == 0 {
				return nil
			}
			var models v1.ModelList
			// Azure's API-key mode needs explicit deployment names, not a provider offerings list.
			if c != nil && family == "azure" {
				if err := c.List(ctx, &models, kclient.InNamespace(system.DefaultNamespace)); err != nil {
					return err
				}
			}
			candidate, err := e.legacyAggregate(ctx, family, secrets, models.Items, allowPrivateNetwork)
			if err != nil {
				return fmt.Errorf("invalid legacy %s configuration; review credentials, endpoint and deployment settings: %w", family, err)
			}
			// Publish the environment proxy with the keys so every replica sees it before first use.
			applyEgressProxy(family, &candidate.Config, httpproxy.FromEnvironment())
			// Validate every key before the one create; a concurrent administrator's provider wins intact.
			if err := e.store.CreateProvider(ctx, family, candidate); err != nil {
				if apierrors.IsAlreadyExists(err) {
					return nil
				}
				return fmt.Errorf("save imported provider %s: %w", family, err)
			}
			slog.Info("imported legacy provider configuration", "provider", family)
			// shortcut: create and completion are separate writes, use a receipt if deletion during interrupted upgrades must be distinguished.
			return nil
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("import %s: %w", family, err))
		}
	}
	return errors.Join(failures...)
}

func (e *Engine) legacyAggregate(ctx context.Context, family string, sources map[string]map[string]string, models []v1.Model, allowPrivateNetwork bool) (llmcoreclient.Provider, error) {
	cfg := llmcoreclient.Config{
		Providers:     map[schemas.ModelProvider]schemas.ProviderConfig{},
		Keys:          map[schemas.ModelProvider][]schemas.Key{},
		AzureAccounts: map[schemas.ModelProvider]map[string]llmcoreclient.AzureAccount{},
	}
	deployments := map[string][]string{}
	for _, m := range models {
		if m.Spec.Manifest.TargetModel != "" {
			deployments[m.Spec.Manifest.ModelProvider] = append(deployments[m.Spec.Manifest.ModelProvider], m.Spec.Manifest.TargetModel)
		}
	}
	for i, source := range slices.Sorted(maps.Keys(sources)) {
		secrets := sources[source]
		body, key := legacyImportPlans[source].build(secrets)
		if i == 0 {
			body["provider"] = family
			if family == "generic-responses" {
				if secretWithSuffix(secrets, "_MODEL_PROVIDER_BASE_URL") == "" {
					return llmcoreclient.Provider{}, fmt.Errorf("missing base URL")
				}
				body["custom_provider_config"].(map[string]any)["is_key_less"] = secretWithSuffix(secrets, "_MODEL_PROVIDER_API_KEY") == ""
			}
			// Inherit Obot's private-IP allowance before validating provider and key endpoints.
			// The gateway flag covers both private and loopback destinations; metadata stays blocked.
			if allowPrivateNetwork {
				network, _ := body["network_config"].(map[string]any)
				if network == nil {
					network = map[string]any{}
				}
				network["allow_private_network"] = true
				body["network_config"] = network
			}
			raw, err := json.Marshal(body)
			if err != nil {
				return llmcoreclient.Provider{}, err
			}
			guarded, err := e.GuardProviderChange(ctx, http.MethodPost, "providers", raw, false)
			if err != nil {
				return llmcoreclient.Provider{}, err
			}
			if _, err = createProvider(ctx, &cfg, guarded.Body); err != nil {
				return llmcoreclient.Provider{}, err
			}
		}
		key["name"], key["weight"], key["models"] = "imported-"+source, 1, []string{"*"}
		if family == "azure" {
			automatic := false
			if source == system.AzureEntraModelProvider {
				for field, suffix := range map[string]string{"azure_subscription_id": "_MODEL_PROVIDER_SUBSCRIPTION_ID", "azure_resource_group": "_MODEL_PROVIDER_RESOURCE_GROUP", "azure_account_name": "_MODEL_PROVIDER_RESOURCE_NAME"} {
					if value := secretWithSuffix(secrets, suffix); value != "" {
						key[field] = value
						automatic = true
					}
				}
			}
			// Entra account metadata enables automatic deployment discovery. Keep "*" so
			// its results aren't filtered down to the old inventory, hiding future deployments.
			// API-key and manual Entra keys need retained deployment names instead;
			// Profiles still determine which of these models each user may access.
			names := deployments[source]
			slices.Sort(names)
			if !automatic && len(names) > 0 {
				key["models"] = slices.Compact(names)
			}
			if secretWithSuffix(secrets, "_MODEL_PROVIDER_ENDPOINT") == "" {
				return llmcoreclient.Provider{}, fmt.Errorf("missing endpoint")
			}
		}
		if source != system.GenericResponsesModelProvider && source != system.AzureEntraModelProvider && source != system.AmazonBedrockModelProvider && secretWithSuffix(secrets, "_MODEL_PROVIDER_API_KEY") == "" {
			return llmcoreclient.Provider{}, fmt.Errorf("missing key")
		}
		if source == system.AmazonBedrockModelProvider {
			accessKey := secretWithSuffix(secrets, "_MODEL_PROVIDER_ACCESS_KEY_ID")
			secretKey := secretWithSuffix(secrets, "_MODEL_PROVIDER_SECRET_ACCESS_KEY")
			// v0.27 required explicit credentials; migration must not switch to the server's AWS identity.
			if accessKey == "" || secretKey == "" {
				return llmcoreclient.Provider{}, fmt.Errorf("bedrock IAM import requires both access-key ID and secret-access key")
			}
		}
		if source == system.AzureEntraModelProvider {
			for _, suffix := range []string{"_MODEL_PROVIDER_CLIENT_ID", "_MODEL_PROVIDER_CLIENT_SECRET", "_MODEL_PROVIDER_TENANT_ID"} {
				if secretWithSuffix(secrets, suffix) == "" {
					return llmcoreclient.Provider{}, fmt.Errorf("missing Entra credentials")
				}
			}
		}
		plainTextSecrets(key)
		raw, err := json.Marshal(key)
		if err != nil {
			return llmcoreclient.Provider{}, err
		}
		if _, err = e.applyKey(ctx, &cfg, family, "", http.MethodPost, raw); err != nil {
			return llmcoreclient.Provider{}, err
		}
	}
	name := schemas.ModelProvider(family)
	provider := llmcoreclient.Provider{Config: cfg.Providers[name], Keys: cfg.Keys[name], AzureAccounts: cfg.AzureAccounts[name]}
	return provider, nil
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
