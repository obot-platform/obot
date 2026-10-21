package server

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"

	"github.com/obot-platform/obot/pkg/api"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
)

const (
	// anthropicCredentialPrefix begins every credential Anthropic issues: API keys, OAuth
	// tokens and Claude session tokens.
	anthropicCredentialPrefix = "sk-ant-"
)

var (
	// anthropicRequestHeaders are the request headers sent on to Anthropic. The others, such
	// as cookies and the caller's Obot credentials, stay in Obot.
	anthropicRequestHeaders = []string{"Accept", "Anthropic-Beta", "Anthropic-Version", "Content-Type", "User-Agent",
		"Sec-Websocket-Extensions", "Sec-Websocket-Key", "Sec-Websocket-Protocol", "Sec-Websocket-Version"}
)

// anthropicProxy forwards the Anthropic API requests Claude Code sends to its API base URL
// that name no model for the gateway to serve, so a Claude Code session using the LLM proxy
// keeps working. They are never sent with a configured provider key. They aren't model
// requests, so they are left out of the LLM audit log; the egress audit log records the
// requests Claude environment sessions send.
type anthropicProxy struct {
	upstream  url.URL
	transport http.RoundTripper
}

func newAnthropicProxy() *anthropicProxy {
	return &anthropicProxy{
		upstream:  url.URL{Scheme: "https", Host: "api.anthropic.com"},
		transport: http.DefaultTransport,
	}
}

// files forwards Files API requests: file uploads, downloads and listings. Only the caller's
// own Anthropic credential can reach the caller's files, so each request is sent with it.
func (a *anthropicProxy) files(req api.Context) error {
	target := "/v1/files"
	if rest := req.PathValue("path"); rest != "" {
		target = path.Join(target, rest)
		if !strings.HasPrefix(target, "/v1/files/") {
			llmtypes.WriteError(req.ResponseWriter, req.Request, "unsupported Files API path", http.StatusBadRequest)
			return nil
		}
	}

	header, credential, ok := callerAnthropicCredential(req.Request.Header)
	if !ok {
		llmtypes.WriteError(req.ResponseWriter, req.Request, "Files API requests are sent to Anthropic with the caller's own Anthropic credential, and this request has none", http.StatusUnauthorized)
		return nil
	}
	a.forward(req, target, header, credential)
	return nil
}

// agentProxyCACert forwards Claude Code's request for the CA certificate of Anthropic's
// agent proxy, which the session's git and tool traffic is checked against. The certificate
// is public, so the request is sent without a credential.
func (a *anthropicProxy) agentProxyCACert(req api.Context) error {
	a.forward(req, "/v1/code/agent-proxy/ca-cert", "", "")
	return nil
}

// agentProxyWebSocket forwards the WebSocket that carries Claude Code's agent-proxy
// tunnel, which its git and tool traffic goes through, with the caller's own Anthropic
// credential.
func (a *anthropicProxy) agentProxyWebSocket(req api.Context) error {
	header, credential, ok := callerAnthropicCredential(req.Request.Header)
	if !ok {
		header, credential = "", ""
	}
	a.forward(req, "/v1/code/agent-proxy/ws", header, credential)
	return nil
}

// forward sends the request to target on Anthropic, with only anthropicRequestHeaders and,
// when header is set, the caller's credential in it.
func (a *anthropicProxy) forward(req api.Context, target, header, credential string) {
	(&httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			u := a.upstream
			u.Path, u.RawQuery = target, r.In.URL.RawQuery
			r.Out.URL, r.Out.Host = &u, u.Host
			// The proxy sets these for a connection upgrade, such as a WebSocket.
			upgrade, connection := r.Out.Header.Get("Upgrade"), r.Out.Header.Get("Connection")
			r.Out.Header = http.Header{}
			if upgrade != "" {
				r.Out.Header.Set("Connection", connection)
				r.Out.Header.Set("Upgrade", upgrade)
			}
			for _, name := range anthropicRequestHeaders {
				if values := r.In.Header.Values(name); len(values) > 0 {
					r.Out.Header[name] = values
				}
			}
			if header != "" {
				r.Out.Header.Set(header, credential)
			}
		},
		Transport: a.transport,
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("Anthropic API proxy error", "path", target, "error", err)
			llmtypes.WriteError(w, r, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		},
	}).ServeHTTP(req.ResponseWriter, req.Request)
}

// callerAnthropicCredential returns the header and value of the caller's Anthropic
// credential. Only credentials Anthropic issued are forwarded: Obot also accepts its own API
// keys, JWTs and tokens in Authorization and X-Api-Key, and Anthropic must never receive
// them. It reports false when there is no Anthropic credential, or when the request carries
// any other credential.
func callerAnthropicCredential(h http.Header) (string, string, bool) {
	var header, credential string
	for _, name := range []string{"Authorization", "X-Api-Key"} {
		value := h.Get(name)
		if value == "" {
			continue
		}
		token := value
		if len(token) > len("Bearer ") && strings.EqualFold(token[:len("Bearer ")], "Bearer ") {
			token = token[len("Bearer "):]
		}
		if !strings.HasPrefix(token, anthropicCredentialPrefix) {
			return "", "", false
		}
		if header == "" {
			header, credential = name, value
		}
	}
	return header, credential, header != ""
}
