package mcpconnect

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

const (
	obotCallbackPath = "/oauth/obot/callback"
)

type callbackResult struct {
	result *auth.AuthorizationResult
	err    error
}

type callbackHandler struct {
	gateway     *url.URL
	openBrowser func(string) error
	mu          sync.Mutex
	state       string
	result      chan callbackResult
}

func (h *callbackHandler) fetch(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	u, err := url.Parse(args.URL)
	if err != nil {
		return nil, err
	}
	state := u.Query().Get("state")
	if state == "" {
		return nil, fmt.Errorf("authorization URL is missing state")
	}
	h.mu.Lock()
	if h.result != nil {
		h.mu.Unlock()
		return nil, fmt.Errorf("authorization already in progress")
	}
	ch := make(chan callbackResult, 1)
	h.state, h.result = state, ch
	h.mu.Unlock()
	defer func() { h.mu.Lock(); h.state, h.result = "", nil; h.mu.Unlock() }()
	if err := h.openBrowser(args.URL); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		return result.result, result.err
	}
}

func (h *callbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != obotCallbackPath {
		q := r.URL.Query()
		if q.Get("state") == "" || (q.Get("code") == "" && q.Get("error") == "") {
			http.Error(w, "missing OAuth callback parameters", http.StatusBadRequest)
			return
		}
		u := *h.gateway
		u.Path = strings.TrimRight(u.Path, "/") + "/oauth/mcp/callback"
		u.RawPath, u.RawQuery, u.Fragment = "", r.URL.RawQuery, ""
		http.Redirect(w, r, u.String(), http.StatusFound)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	q := r.URL.Query()
	if h.result == nil || q.Get("state") == "" || q.Get("state") != h.state {
		http.Error(w, "invalid or expired OAuth state", http.StatusBadRequest)
		return
	}
	result := callbackResult{result: &auth.AuthorizationResult{Code: q.Get("code"), State: q.Get("state"), Iss: q.Get("iss")}}
	if q.Get("error") != "" {
		result.err = fmt.Errorf("authorization failed: %s: %s", q.Get("error"), q.Get("error_description"))
	} else if result.result.Code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}
	h.result <- result
	h.result = nil // consume once, including errors
	h.state = ""
	if result.err != nil {
		http.Error(w, "Authorization failed. See the MCP client's error output.", http.StatusBadRequest)
		return
	}
	_, _ = w.Write([]byte("Authorization received. You can close this window."))
}

func gatewayBaseURL(connectURL string) (*url.URL, error) {
	u, err := url.Parse(connectURL)
	if err != nil {
		return nil, fmt.Errorf("invalid connect URL: %w", err)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("connect URL must be an HTTP(S) Obot connection URL without credentials, query, or fragment")
	}
	prefix, id, ok := strings.Cut(u.Path, "/mcp-connect/")
	if !ok || id == "" || strings.Contains(id, "/") {
		return nil, fmt.Errorf("connect URL must point to /mcp-connect/<id>")
	}
	u.Path, u.RawPath = prefix, ""
	return u, nil
}
