package llmengine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

// IsSecretReference reports whether the engine would read a JSON value from the host's
// environment or a vault, rather than take it as given, wherever it accepts a secret. It
// decodes the value with the engine's own decoder, so it can't disagree with the engine on
// spelling, such as key names in another case.
func IsSecretReference(raw []byte) bool {
	var v schemas.SecretVar
	if err := v.UnmarshalJSON(raw); err != nil {
		return false
	}
	return v.IsFromSecret()
}

// SecretValue returns the value the engine reads from a JSON value it accepts as a secret,
// such as a region, which may be a string or an object holding one.
func SecretValue(raw []byte) string {
	var v schemas.SecretVar
	if err := v.UnmarshalJSON(raw); err != nil {
		return ""
	}
	return v.GetValue()
}

// AnthropicIsDirect reports whether the engine's Anthropic provider sends requests straight to
// Anthropic over verified TLS: no other base URL, and no setting that would let another
// server present a certificate for Anthropic's host. A caller's Claude subscription token is
// only forwarded when it does, so an administrator can't redirect users' tokens to a server
// of their own.
func (e *Engine) AnthropicIsDirect(ctx context.Context) bool {
	var provider struct {
		NetworkConfig struct {
			BaseURL            string          `json:"base_url"`
			InsecureSkipVerify bool            `json:"insecure_skip_verify"`
			CACertPEM          json.RawMessage `json:"ca_cert_pem"`
		} `json:"network_config"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers/anthropic", nil, &provider); err != nil {
		return false
	}
	nc := provider.NetworkConfig
	if nc.InsecureSkipVerify || (len(nc.CACertPEM) > 0 && SecretValue(nc.CACertPEM) != "") {
		return false
	}
	if nc.BaseURL == "" {
		return true
	}
	u, err := url.Parse(nc.BaseURL)
	return err == nil && u.Scheme == "https" && u.User == nil && strings.EqualFold(u.Hostname(), "api.anthropic.com") && (u.Port() == "" || u.Port() == "443")
}
