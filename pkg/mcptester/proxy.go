package mcptester

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
)

const (
	anthropicVersion = "2023-06-01"
)

// NewLLMProxyRequest builds the request the tester handler will send through
// Obot's existing LLM gateway. Reusing the browser request's authentication
// headers makes the gateway reconstruct the same user principal, so model
// access policies, message policies, usage metering, and audit identity remain
// on the established proxy path. The supplied context propagates cancellation.
func NewLLMProxyRequest(ctx context.Context, serverURL string, model ResolvedModel, body []byte, inbound http.Header) (*http.Request, error) {
	base, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse Obot server URL: %w", err)
	}
	base.Path = strings.TrimSuffix(base.Path, "/") + "/api/llm-proxy/" + model.ProxyPath + model.APIPath

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create LLM proxy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", types.MCPTesterClientName)
	if model.Dialect == llmtypes.DialectAnthropicMessages {
		req.Header.Set("anthropic-version", anthropicVersion)
	}

	// Forwarding the caller's credentials is safe because serverURL is
	// static configuration (services.Config.ServerURL, derived from the
	// --hostname flag) and therefore always points back at Obot itself.
	for _, header := range []string{"Authorization", "Cookie"} {
		for _, value := range inbound.Values(header) {
			req.Header.Add(header, value)
		}
	}
	return req, nil
}
