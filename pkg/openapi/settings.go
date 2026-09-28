package openapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// wrapperSettings is the OPENAPI_CONFIG_JSON wire format, not a catalog entry.
// OpenAPIRuntimeConfig also stores the import source and schema, which must not
// enter this environment variable. Credentials are represented by names only;
// their values and prefixes stay in Obot's per-request header handling.
type wrapperSettings struct {
	BaseURL           string   `json:"baseURL"`
	CredentialHeaders []string `json:"credentialHeaders"`
}

func (s wrapperSettings) marshal() ([]byte, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxSettingsBytes {
		return nil, fmt.Errorf("OpenAPI settings exceed 96 KiB")
	}
	return data, nil
}

// SettingsJSON validates the selected credential definitions and returns exactly
// the wrapper's settings contract. Call after Parse, before persisting/deploying.
// Header values and prefixes are deliberately absent from the resulting JSON.
func SettingsJSON(result *Result, headers []types.MCPConfig) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("a validated schema import is required")
	}
	names := make([]string, 0, len(headers))
	seen := map[string]bool{}
	for _, header := range headers {
		if header.Usage != types.Header {
			return nil, fmt.Errorf("OpenAPI credentials must be header inputs")
		}
		if err := validateHeader(header.Key); err != nil {
			return nil, err
		}
		key := strings.ToLower(header.Key)
		if seen[key] {
			return nil, fmt.Errorf("duplicate credential header names")
		}
		seen[key] = true
		names = append(names, header.Key)
	}
	base, err := destination(result.BaseURL)
	if err != nil {
		return nil, err
	}
	if len(names) > 0 && !strings.HasPrefix(base, "https://") {
		return nil, fmt.Errorf("credential forwarding requires an HTTPS API destination")
	}
	settings := wrapperSettings{
		BaseURL:           base,
		CredentialHeaders: names,
	}
	return settings.marshal()
}
