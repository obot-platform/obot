package client

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

// UnmarshalJSON reads the flat provider config used by the admin API.
func (p *ProviderConfig) UnmarshalJSON(data []byte) error {
	// The defined type avoids calling this method recursively. The explicit
	// Network field shadows the embedded network_config so we can read its
	// headers before Bifrost's decoder interprets them.
	type providerConfig ProviderConfig
	wire := struct {
		*providerConfig
		Network json.RawMessage `json:"network_config"`
	}{providerConfig: (*providerConfig)(p)}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	return decodeProviderNetwork(wire.Network, &p.NetworkConfig)
}

// UnmarshalJSON reads the stored provider resource, where Config is nested
// alongside Keys. Capture Config so storage and the admin API share the
// literal-header decoder.
func (p *Provider) UnmarshalJSON(data []byte) error {
	type provider Provider
	wire := struct {
		*provider
		Config json.RawMessage
	}{provider: (*provider)(p)}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if len(wire.Config) == 0 {
		return nil
	}
	config := ProviderConfig{ProviderConfig: p.Config}
	if err := json.Unmarshal(wire.Config, &config); err != nil {
		return err
	}
	p.Config = config.ProviderConfig
	return nil
}

// Obot treats provider headers as literal strings. Bifrost reads strings and
// typed secret objects, but writes headers as strings for compatibility. A
// typed plaintext value such as "env.MY_VAR" would lose its type on save and
// be interpreted as a secret reference on the next read.
func decodeProviderNetwork(data json.RawMessage, network *schemas.NetworkConfig) error {
	if len(data) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var headers map[string]string
	for name, raw := range fields {
		if strings.EqualFold(name, "extra_headers") {
			if err := json.Unmarshal(raw, &headers); err != nil {
				return fmt.Errorf("decode provider extra headers: %w", err)
			}
			delete(fields, name)
		}
	}
	// Let Bifrost decode the remaining settings, including retry durations,
	// after removing the headers that would trigger secret resolution.
	normalized, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(normalized, network); err != nil {
		return err
	}
	if headers != nil {
		network.ExtraHeaders = make(map[string]schemas.SecretVar, len(headers))
		for name, value := range headers {
			network.ExtraHeaders[name] = schemas.SecretVar{Val: value, SecretType: schemas.SecretTypePlainText}
		}
	}
	return nil
}
