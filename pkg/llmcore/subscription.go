package llmcore

import (
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llm"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

const (
	codexUpstream = "https://chatgpt.com/backend-api/codex"
)

type subscriptionTokenKey struct{}

// Kept out of generic routing headers and observer metadata; never persisted.
type subscriptionCredentialValue struct {
	provider  schemas.ModelProvider
	token     string
	accountID string
	lease     *subscriptionLease
}

// A stream transfers its admission lock to background draining before HTTP returns.
type subscriptionLease struct {
	lock     *sync.RWMutex
	draining bool // Set/read by the HTTP goroutine; the drainer only releases the lock.
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
	var auth, agents, accounts []string
	for name, values := range headers {
		switch {
		case strings.EqualFold(name, "authorization"):
			if len(values) != 1 || auth != nil {
				return subscriptionCredentialValue{}, false
			}
			auth = values
		case strings.EqualFold(name, "chatgpt-account-id"):
			if accounts != nil || len(values) != 1 || values[0] == "" || strings.ContainsAny(values[0], " ,\t\r\n") {
				return subscriptionCredentialValue{}, false
			}
			accounts = values
		case isCredentialHeader(name):
			return subscriptionCredentialValue{}, false
		case strings.EqualFold(name, "user-agent"):
			agents = append(agents, values...)
		}
	}
	if auth == nil {
		return subscriptionCredentialValue{}, accounts == nil
	}
	if len(agents) != 1 || len(auth[0]) <= 7 || !strings.EqualFold(auth[0][:7], "bearer ") || strings.ContainsAny(auth[0][7:], " \t\r\n,") {
		return subscriptionCredentialValue{}, false
	}
	codexAgent := schemas.CodexCLI.Matches(strings.ReplaceAll(agents[0], "_", "-"))
	if codexAgent && schemas.ClaudeCLI.Matches(agents[0]) {
		return subscriptionCredentialValue{}, false
	}
	credential := subscriptionCredentialValue{token: auth[0]}
	if schemas.ClaudeCLI.Matches(agents[0]) && strings.HasPrefix(strings.ToLower(auth[0][7:]), "sk-ant-oat") && accounts == nil {
		credential.provider = schemas.Anthropic
		return credential, true
	}
	// Classify the JWT shape only. OpenAI validates it; Obot neither trusts its
	// claims nor derives an account identity from an unverified payload.
	parts := strings.Split(auth[0][7:], ".")
	if !codexAgent || len(parts) != 3 || !strings.HasPrefix(parts[0], "eyJ") {
		return subscriptionCredentialValue{}, false
	}
	for _, part := range parts {
		if part == "" || strings.ContainsFunc(part, func(r rune) bool {
			switch {
			case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
				return false
			default:
				return true
			}
		}) {
			return subscriptionCredentialValue{}, false
		}
	}
	credential.provider = schemas.OpenAI
	if accounts != nil {
		credential.accountID = accounts[0]
	}
	return credential, true
}

// Check the effective standalone runtime settings, including proxy inheritance.
// IsSet also catches configured CAs whose secret reference has not resolved yet.
func subscriptionDestination(cfg *llmcoreclient.Config, provider schemas.ModelProvider) bool {
	pc, exists := cfg.Providers[provider]
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
		// Codex owns its upstream account identity as well as its bearer credential.
		if provider == schemas.OpenAI && (strings.EqualFold(name, "cookie") || strings.EqualFold(name, "openai-organization") || strings.EqualFold(name, "openai-project")) {
			return false
		}
	}
	if proxy := providerProxy(pc, cfg.Settings["proxy"]); proxy != nil && (proxy.SkipTLSVerify || proxy.CACertPEM.IsSet()) {
		return false
	}
	if nc.BaseURL == "" {
		return true
	}
	host := "api.anthropic.com"
	if provider == schemas.OpenAI {
		host = "api.openai.com"
	}
	u, err := url.Parse(nc.BaseURL)
	return err == nil && u.Scheme == "https" && u.User == nil && strings.EqualFold(u.Hostname(), host) && (u.Port() == "" || u.Port() == "443")
}

// prepareSubscription runs before access/budget hooks for correct classification,
// and again at the final transition gate after any plugin request rewrites.
func prepareSubscription(ctx *schemas.BifrostContext, req *schemas.BifrostRequest, a *account) *schemas.LLMPluginShortCircuit {
	credential, _ := ctx.Value(subscriptionTokenKey{}).(subscriptionCredentialValue)
	if credential.token == "" {
		return nil
	}

	// Every attempt starts clean. Keep the original token private so a refused
	// foreign fallback cannot erase the credential for a later same-provider fallback.
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
	admitted := provider == credential.provider && subscriptionDestination(a.config.Load(), provider)
	if credential.provider == schemas.OpenAI {
		admitted = admitted && (req.RequestType == schemas.ResponsesRequest || req.RequestType == schemas.ResponsesStreamRequest ||
			req.RequestType == schemas.PassthroughRequest && req.PassthroughRequest != nil && req.PassthroughRequest.Method == http.MethodGet && req.PassthroughRequest.Path == "/models")
	}
	llm.SetSubscriptionAttempt(ctx, admitted)
	// Core v1.12.0 blocks reserved writes during hooks. This engine-owned hook
	// updates only credential selection and the fixed endpoint, then restores the
	// guard before the next plugin. Direct Client calls have no private marker.
	root := ctx.Root()
	root.UnblockRestrictedWrites()
	root.SetValue(schemas.BifrostContextKeySkipKeySelection, admitted && provider == schemas.Anthropic)
	root.ClearValue(schemas.BifrostContextKeyDirectKey)
	root.ClearValue(schemas.BifrostContextKeyURLPath)
	if admitted && provider == schemas.OpenAI {
		// An empty transient direct key bypasses the configured pool on every retry.
		// Keep auth in ExtraHeaders: a nonempty Value makes OpenAI's adapter rebuild
		// Authorization, losing the caller's original bearer spelling. IDs/names
		// stay empty so key metadata cannot expose a credential.
		root.SetValue(schemas.BifrostContextKeyDirectKey, schemas.Key{})
		// Core's absolute URL override bypasses the ordinary API-key base URL.
		root.SetValue(schemas.BifrostContextKeyURLPath, codexUpstream+"/responses")
		if req.PassthroughRequest != nil {
			req.PassthroughRequest.UpstreamURL = codexUpstream
			req.PassthroughRequest.SafeHeaders = nil
			req.PassthroughRequest.Body = nil
		}
	}
	root.BlockRestrictedWrites()
	if !admitted {
		return &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{
			StatusCode:     new(http.StatusForbidden),
			AllowFallbacks: new(true),
			Error: &schemas.ErrorField{
				Type:    new("permission_denied"),
				Message: "Subscription requests require their standard provider over verified TLS without configured authentication overrides",
			},
		}}
	}

	extra["authorization"] = []string{credential.token}
	if credential.accountID != "" {
		extra["chatgpt-account-id"] = []string{credential.accountID}
	}
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
