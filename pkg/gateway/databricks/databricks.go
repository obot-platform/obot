package databricks

import (
	"fmt"
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
	token        string
	workspaceURL url.URL
	next         http.RoundTripper
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

// Transport authenticates only requests to the configured workspace origin,
// ensuring provider credentials are not sent to discovery or redirect targets.
func Transport(credEnv map[string]string, next http.RoundTripper) (http.RoundTripper, error) {
	workspaceURL, err := BaseURL(credEnv)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(credEnv[TokenEnv])
	if token == "" {
		return nil, fmt.Errorf("credential %q is missing or empty", TokenEnv)
	}
	if next == nil {
		next = http.DefaultTransport
	}
	return transport{
		token:        token,
		workspaceURL: workspaceURL,
		next:         next,
	}, nil
}

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Del("Authorization")
	req.Header.Del("X-Api-Key")
	if strings.EqualFold(req.URL.Scheme, t.workspaceURL.Scheme) &&
		strings.EqualFold(req.URL.Host, t.workspaceURL.Host) {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.next.RoundTrip(req)
}
