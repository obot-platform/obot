package llmengine

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
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
// in-cluster vLLM or Ollama servers), are left alone. This only persists desired
// settings; startup and model controllers apply them through Sync.
func (e *Engine) ApplyEgressProxy(ctx context.Context) bool {
	if !e.Ready() {
		return false
	}
	cfg := httpproxy.FromEnvironment()
	if cfg.HTTPSProxy == "" && cfg.HTTPProxy == "" {
		return false
	}

	providers, err := e.ListProviders(ctx)
	if err != nil {
		slog.Warn("failed to list LLM gateway providers to apply the egress proxy", "error", err)
		return false
	}

	changed := false
	for _, provider := range providers {
		name := string(provider.Name)
		if name == "" {
			continue
		}
		updated := false
		err := e.mutation(ctx, func(ctx context.Context) error {
			// The listing supplies names only. Recompute from the latest aggregate
			// and edit only its proxy so intervening admin changes are preserved.
			return e.store.MutateProvider(ctx, name, func(current *llmcoreclient.Provider) error {
				updated = applyProviderEgressProxy(name, current, cfg)
				return nil
			})
		})
		if err != nil {
			slog.Warn("failed to update the egress proxy for an LLM gateway provider", "provider", name, "error", err)
			continue
		}
		if updated {
			slog.Info("LLM gateway provider egress proxy updated", "provider", name)
			changed = true
		}
	}
	return changed
}

// applyProviderEgressProxy also runs before a migration aggregate is first stored:
// resource watches on another replica must never observe an unproxied import.
func applyProviderEgressProxy(name string, p *llmcoreclient.Provider, cfg *httpproxy.Config) bool {
	if cfg.HTTPSProxy == "" && cfg.HTTPProxy == "" {
		return false
	}
	if len(p.Keys) > 1 && (name == "azure" || name == "bedrock_mantle") {
		if pc := p.Config.ProxyConfig; pc != nil && pc.Type != schemas.NoProxy {
			return false
		}
		// Shared pools need proxy selection and NO_PROXY evaluated for each destination.
		p.Config.ProxyConfig = &schemas.ProxyConfig{Type: schemas.EnvProxy}
		return true
	}
	u, err := url.Parse(providerEgressTarget(name, p))
	if err != nil {
		return false
	}
	proxyURL, err := cfg.ProxyFunc()(u)
	pc := p.Config.ProxyConfig
	if pc != nil && pc.Type != schemas.NoProxy {
		// Only remove a proxy inherited from the environment when the endpoint is
		// excluded (localhost or NO_PROXY). An administrator's proxy takes precedence.
		if (err != nil || proxyURL == nil) && isEgressProxy(pc, cfg) {
			p.Config.ProxyConfig = &schemas.ProxyConfig{Type: schemas.NoProxy}
			return true
		}
		return false
	}
	if err != nil || proxyURL == nil {
		return false
	}
	p.Config.ProxyConfig = &schemas.ProxyConfig{
		Type: proxyType(proxyURL),
		URL:  &schemas.SecretVar{Val: proxyURL.String(), SecretType: schemas.SecretTypePlainText},
	}
	return true
}

func providerEgressTarget(name string, p *llmcoreclient.Provider) string {
	if target := p.Config.NetworkConfig.BaseURL; target != "" {
		return target
	}
	// Imported cloud providers have one key. Their hosts live in key settings,
	// rather than network_config, and must be used when evaluating NO_PROXY.
	if len(p.Keys) == 1 {
		key := p.Keys[0]
		if name == "azure" && key.AzureKeyConfig != nil {
			return key.AzureKeyConfig.Endpoint.GetValue()
		}
		if name == "bedrock_mantle" && key.BedrockMantleKeyConfig != nil {
			return "https://bedrock-mantle." + key.BedrockMantleKeyConfig.Region.GetValue() + ".api.aws"
		}
	}
	if target := defaultProviderURLs[name]; target != "" {
		return target
	}
	return "https://" + name + ".invalid"
}

// isEgressProxy compares the stored credential value, not a redacted listing,
// so unrelated administrator proxies cannot match by their masked prefix/suffix.
func isEgressProxy(pc *schemas.ProxyConfig, cfg *httpproxy.Config) bool {
	if pc == nil {
		return false
	}
	configured := pc.URL.GetValue()
	return configured != "" && (configured == cfg.HTTPSProxy || configured == cfg.HTTPProxy)
}

func proxyType(u *url.URL) schemas.ProxyType {
	if strings.HasPrefix(u.Scheme, "socks5") {
		return schemas.Socks5Proxy
	}
	return schemas.HTTPProxy
}
