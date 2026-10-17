package llmengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ProviderChange is a provider change made through the admin API, after GuardProviderChange
// checked it.
type ProviderChange struct {
	// Body is the request body to send to the engine.
	Body []byte
}

// GuardProviderChange checks a change to a provider made through the admin API, where path is
// the engine API path ("providers" or "providers/<name>"):
//
//   - An update that leaves out a custom provider's custom_provider_config keeps it. The
//     engine treats a missing custom_provider_config as removing it, which it refuses.
func (e *Engine) GuardProviderChange(ctx context.Context, method, path string, body []byte) (ProviderChange, error) {
	change := ProviderChange{Body: body}
	if method != http.MethodPut {
		return change, nil
	}
	parts := strings.Split(path, "/")
	if parts[0] != "providers" || len(parts) != 2 || !json.Valid(body) {
		return change, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		// Not an object; the engine refuses it.
		return change, nil
	}
	return e.guardProviderUpdate(ctx, parts[1], fields)
}

func (e *Engine) guardProviderUpdate(ctx context.Context, name string, fields map[string]json.RawMessage) (ProviderChange, error) {
	var current map[string]json.RawMessage
	if err := e.Do(ctx, http.MethodGet, "/api/providers/"+url.PathEscape(name), nil, &current); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			// An update creates a provider that doesn't exist yet.
			body, err := json.Marshal(fields)
			return ProviderChange{Body: body}, err
		}
		return ProviderChange{}, fmt.Errorf("failed to read LLM gateway provider %s: %w", name, err)
	}

	if cpc, ok := current["custom_provider_config"]; ok && !isNull(cpc) && fieldNamed(fields, "custom_provider_config") == "" {
		fields["custom_provider_config"] = cpc
	}
	body, err := json.Marshal(fields)
	return ProviderChange{Body: body}, err
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
