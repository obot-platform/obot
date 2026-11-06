package llmengine

import (
	"bytes"
	"encoding/json"
	"net/url"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	// azureDefaultScope is the scope the gateway requests for Azure AI when none is configured.
	azureDefaultScope = "https://cognitiveservices.azure.com/.default"
)

var (
	// azureAIHostSuffixes are the Azure OpenAI and AI Services hosts, in the public and
	// sovereign clouds.
	azureAIHostSuffixes = []string{
		".openai.azure.com", ".cognitiveservices.azure.com", ".services.ai.azure.com",
		".openai.azure.us", ".cognitiveservices.azure.us",
		".openai.azure.cn", ".cognitiveservices.azure.cn",
	}
)

// ValidateAzureKey restricts ambient Azure identity to Azure AI endpoints and scopes.
// Call it on the effective key after merging partial updates and restoring masked secrets.
func ValidateAzureKey(body []byte) error {
	var key map[string]json.RawMessage
	if err := json.Unmarshal(body, &key); err != nil {
		return err
	}
	azureConfig := key[fieldNamed(key, "azure_key_config")]
	var cfg map[string]json.RawMessage
	// Missing config can still use server identity; aliases can supply an endpoint independently.
	if !isNull(azureConfig) {
		if err := json.Unmarshal(azureConfig, &cfg); err != nil {
			return err
		}
	}
	secret := func(fields map[string]json.RawMessage, name string) string {
		v := fields[fieldNamed(fields, name)]
		return SecretValue(v)
	}
	value := key[fieldNamed(key, "value")]
	if SecretValue(value) != "" || (secret(cfg, "client_id") != "" && secret(cfg, "client_secret") != "" && secret(cfg, "tenant_id") != "") {
		return nil
	}
	if scopes, ok := cfg[fieldNamed(cfg, "scopes")]; ok {
		var list []string
		_ = json.Unmarshal(scopes, &list)
		for _, s := range list {
			if s = strings.TrimSpace(s); s != "" && s != azureDefaultScope {
				return types.NewErrBadRequest("an Azure key without credentials uses the Obot server's own identity, so it can't request other scopes")
			}
		}
	}
	// The key's endpoint, and any endpoint a model alias of the key overrides it with.
	var endpoints []string
	// Declare before assigning so the closure can call itself recursively.
	var collect func(raw json.RawMessage)
	collect = func(raw json.RawMessage) {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 {
			return
		}
		switch trimmed[0] {
		case '{':
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(trimmed, &fields)
			for k, v := range fields {
				if strings.EqualFold(k, "endpoint") {
					endpoints = append(endpoints, SecretValue(v))
				}
				collect(v)
			}
		case '[':
			var items []json.RawMessage
			_ = json.Unmarshal(trimmed, &items)
			for _, item := range items {
				collect(item)
			}
		}
	}
	for _, v := range key {
		collect(v)
	}
	for _, endpoint := range endpoints {
		if endpoint != "" && !isAzureAIEndpoint(endpoint) {
			return types.NewErrBadRequest("an Azure key without credentials uses the Obot server's own identity, so its endpoint must be an Azure OpenAI or AI Services host (https://<resource>.openai.azure.com, .cognitiveservices.azure.com or .services.ai.azure.com); enter an API key or service principal to use %q", endpoint)
		}
	}
	return nil
}

func isAzureAIEndpoint(endpoint string) bool {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	return slices.ContainsFunc(azureAIHostSuffixes, func(suffix string) bool { return strings.HasSuffix(host, suffix) })
}
