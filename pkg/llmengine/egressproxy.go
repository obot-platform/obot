package llmengine

import (
	"context"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

var (
	// defaultProviderURLs are the endpoints used when a provider has no base URL, for deciding
	// whether its traffic goes through the egress proxy. Providers not listed are assumed to be
	// public services.
	defaultProviderURLs = map[string]string{
		"openai":    "https://api.openai.com",
		"anthropic": "https://api.anthropic.com",
		"gemini":    "https://generativelanguage.googleapis.com",
		"mistral":   "https://api.mistral.ai",
		"groq":      "https://api.groq.com",
	}
)

// ApplyEgressProxy routes providers through the HTTP(S) proxy configured in Obot's
// environment. Bifrost's HTTP client does not read HTTPS_PROXY/NO_PROXY itself, so without
// this an Obot deployed behind an egress proxy could not reach any provider. Providers that
// already have a proxy configured, and those whose endpoint NO_PROXY excludes (such as
// in-cluster vLLM or Ollama servers), are left alone.
func (e *Engine) ApplyEgressProxy(ctx context.Context) (changed bool) {
	if !e.Ready() {
		return false
	}
	cfg := httpproxy.FromEnvironment()
	if cfg.HTTPSProxy == "" && cfg.HTTPProxy == "" {
		return false
	}
	proxyFor := cfg.ProxyFunc()

	var resp struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers", nil, &resp); err != nil {
		slog.Warn("failed to list LLM gateway providers to apply the egress proxy", "error", err)
		return false
	}
	for _, p := range resp.Providers {
		name, _ := p["name"].(string)
		if name == "" {
			continue
		}
		network, _ := p["network_config"].(map[string]any)
		target, _ := network["base_url"].(string)
		if target == "" {
			target = defaultProviderURLs[name]
		}
		if target == "" {
			target = "https://" + name + ".invalid"
		}
		u, err := url.Parse(target)
		if err != nil {
			continue
		}
		proxyURL, err := proxyFor(u)
		if hasProxy(p["proxy_config"]) {
			// A provider moved to an endpoint the proxy doesn't apply to (localhost, NO_PROXY)
			// stops using the egress proxy Obot gave it; any other proxy is the admin's.
			if (err != nil || proxyURL == nil) && isEgressProxy(p["proxy_config"], cfg) {
				body := maps.Clone(p)
				delete(body, "keys")
				// An omitted proxy configuration is kept, so turn it off explicitly.
				body["proxy_config"] = map[string]any{"type": "none"}
				if err := e.Do(ctx, http.MethodPut, "/api/providers/"+url.PathEscape(name), body, nil); err != nil {
					slog.Warn("failed to remove the egress proxy from an LLM gateway provider", "provider", name, "error", err)
					continue
				}
				slog.Info("LLM gateway provider no longer uses the egress proxy", "provider", name)
				changed = true
			}
			continue
		}
		if err != nil || proxyURL == nil {
			continue
		}
		// Send the provider's whole configuration back: the update treats some omitted
		// fields as removed (custom_provider_config can't be removed, so custom providers
		// would be rejected). Redacted values are kept as they are. Keys have their own API.
		body := maps.Clone(p)
		delete(body, "keys")
		body["proxy_config"] = map[string]any{
			"type": proxyType(proxyURL),
			"url":  map[string]any{"value": proxyURL.String(), "type": "plain_text"},
		}
		if err := e.Do(ctx, http.MethodPut, "/api/providers/"+url.PathEscape(name), body, nil); err != nil {
			slog.Warn("failed to apply the egress proxy to an LLM gateway provider", "provider", name, "error", err)
			continue
		}
		slog.Info("LLM gateway provider uses the egress proxy", "provider", name, "proxy", proxyURL.Redacted())
		changed = true
	}
	return changed
}

// hasProxy reports whether a provider's proxy configuration routes through a proxy.
func hasProxy(proxyConfig any) bool {
	pc, ok := proxyConfig.(map[string]any)
	return ok && pc["type"] != "none"
}

// isEgressProxy reports whether a provider's proxy configuration is the environment's
// egress proxy, which Obot applies, as opposed to one an administrator configured. The engine
// returns the proxy URL redacted, so a redacted URL matches the environment's proxy redacted
// the same way.
func isEgressProxy(proxyConfig any, cfg *httpproxy.Config) bool {
	pc, _ := proxyConfig.(map[string]any)
	var configured string
	switch v := pc["url"].(type) {
	case string:
		configured = v
	case map[string]any:
		configured, _ = v["value"].(string)
	}
	if configured == "" {
		return false
	}
	for _, env := range []string{cfg.HTTPSProxy, cfg.HTTPProxy} {
		if env != "" && (configured == env || matchesRedacted(configured, env)) {
			return true
		}
	}
	return false
}

// matchesRedacted reports whether redacted is value as the engine redacts secrets: its first
// and last four characters with 24 asterisks between.
func matchesRedacted(redacted, value string) bool {
	return len(value) > 8 && redacted == value[:4]+strings.Repeat("*", 24)+value[len(value)-4:]
}

func proxyType(u *url.URL) string {
	if strings.HasPrefix(u.Scheme, "socks5") {
		return "socks5"
	}
	return "http"
}
