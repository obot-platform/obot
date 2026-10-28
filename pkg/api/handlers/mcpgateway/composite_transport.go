package mcpgateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/obot-platform/mmmcp/component"
)

// compositeRequestTransport forwards the current request's loopback credential
// on MCP messages, rather than the credential that opened a pooled connection.
// The receiving gateway authenticates this token and derives attribution from it.
type compositeRequestTransport struct {
	base http.RoundTripper
}

// newCompositeHTTPClient is only for Obot-generated internal component connect URLs.
func newCompositeHTTPClient() *http.Client {
	return &http.Client{Transport: compositeRequestTransport{base: http.DefaultTransport}}
}

func (t compositeRequestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	initial := req
	for initial.Response != nil && initial.Response.Request != nil {
		initial = initial.Response.Request
	}
	if !strings.EqualFold(req.URL.Scheme, initial.URL.Scheme) || !strings.EqualFold(req.URL.Host, initial.URL.Host) {
		// A backend redirect must not disclose an internal gateway credential,
		// including when the HTTP client kept it for a subdomain or another port.
		outbound := req.Clone(req.Context())
		outbound.Header.Del("Authorization")
		return t.base.RoundTrip(outbound)
	}
	if req.Method == http.MethodPost && req.GetBody != nil {
		if authorization := component.RequestHeadersFromContext(req.Context()).Get("Authorization"); authorization != "" {
			// The SDK provides GetBody for its JSON-RPC POSTs. Inspect a copy so
			// the original body and redirect/retry behavior remain intact.
			body, err := req.GetBody()
			if err != nil {
				_ = req.Body.Close()
				return nil, err
			}
			var message struct {
				Method string `json:"method"`
			}
			err = json.NewDecoder(body).Decode(&message)
			_ = body.Close()
			if err != nil {
				_ = req.Body.Close()
				return nil, err
			}
			// Responses to server-initiated requests have no method and retain
			// the opening context, rather than a current frontend credential.
			if message.Method != "" {
				outbound := req.Clone(req.Context())
				outbound.Header.Set("Authorization", authorization)
				return t.base.RoundTrip(outbound)
			}
		}
	}
	// Background responses, SSE reconnects and cleanup have no current frontend
	// message. Keep the OAuth handler's refreshable connection credential: the
	// request context retained at connection creation may contain an expired one.
	return t.base.RoundTrip(req)
}
