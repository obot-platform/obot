// Package mcplogin relays one UI OAuth attempt. It never creates an MCP session
// or reads, stores, or uses Obot or provider access tokens.
package mcplogin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/pkg/browser"
)

const (
	completionPath = "/oauth/obot/callback"
)

func Run(ctx context.Context, loginURL string) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return run(ctx, loginURL, &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, func(u string) error {
		fmt.Fprintln(os.Stderr, "Opening browser to complete the authentication requested by Obot.")
		browser.Stdout = os.Stderr
		if err := browser.OpenURL(u); err != nil {
			fmt.Fprintf(os.Stderr, "Open this URL in your browser:\n%s\n", u)
		}
		return nil
	})
}

func run(ctx context.Context, loginURL string, client *http.Client, openBrowser func(string) error) error {
	endpoint, err := url.Parse(loginURL)
	if err != nil || endpoint.User != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return fmt.Errorf("--url must be the UI authentication URL from Obot")
	}
	prefix, state, ok := strings.Cut(endpoint.Path, "/oauth/mcp/login/")
	if !ok || state == "" || strings.Contains(state, "/") {
		return fmt.Errorf("--url must be the UI authentication URL from Obot, not an MCP connection URL")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("load authentication attempt: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication attempt is unavailable (HTTP %d); retry authentication in Obot", response.StatusCode)
	}
	var attempt types.MCPLocalLogin
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&attempt); err != nil {
		return fmt.Errorf("read authentication attempt: %w", err)
	}
	redirect, err := url.Parse(attempt.RedirectURL)
	if err != nil {
		return fmt.Errorf("invalid callback URL")
	}
	port, err := strconv.Atoi(redirect.Port())
	if err != nil || port < 1024 || port > 65535 || redirect.Scheme != "http" || redirect.Hostname() != "localhost" || redirect.User != nil || redirect.RawQuery != "" || redirect.Fragment != "" || redirect.Path == "" || mcp.ValidateLocalhostCallbackPath(redirect.EscapedPath()) != nil {
		return fmt.Errorf("authentication attempt has an invalid localhost callback URL")
	}
	authorization, err := url.Parse(attempt.AuthorizationURL)
	if err != nil || (authorization.Scheme != "https" && authorization.Scheme != "http") || authorization.Hostname() == "" || authorization.User != nil || attempt.State != state || authorization.Query().Get("state") != state || authorization.Query().Get("redirect_uri") != attempt.RedirectURL {
		return fmt.Errorf("authentication attempt has an invalid authorization URL")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", redirect.Port()))
	if err != nil {
		return fmt.Errorf("listen for OAuth callback: %w; retry authentication in Obot to get a new command", err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	callback := *endpoint
	callback.Path, callback.RawPath = prefix+"/oauth/mcp/callback", ""
	result := make(chan error, 1)
	var relayed atomic.Bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("state") != state {
			http.Error(w, "invalid OAuth state", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case redirect.Path:
			if r.URL.Query().Get("code") == "" && r.URL.Query().Get("error") == "" {
				http.Error(w, "missing OAuth callback parameters", http.StatusBadRequest)
				return
			}
			if !relayed.CompareAndSwap(false, true) {
				http.Error(w, "callback already received", http.StatusBadRequest)
				return
			}
			target := callback
			target.RawQuery = r.URL.RawQuery
			http.Redirect(w, r, target.String(), http.StatusFound)
		case completionPath:
			if !relayed.Load() || (r.URL.Query().Get("status") != "complete" && r.URL.Query().Get("error") == "") {
				http.Error(w, "unexpected completion", http.StatusBadRequest)
				return
			}
			var loginErr error
			if r.URL.Query().Get("error") != "" {
				loginErr = fmt.Errorf("authentication failed; retry authentication in Obot")
				http.Error(w, "Authentication failed. Return to Obot to retry.", http.StatusBadRequest)
			} else {
				_, _ = io.WriteString(w, "Authentication complete. Return to Obot to continue.")
			}
			select {
			case result <- loginErr:
			default:
			}
		default:
			http.NotFound(w, r)
		}
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			select {
			case result <- err:
			default:
			}
		}
	}()
	defer func() {
		shutdownCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := openBrowser(attempt.AuthorizationURL); err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return fmt.Errorf("waiting for authentication: %w", ctx.Err())
	}
}
