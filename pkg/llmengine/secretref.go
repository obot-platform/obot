package llmengine

import (
	"bytes"
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

// HasSecretValue reports whether a JSON value the engine accepts as an optional secret, such as
// a CA certificate, holds a value. JSON null leaves the setting unset.
func HasSecretValue(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && SecretValue(trimmed) != ""
}

// IsMaskedSecret reports whether a JSON value is a secret as the engine lists it, redacted,
// which the engine takes to mean "keep the stored value" when it is sent back in an update.
func IsMaskedSecret(raw []byte) bool {
	var v schemas.SecretVar
	if err := v.UnmarshalJSON(raw); err != nil {
		return false
	}
	return v.IsMaskedPlaceholder()
}

// AnthropicIsDirect reports whether the engine's Anthropic provider sends requests straight to
// Anthropic over verified TLS: no other base URL, and no setting that would let another
// server present a certificate for Anthropic's host, whether in the provider's network or
// proxy settings or in the gateway's global proxy. A caller's Claude subscription token is
// only forwarded when it does, so an administrator can't redirect users' tokens to a server
// of their own. A proxy without such settings, such as the environment's egress proxy, only
// relays the TLS connection to Anthropic, so it doesn't stop the token being forwarded.
func (e *Engine) AnthropicIsDirect(ctx context.Context) bool {
	var provider struct {
		NetworkConfig struct {
			BaseURL            string          `json:"base_url"`
			InsecureSkipVerify bool            `json:"insecure_skip_verify"`
			CACertPEM          json.RawMessage `json:"ca_cert_pem"`
		} `json:"network_config"`
		ProxyConfig struct {
			SkipTLSVerify bool            `json:"skip_tls_verify"`
			CACertPEM     json.RawMessage `json:"ca_cert_pem"`
		} `json:"proxy_config"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers/anthropic", nil, &provider); err != nil {
		return false
	}
	nc, pc := provider.NetworkConfig, provider.ProxyConfig
	if nc.InsecureSkipVerify || HasSecretValue(nc.CACertPEM) {
		return false
	}
	if pc.SkipTLSVerify || HasSecretValue(pc.CACertPEM) {
		return false
	}
	// The engine applies its global proxy's skip_tls_verify to the TLS settings of the
	// clients it builds, so it is treated as turning certificate checks off whether or not
	// the proxy itself is on.
	var globalProxy struct {
		SkipTLSVerify bool `json:"skip_tls_verify"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/proxy-config", nil, &globalProxy); err != nil || globalProxy.SkipTLSVerify {
		return false
	}
	if nc.BaseURL == "" {
		return true
	}
	u, err := url.Parse(nc.BaseURL)
	return err == nil && u.Scheme == "https" && u.User == nil && strings.EqualFold(u.Hostname(), "api.anthropic.com") && (u.Port() == "" || u.Port() == "443")
}
