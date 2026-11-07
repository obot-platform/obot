package llmcore

import (
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llm"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

type subscriptionTokenKey struct{}

// Kept out of generic routing headers and observer metadata; never persisted.
type subscriptionCredentialValue struct {
	token string
}

type subscriptionPlugin struct {
	account *account
}

func isCredentialHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "x-api-key", "api-key", "x-goog-api-key":
		return true
	}
	return false
}

// The proxy owns authentication and admin enablement. Core only admits one
// unambiguous upstream credential mode, using the proxy's OAuth prefix rule.
func subscriptionCredential(headers http.Header) (subscriptionCredentialValue, bool) {
	var auth, agents []string
	for name, values := range headers {
		switch {
		case strings.EqualFold(name, "authorization"):
			if len(values) != 1 || auth != nil {
				return subscriptionCredentialValue{}, false
			}
			auth = values
		case isCredentialHeader(name):
			return subscriptionCredentialValue{}, false
		case strings.EqualFold(name, "user-agent"):
			agents = append(agents, values...)
		}
	}
	if auth == nil {
		return subscriptionCredentialValue{}, true
	}
	if len(agents) != 1 || !schemas.ClaudeCLI.Matches(agents[0]) {
		return subscriptionCredentialValue{}, false
	}
	if !strings.HasPrefix(strings.ToLower(auth[0]), "bearer sk-ant-oat") || strings.ContainsAny(auth[0][7:], " \t\r\n,") {
		return subscriptionCredentialValue{}, false
	}
	return subscriptionCredentialValue{token: auth[0]}, true
}

// Check the effective standalone runtime settings, including proxy inheritance.
// IsSet also catches configured CAs whose secret reference has not resolved yet.
func subscriptionDestination(cfg *llmcoreclient.Config) bool {
	pc, exists := cfg.Providers[schemas.Anthropic]
	if !exists || pc.CustomProviderConfig != nil {
		return false
	}
	nc := pc.NetworkConfig
	if nc.InsecureSkipVerify || nc.CACertPEM.IsSet() {
		return false
	}
	for name := range nc.ExtraHeaders {
		if isCredentialHeader(name) || strings.EqualFold(name, "chatgpt-account-id") || strings.EqualFold(name, "x-obot-api-key") {
			return false
		}
	}
	if proxy := ProviderProxy(pc, cfg.Settings["proxy"]); proxy != nil && (proxy.SkipTLSVerify || proxy.CACertPEM.IsSet()) {
		return false
	}
	if nc.BaseURL == "" {
		return true
	}
	u, err := url.Parse(nc.BaseURL)
	return err == nil && u.Scheme == "https" && u.User == nil && strings.EqualFold(u.Hostname(), "api.anthropic.com") && (u.Port() == "" || u.Port() == "443")
}

// prepareSubscription runs before access/budget hooks for correct classification,
// and again at the final transition gate after any plugin request rewrites.
func prepareSubscription(ctx *schemas.BifrostContext, req *schemas.BifrostRequest, a *account) *schemas.LLMPluginShortCircuit {
	credential, _ := ctx.Value(subscriptionTokenKey{}).(subscriptionCredentialValue)
	if credential.token == "" {
		return nil
	}

	// Every attempt starts clean. Keep the original token private so a refused
	// foreign fallback cannot erase the credential for a later Anthropic fallback.
	extra, _ := ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)
	extra = maps.Clone(extra)
	if extra == nil {
		extra = make(map[string][]string)
	}
	for name := range extra {
		if isCredentialHeader(name) || strings.EqualFold(name, "chatgpt-account-id") || strings.EqualFold(name, "x-obot-api-key") {
			delete(extra, name)
		}
	}
	ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, extra)
	provider, _, _ := req.GetRequestFields()
	admitted := provider == schemas.Anthropic && subscriptionDestination(a.config.Load())
	llm.SetSubscriptionAttempt(ctx, admitted)
	// Core v1.12.0 blocks reserved writes during hooks. This engine-owned hook
	// updates only key selection, then restores the guard before the next plugin.
	root := ctx.Root()
	root.UnblockRestrictedWrites()
	root.SetValue(schemas.BifrostContextKeySkipKeySelection, admitted)
	root.BlockRestrictedWrites()
	if !admitted {
		return &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{
			StatusCode:     new(http.StatusForbidden),
			AllowFallbacks: new(true),
			Error: &schemas.ErrorField{
				Type:    new("permission_denied"),
				Message: "Claude subscription requests require the standard Anthropic provider over verified TLS without configured authentication overrides",
			},
		}}
	}

	extra["authorization"] = []string{credential.token}
	return nil
}

func (*subscriptionPlugin) GetName() string { return "obot-subscription" }
func (*subscriptionPlugin) Cleanup() error  { return nil }

func (*subscriptionPlugin) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}

func (p *subscriptionPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, prepareSubscription(ctx, req, p.account), nil
}

func (*subscriptionPlugin) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
}
