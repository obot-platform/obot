package openapi

import (
	"fmt"
	"net/url"
	"strings"
)

// sourceURL checks the schema location; safehttp enforces its network policy.
func sourceURL(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || strings.Contains(value, "\\") {
		return nil, fmt.Errorf("schema source must be an absolute HTTP(S) URL without credentials or fragment")
	}
	return u, nil
}

// destination checks the API base URL against the hosted wrapper contract.
// Network policy must be enforced when the wrapper executes API requests.
func destination(value string) (string, error) {
	u, err := sourceURL(value)
	if err != nil || u.RawQuery != "" || u.ForceQuery || strings.ContainsAny(value, "{}") {
		return "", fmt.Errorf("baseURL must be absolute HTTP(S), without credentials, query, fragment, or variables")
	}
	return strings.TrimRight(u.String(), "/") + "/", nil
}
