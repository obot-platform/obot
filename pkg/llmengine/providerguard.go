package llmengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/safehttp"
)

var (
	// keyDialTargets are the fields of a provider key that decide where the engine connects
	// with the key's credentials, as paths into the key's JSON. An "*" matches every entry of
	// an object.
	keyDialTargets = [][]string{
		{"ollama_key_config", "url"},
		{"sgl_key_config", "url"},
		{"vllm_key_config", "url"},
		{"azure_key_config", "endpoint"},
		{"databricks_key_config", "workspace_url"},
		{"oauth_key_config", "token_url"},
		{"bedrock_key_config", "endpoints", "*"},
		{"bedrock_mantle_key_config", "endpoints", "*"},
		{"aliases", "*", "endpoint"},
	}
)

// ProviderChange is a provider change made through the admin API, after GuardProviderChange
// checked it.
type ProviderChange struct {
	// Body is the request body to send to the engine.
	Body []byte
	// RemoveKeys are the IDs of the provider's keys to remove before the change: its base URL
	// changes, and the caller asked for its keys to be removed rather than sent to the new
	// address.
	RemoveKeys []string
}

type networkConfig struct {
	BaseURL             string `json:"base_url"`
	AllowPrivateNetwork bool   `json:"allow_private_network"`
}

// GuardProviderChange checks a change to a provider or provider key made through the admin
// API, where path is the engine API path ("providers", "providers/<name>",
// "providers/<name>/keys" or "providers/<name>/keys/<id>"):
//
//   - Every address the provider connects to (its base URL and its keys' server URLs) may be
//     a loopback or private network address only when the provider allows private network
//     addresses. Link-local and cloud metadata addresses are never allowed. The engine itself
//     lets loopback addresses through, and doesn't know every cloud metadata address.
//   - A provider's base URL can't change while it has keys, so stored keys aren't sent to a
//     new address. With removeKeys, the keys are removed instead (see ProviderChange).
//   - A key's server URL can't change without its secrets being entered again.
//   - An update that leaves out a custom provider's custom_provider_config keeps it. The
//     engine treats a missing custom_provider_config as removing it, which it refuses.
func (e *Engine) GuardProviderChange(ctx context.Context, method, path string, body []byte, removeKeys bool) (ProviderChange, error) {
	change := ProviderChange{Body: body}
	if method != http.MethodPost && method != http.MethodPut {
		return change, nil
	}
	parts := strings.Split(path, "/")
	if parts[0] != "providers" || len(parts) > 4 || !json.Valid(body) {
		return change, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		// Not an object; the engine refuses it.
		return change, nil
	}

	switch {
	case len(parts) == 1 && method == http.MethodPost:
		return change, checkProviderAddresses(ctx, fields, nil)
	case len(parts) == 2 && method == http.MethodPut:
		return e.guardProviderUpdate(ctx, parts[1], fields, removeKeys)
	case len(parts) == 3 && parts[2] == "keys" && method == http.MethodPost:
		return change, e.guardKey(ctx, parts[1], fields, nil)
	case len(parts) == 4 && parts[2] == "keys" && method == http.MethodPut:
		keys, err := e.providerKeys(ctx, parts[1])
		if err != nil {
			// The engine reports the missing provider or key.
			return change, nil
		}
		for _, key := range keys {
			if id, _ := key["id"].(string); id == parts[3] {
				stored, _ := json.Marshal(key)
				return change, e.guardKey(ctx, parts[1], fields, stored)
			}
		}
	}
	return change, nil
}

func (e *Engine) guardProviderUpdate(ctx context.Context, name string, fields map[string]json.RawMessage, removeKeys bool) (ProviderChange, error) {
	var current map[string]json.RawMessage
	if err := e.Do(ctx, http.MethodGet, "/api/providers/"+url.PathEscape(name), nil, &current); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			// An update creates a provider that doesn't exist yet.
			body, _ := json.Marshal(fields)
			return ProviderChange{Body: body}, checkProviderAddresses(ctx, fields, nil)
		}
		return ProviderChange{}, fmt.Errorf("failed to read LLM gateway provider %s: %w", name, err)
	}

	keys, err := e.providerKeys(ctx, name)
	if err != nil {
		return ProviderChange{}, fmt.Errorf("failed to read the keys of LLM gateway provider %s: %w", name, err)
	}
	if err := checkProviderAddresses(ctx, fields, keys); err != nil {
		return ProviderChange{}, err
	}

	change := ProviderChange{}
	before, _ := networkSettings(current)
	after, _ := networkSettings(fields)
	if normalizeBaseURL(before.BaseURL) != normalizeBaseURL(after.BaseURL) && len(keys) > 0 {
		if !removeKeys {
			return ProviderChange{}, types.NewErrBadRequest("changing a provider's base URL removes its keys, so they aren't sent to the new address: remove the keys first, or confirm their removal, and add them again after the change")
		}
		for _, key := range keys {
			if id, _ := key["id"].(string); id != "" {
				change.RemoveKeys = append(change.RemoveKeys, id)
			}
		}
	}

	if cpc, ok := current["custom_provider_config"]; ok && !isNull(cpc) && fieldNamed(fields, "custom_provider_config") == "" {
		fields["custom_provider_config"] = cpc
	}
	change.Body, err = json.Marshal(fields)
	return change, err
}

// guardKey checks a new key, or an update of the stored key (redacted, as the engine lists it).
func (e *Engine) guardKey(ctx context.Context, provider string, fields map[string]json.RawMessage, stored []byte) error {
	var current map[string]json.RawMessage
	if err := e.Do(ctx, http.MethodGet, "/api/providers/"+url.PathEscape(provider), nil, &current); err != nil {
		// The engine reports the missing provider.
		return nil
	}
	network, _ := networkSettings(current)

	body, _ := json.Marshal(fields)
	changed := false
	for _, target := range keyDialTargets {
		for p, raw := range valuesAt(body, target) {
			address := SecretValue(raw)
			if address == "" {
				continue
			}
			if stored != nil {
				if old, ok := valuesAt(stored, target)[p]; ok && SecretValue(old) == address {
					continue
				}
				changed = true
			}
			if err := checkDestination(ctx, p, address, network.AllowPrivateNetwork); err != nil {
				return err
			}
		}
	}
	if !changed {
		return nil
	}

	// The key now connects somewhere else: its stored secrets must be entered again, so they
	// can't be redirected to another host.
	for p, raw := range redactedValues(stored, nil) {
		entered, ok := valueAt(body, strings.Split(p, "."))
		var v schemas.SecretVar
		if !ok || v.UnmarshalJSON(entered) != nil || v.GetValue() == "" || v.IsRedacted() || bytes.Equal(bytes.TrimSpace(entered), bytes.TrimSpace(raw)) {
			return types.NewErrBadRequest("changing where a key connects requires entering its secrets again (%s)", p)
		}
	}
	return nil
}

// checkProviderAddresses checks a provider's base URL, and its keys' server URLs, against the
// address policy its allow_private_network setting chooses.
func checkProviderAddresses(ctx context.Context, fields map[string]json.RawMessage, keys []map[string]any) error {
	network, err := networkSettings(fields)
	if err != nil {
		// The engine refuses it.
		return nil
	}
	if network.BaseURL != "" {
		if err := checkDestination(ctx, "base URL", network.BaseURL, network.AllowPrivateNetwork); err != nil {
			return err
		}
	}
	for _, key := range keys {
		data, _ := json.Marshal(key)
		for _, target := range keyDialTargets {
			for p, raw := range valuesAt(data, target) {
				if address := SecretValue(raw); address != "" {
					if err := checkDestination(ctx, fmt.Sprintf("key %v %s", key["name"], p), address, network.AllowPrivateNetwork); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// checkDestination checks an address a provider connects to. Loopback and private network
// addresses are allowed only when allowPrivate is set. Link-local, unspecified and cloud
// metadata addresses never are. Host names are resolved, and every address they resolve to
// is checked.
func checkDestination(ctx context.Context, what, address string, allowPrivate bool) error {
	err := safehttp.ValidateURL(ctx, address, safehttp.Options{
		BlockLoopback:  !allowPrivate,
		BlockPrivateIP: !allowPrivate,
		BlockLinkLocal: true,
	})
	if err == nil {
		u, _ := url.Parse(address)
		if ip := net.ParseIP(u.Hostname()); ip == nil || !ip.IsUnspecified() {
			return nil
		}
		err = &safehttp.BlockedAddressError{Host: u.Hostname(), Reason: "unspecified", IP: net.ParseIP(u.Hostname())}
	}
	var blocked *safehttp.BlockedAddressError
	switch {
	case errors.As(err, &blocked) && (blocked.Reason == "loopback" || blocked.Reason == "private"):
		return types.NewErrBadRequest("invalid %s %s: %s is a %s network address, which needs Allow private network addresses", what, address, blocked.IP, blocked.Reason)
	case errors.As(err, &blocked):
		return types.NewErrBadRequest("invalid %s %s: %s is a %s address, which can't be used", what, address, blocked.IP, blocked.Reason)
	default:
		return types.NewErrBadRequest("invalid %s %s: %v", what, address, err)
	}
}

// networkSettings returns a provider's network settings, matching field names in any case as
// the engine does.
func networkSettings(fields map[string]json.RawMessage) (networkConfig, error) {
	var network networkConfig
	if name := fieldNamed(fields, "network_config"); name != "" && !isNull(fields[name]) {
		err := json.Unmarshal(fields[name], &network)
		return network, err
	}
	return network, nil
}

// normalizeBaseURL makes equivalent spellings of a base URL equal.
func normalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String()
}

// fieldNamed returns the key of fields that matches name in any case, as the engine matches
// field names, or "" if there is none.
func fieldNamed(fields map[string]json.RawMessage, name string) string {
	if _, ok := fields[name]; ok {
		return name
	}
	for k := range fields {
		if strings.EqualFold(k, name) {
			return k
		}
	}
	return ""
}

func isNull(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null"
}

// valueAt returns the JSON value at path, matching field names in any case.
func valueAt(data []byte, path []string) (json.RawMessage, bool) {
	raw := json.RawMessage(data)
	for _, name := range path {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return nil, false
		}
		key := fieldNamed(fields, name)
		if key == "" {
			return nil, false
		}
		raw = fields[key]
	}
	return raw, !isNull(raw)
}

// valuesAt returns the JSON values at path, where "*" matches every entry of an object, by
// their dotted path.
func valuesAt(data []byte, path []string) map[string]json.RawMessage {
	result := map[string]json.RawMessage{}
	var walk func(raw json.RawMessage, rest []string, at string)
	walk = func(raw json.RawMessage, rest []string, at string) {
		if isNull(raw) {
			return
		}
		if len(rest) == 0 {
			result[strings.TrimPrefix(at, ".")] = raw
			return
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return
		}
		if rest[0] == "*" {
			for k, v := range fields {
				walk(v, rest[1:], at+"."+k)
			}
			return
		}
		if key := fieldNamed(fields, rest[0]); key != "" {
			walk(fields[key], rest[1:], at+"."+rest[0])
		}
	}
	walk(data, path, "")
	return result
}

// redactedValues returns the values in a key as the engine lists it that it redacted, which
// are its secrets, by their dotted path.
func redactedValues(data []byte, at []string) map[string]json.RawMessage {
	result := map[string]json.RawMessage{}
	if data == nil {
		return result
	}
	var v schemas.SecretVar
	trimmed := bytes.TrimSpace(data)
	// Below the key itself, a string or an object with a value is a secret's value.
	if len(at) > 0 && len(trimmed) > 0 && (trimmed[0] == '"' || (trimmed[0] == '{' && hasField(trimmed, "value"))) {
		if v.UnmarshalJSON(trimmed) == nil && v.IsRedacted() {
			result[strings.Join(at, ".")] = trimmed
		}
		return result
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(trimmed, &fields) != nil {
		return result
	}
	for k, child := range fields {
		maps.Copy(result, redactedValues(child, append(append([]string{}, at...), k)))
	}
	return result
}

func hasField(data []byte, name string) bool {
	var fields map[string]json.RawMessage
	return json.Unmarshal(data, &fields) == nil && fieldNamed(fields, name) != ""
}
