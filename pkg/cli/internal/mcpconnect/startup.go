package mcpconnect

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// authenticate probes the endpoint before reading stdin so an interactive launch
// can complete OAuth without an MCP client. GET does not create an MCP session;
// the downstream client's initialize request still negotiates the real session.
func authenticate(ctx context.Context, endpoint string, handler auth.OAuthHandler, client *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	source, err := handler.TokenSource(ctx)
	if err != nil {
		return err
	}
	if source != nil {
		token, err := source.Token()
		if err != nil {
			return err
		}
		token.SetAuthHeader(req)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("probe MCP authentication: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return handler.Authorize(ctx, req, resp)
	}
	// Servers may require a session ID for GET or disable standalone SSE.
	// Authentication for those responses is deferred to the actual MCP request.
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("probe MCP authentication: HTTP %d", resp.StatusCode)
}
