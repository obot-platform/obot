package databricks

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/system"
)

const (
	WorkspaceURLEnv = "OBOT_DATABRICKS_MODEL_PROVIDER_WORKSPACE_URL"
	TokenEnv        = "OBOT_DATABRICKS_MODEL_PROVIDER_TOKEN"
)

type transport struct {
	token string
	next  http.RoundTripper
}

// IsProvider reports whether providerName identifies the Databricks model provider.
func IsProvider(providerName string) bool {
	return providerName == system.DatabricksModelProvider
}

// ResponsesPath returns the Databricks serving endpoint path for a Responses dialect.
func ResponsesPath(dialect llmtypes.Dialect) (string, error) {
	switch dialect {
	case llmtypes.DialectOpenAIResponses:
		return "responses", nil
	case llmtypes.DialectOpenResponses:
		return "open-responses", nil
	default:
		return "", fmt.Errorf("unsupported Databricks model dialect %q", dialect)
	}
}

// BaseURL validates and returns the configured Databricks workspace URL.
func BaseURL(credEnv map[string]string) (url.URL, error) {
	raw := strings.TrimSpace(credEnv[WorkspaceURLEnv])
	if raw == "" {
		return url.URL{}, fmt.Errorf("credential %q is missing or empty", WorkspaceURLEnv)
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse Databricks workspace URL: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return url.URL{}, fmt.Errorf("databricks workspace URL must be an absolute HTTPS URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return url.URL{}, fmt.Errorf("databricks workspace URL must contain only the scheme and host")
	}
	u.Path = ""
	return *u, nil
}

// Transport returns a transport that authenticates workspace requests while
// ensuring provider credentials are never sent to the local discovery daemon.
func Transport(credEnv map[string]string, next http.RoundTripper) (http.RoundTripper, error) {
	token := strings.TrimSpace(credEnv[TokenEnv])
	if token == "" {
		return nil, fmt.Errorf("credential %q is missing or empty", TokenEnv)
	}
	if next == nil {
		next = http.DefaultTransport
	}
	return transport{token: token, next: next}, nil
}

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Del("Authorization")
	req.Header.Del("X-Api-Key")
	if !isLoopback(req.URL.Hostname()) {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.next.RoundTrip(req)
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
