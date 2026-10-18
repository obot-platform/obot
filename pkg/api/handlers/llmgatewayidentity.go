package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/llmengine"
)

const (
	// azureDefaultScope is the scope the gateway requests for Azure AI when none is configured.
	azureDefaultScope = "https://cognitiveservices.azure.com/.default"

	// The reasons TLS settings are refused for a provider.
	ambientIdentityReason = "it may authenticate with the Obot server's own cloud identity"
	subscriptionReason    = "Claude subscriptions are allowed, and users' subscription tokens may only reach Anthropic over verified TLS"
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

// Some gateway providers fall back to the Obot server's own cloud identity when a key has no
// credentials: Azure to its managed or workload identity, Vertex and Bedrock to their default
// credentials. That identity belongs to the server, not to the administrator configuring the
// gateway, so it may only ever reach the cloud's own hosts over verified TLS.
// rejectAmbientCredentialRedirects refuses configuration that would let it go elsewhere:
//   - certificate checks turned off, or another CA trusted, on an Azure, Vertex or Bedrock
//     provider (including a custom provider built on one), in its network or proxy settings,
//     which would let a server the administrator controls stand in for the cloud's host;
//   - an Azure key without credentials whose endpoint, or a model alias's endpoint, isn't an
//     Azure AI host, or that asks for scopes other than the default, which would send the
//     server's Azure token to that endpoint or for another Azure service.
//
// Vertex builds its host from the region, which rejectUnsafeConfig limits to region names.
// providerType is the base type of the provider the request is for, or "" if unknown.
func rejectAmbientCredentialRedirects(body []byte, providerType string) error {
	ambientProvider := slices.Contains([]string{"azure", "vertex", "bedrock"}, providerType)
	return walkObjects(body, func(fields map[string]json.RawMessage) error {
		if ambientProvider {
			for name, settings := range fields {
				if strings.EqualFold(name, "network_config") || strings.EqualFold(name, "proxy_config") {
					if err := checkProviderTLS(settings, ambientIdentityReason); err != nil {
						return err
					}
				}
			}
		}
		if azure, ok := field(fields, "azure_key_config"); ok {
			if err := checkAzureKey(fields, azure); err != nil {
				return err
			}
		}
		return nil
	})
}

// rejectSubscriptionRedirects refuses, on the Anthropic provider while Claude subscriptions are
// allowed, proxy settings that trust another CA or turn certificate checks off, which would let
// a TLS-inspecting server the administrator controls read users' subscription tokens.
// AnthropicIsDirect stops forwarding the tokens in that case anyway; this tells the
// administrator why rather than quietly falling back to Obot's key.
func rejectSubscriptionRedirects(body []byte) error {
	return walkObjects(body, func(fields map[string]json.RawMessage) error {
		for name, settings := range fields {
			if strings.EqualFold(name, "proxy_config") {
				if err := checkProviderTLS(settings, subscriptionReason); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// rejectGlobalProxySkipVerify refuses turning off certificate checks on the gateway's global
// proxy. The engine applies that setting to every TLS connection its HTTP clients make, so it
// would turn them off for providers that use the server's own cloud identity and for users'
// Claude subscription tokens too, and a provider can be added under it later. Proxies that
// inspect TLS can be trusted with a CA in the proxy settings of providers with their own keys.
func rejectGlobalProxySkipVerify(path string, body []byte) error {
	if path != "proxy-config" {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return nil
	}
	for name, v := range fields {
		var skip bool
		if strings.EqualFold(name, "skip_tls_verify") && json.Unmarshal(v, &skip) == nil && skip {
			return types.NewErrBadRequest("the gateway proxy can't turn off certificate checks: it applies to every provider, including those that use the Obot server's own cloud identity or users' Claude subscriptions; to trust a TLS-inspecting proxy, add its CA to the proxy settings of a provider that uses its own API key")
		}
	}
	return nil
}

// walkObjects calls fn for every object in a JSON body, however deeply nested.
func walkObjects(body []byte, fn func(fields map[string]json.RawMessage) error) error {
	if !json.Valid(body) {
		return nil
	}
	var walk func(raw json.RawMessage) error
	walk = func(raw json.RawMessage) error {
		trimmed := bytes.TrimSpace(raw)
		switch {
		case len(trimmed) > 0 && trimmed[0] == '{':
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(trimmed, &fields); err != nil {
				return nil
			}
			if err := fn(fields); err != nil {
				return err
			}
			for _, child := range fields {
				if err := walk(child); err != nil {
					return err
				}
			}
		case len(trimmed) > 0 && trimmed[0] == '[':
			var items []json.RawMessage
			if err := json.Unmarshal(trimmed, &items); err != nil {
				return nil
			}
			for _, child := range items {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(body)
}

// field returns an object's field by name in any case, as Bifrost matches field names.
func field(fields map[string]json.RawMessage, name string) (json.RawMessage, bool) {
	for k, v := range fields {
		if strings.EqualFold(k, name) {
			return v, true
		}
	}
	return nil, false
}

func objectFields(raw json.RawMessage) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	return fields
}

// checkProviderTLS refuses network or proxy settings that turn certificate checks off or trust
// another CA. Every spelling of a setting is checked, as the engine matches field names in any
// case.
func checkProviderTLS(settings json.RawMessage, reason string) error {
	for name, v := range objectFields(settings) {
		switch {
		case strings.EqualFold(name, "insecure_skip_verify"), strings.EqualFold(name, "skip_tls_verify"):
			var skip bool
			if json.Unmarshal(v, &skip) == nil && skip {
				return types.NewErrBadRequest("certificate checks can't be turned off for this provider: %s", reason)
			}
		case strings.EqualFold(name, "ca_cert_pem"):
			if llmengine.HasSecretValue(v) {
				return types.NewErrBadRequest("a custom CA can't be trusted for this provider: %s", reason)
			}
		}
	}
	return nil
}

// checkAzureKey checks an Azure key that has no credentials of its own.
func checkAzureKey(key map[string]json.RawMessage, azureConfig json.RawMessage) error {
	cfg := objectFields(azureConfig)
	secret := func(fields map[string]json.RawMessage, name string) string {
		v, _ := field(fields, name)
		return llmengine.SecretValue(v)
	}
	value, _ := field(key, "value")
	if llmengine.SecretValue(value) != "" || (secret(cfg, "client_id") != "" && secret(cfg, "client_secret") != "" && secret(cfg, "tenant_id") != "") {
		return nil
	}
	if scopes, ok := field(cfg, "scopes"); ok {
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
	var collect func(raw json.RawMessage)
	collect = func(raw json.RawMessage) {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 {
			return
		}
		switch trimmed[0] {
		case '{':
			for k, v := range objectFields(trimmed) {
				if strings.EqualFold(k, "endpoint") {
					endpoints = append(endpoints, llmengine.SecretValue(v))
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

// providerType returns the base type of the gateway provider a request to path is for: the
// provider's own name for a built-in provider, or the type a custom provider is built on.
func (h *LLMGatewayHandler) providerType(ctx context.Context, path string, body []byte) string {
	if !strings.HasPrefix(path, "providers") {
		return ""
	}
	var name string
	if names := providerNames(path, body); len(names) > 0 {
		name = names[0]
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	baseType := func(fields map[string]json.RawMessage) string {
		var t string
		if custom, ok := field(fields, "custom_provider_config"); ok {
			if v, ok := field(objectFields(custom), "base_provider_type"); ok {
				_ = json.Unmarshal(v, &t)
			}
		}
		return t
	}
	if t := baseType(fields); t != "" {
		return t
	}
	if name != "" {
		var existing map[string]json.RawMessage
		if err := h.engine.Do(ctx, http.MethodGet, "/api/providers/"+url.PathEscape(name), nil, &existing); err == nil {
			if t := baseType(existing); t != "" {
				return t
			}
		}
	}
	return name
}

// providerNames returns the name of the gateway provider a request to path is for: from the
// path, or, when creating a provider, from every spelling of the body's "provider" field.
func providerNames(path string, body []byte) []string {
	rest, ok := strings.CutPrefix(path, "providers")
	if !ok {
		return nil
	}
	if name, _, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/"); name != "" {
		return []string{name}
	}
	var names []string
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	for k, v := range fields {
		var name string
		if strings.EqualFold(k, "provider") && json.Unmarshal(v, &name) == nil && name != "" {
			names = append(names, name)
		}
	}
	return names
}

// isAnthropicProvider reports whether a request to path is for the Anthropic provider, the
// one Claude subscription tokens are forwarded to.
func isAnthropicProvider(path string, body []byte) bool {
	return slices.ContainsFunc(providerNames(path, body), func(name string) bool {
		return strings.EqualFold(name, "anthropic")
	})
}
