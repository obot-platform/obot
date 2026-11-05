package llmcore

import (
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

type callerCredentialsKey struct{}

// credentialPlugin runs after request-rewriting plugins on every attempt. The
// proxy admits credentials for Anthropic only; routing and fallbacks must not
// carry them elsewhere or silently substitute a configured, paid gateway key.
type credentialPlugin struct {
	account *account
}

func (*credentialPlugin) GetName() string { return "obot-caller-credentials" }
func (*credentialPlugin) Cleanup() error  { return nil }
func (*credentialPlugin) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}

func (p *credentialPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	extra, _ := ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)
	credentials, _ := ctx.Value(callerCredentialsKey{}).(map[string][]string)
	// A direct context may be reused with new headers. Neither a prior absence
	// nor an admitted snapshot can exempt the current headers from inspection.
	incoming := make(map[string][]string)
	for name, values := range extra {
		switch strings.ToLower(name) {
		case "authorization", "x-api-key":
			if len(values) > 0 && strings.Join(values, "") != "" {
				incoming[strings.ToLower(name)] = append(incoming[strings.ToLower(name)], values...)
			}
		}
	}
	if len(credentials) == 0 && len(incoming) > 0 {
		credentials = incoming
		// Retain the admitted credential across a refused fallback so a later
		// Anthropic attempt can still use it. Published header maps stay immutable.
		ctx.SetValue(callerCredentialsKey{}, credentials)
	}
	if len(credentials) == 0 {
		return req, nil, nil
	}
	// Credential-free headers are expected after our own fallback refusals.
	// Changed nonempty credentials must be refused, not masked by the originals.
	changed := len(incoming) > 0 && !maps.EqualFunc(credentials, incoming, slices.Equal[[]string])
	headers := make(map[string][]string, len(extra)+len(credentials))
	maps.Copy(headers, extra)
	extra = headers
	for name := range extra {
		if strings.EqualFold(name, "authorization") || strings.EqualFold(name, "x-api-key") {
			delete(extra, name)
		}
	}
	// Remove credentials even on a refused attempt, without changing the saved
	// originals that a later safe Anthropic fallback may use.
	provider, _, _ := req.GetRequestFields()
	lease, _ := ctx.Value(subscriptionLeaseKey{}).(*subscriptionLease)
	// Unsupported or unowned contexts remain permission failures even during a
	// transition. Only an HTTP owner can distinguish temporary lease admission
	// refusal from a direct caller that has no managed credential lifetime.
	owned := lease != nil && lease.account == p.account && lease.owner == ctx.Root() && ctx.Root().Err() == nil && (lease.state.Load() == subscriptionLeaseActive || lease.state.Load() == subscriptionLeasePending)
	if changed || !supportedSubscription(ctx, credentials) || !owned || provider != schemas.Anthropic || !p.account.subscriptionDestinationSafe() {
		ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, extra)
		return req, credentialRefusal(http.StatusForbidden, "permission_denied", "caller credentials require a single Claude CLI Anthropic subscription bearer token and a verified Anthropic destination without configured authentication overrides"), nil
	}
	if _, pending := p.account.transitioning.Load(schemas.Anthropic); pending || lease.state.Load() == subscriptionLeasePending {
		ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, extra)
		return req, credentialRefusal(http.StatusServiceUnavailable, "api_error", "Anthropic is being reconfigured; please retry"), nil
	}

	// Keep the saved admission private even if a caller edits current headers.
	for name, values := range credentials {
		extra[name] = slices.Clone(values)
	}
	ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, extra)

	return req, nil, nil
}

func (*credentialPlugin) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
}

func hasCallerCredentials(headers map[string][]string) bool {
	for name, values := range headers {
		if strings.EqualFold(name, "authorization") || strings.EqualFold(name, "x-api-key") {
			if strings.Join(values, "") != "" {
				return true
			}
		}
	}
	return false
}

func subscriptionBearer(value string) bool {
	// Match the proxy's token prefix, but refuse combined/repeated credentials
	// and whitespace inside a token rather than forwarding ambiguous HTTP auth.
	scheme, token, ok := strings.Cut(value, " ")
	if !ok || !strings.EqualFold(scheme, "bearer") || !strings.HasPrefix(strings.ToLower(token), "sk-ant-oat") {
		return false
	}

	// RFC 6750 bearer-token characters, with optional trailing padding. Reject
	// controls and combined header values before the provider builds HTTP headers.
	for _, char := range strings.TrimRight(token, "=") {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("-._~+/", char)) {
			return false
		}
	}
	return true
}

func (a *account) subscriptionDestinationSafe() bool {
	if a == nil {
		return false
	}
	cfg := a.config.Load()
	if cfg == nil {
		return false
	}
	pc, ok := cfg.Providers[schemas.Anthropic]
	if !ok || pc.CustomProviderConfig != nil {
		return false
	}
	nc := pc.NetworkConfig
	// The adapter applies configured network headers before caller headers. A
	// configured x-api-key would survive alongside the bearer even with key
	// selection skipped; configured auth overrides are unsupported for subscriptions.
	for name := range nc.ExtraHeaders {
		if strings.EqualFold(name, "authorization") || strings.EqualFold(name, "x-api-key") {
			return false
		}
	}

	// IsSet also detects unresolved secret references; an empty resolved value
	// must not turn an administrator's custom trust root into system trust.
	if nc.InsecureSkipVerify || nc.CACertPEM.IsSet() {
		return false
	}
	if proxy := providerProxy(pc, cfg.Settings["proxy"]); proxy != nil && (proxy.SkipTLSVerify || proxy.CACertPEM.IsSet()) {
		return false
	}
	if nc.BaseURL == "" {
		return true
	}
	u, err := url.Parse(nc.BaseURL)
	return err == nil && u.Scheme == "https" && u.User == nil && strings.EqualFold(u.Hostname(), "api.anthropic.com") && (u.Port() == "" || u.Port() == "443")
}

func supportedSubscription(ctx *schemas.BifrostContext, credentials map[string][]string) bool {
	ua, _ := ctx.Value(schemas.BifrostContextKeyUserAgent).(string)
	skip, _ := ctx.Value(schemas.BifrostContextKeySkipKeySelection).(bool)
	auth := credentials["authorization"]
	// Direct keys take precedence over key skipping in core. A lease cannot
	// authorize a request that would mix that override with subscription auth.
	return len(auth) == 1 && subscriptionBearer(auth[0]) && strings.Join(credentials["x-api-key"], "") == "" && schemas.ClaudeCLI.Matches(ua) && skip && ctx.Value(schemas.BifrostContextKeyDirectKey) == nil
}

func credentialRefusal(status int, kind, message string) *schemas.LLMPluginShortCircuit {
	return &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{
		StatusCode:     schemas.Ptr(status),
		AllowFallbacks: schemas.Ptr(true),
		Error:          &schemas.ErrorField{Type: schemas.Ptr(kind), Message: message},
	}}
}
