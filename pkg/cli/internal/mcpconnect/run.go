package mcpconnect

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/adrg/xdg"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pkg/browser"
)

func Run(ctx context.Context, connectURL string, callbackPaths ...string) error {
	gateway, err := gatewayBaseURL(connectURL)
	if err != nil {
		return err
	}
	paths, err := allowedCallbackPaths(callbackPaths)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for OAuth callback: %w", err)
	}
	defer listener.Close()
	redirectURL := fmt.Sprintf("http://localhost:%d%s", listener.Addr().(*net.TCPAddr).Port, obotCallbackPath)
	callback := &callbackHandler{gateway: gateway, providerPaths: paths, openBrowser: func(u string) error {
		fmt.Fprintln(os.Stderr, "Opening browser to authenticate with Obot.")
		browser.Stdout = os.Stderr
		if err := browser.OpenURL(u); err != nil {
			fmt.Fprintf(os.Stderr, "Open this URL in your browser to authenticate:\n%s\n", u)
		}
		return nil
	}}
	server := &http.Server{Handler: callback, ReadHeaderTimeout: 10 * time.Second}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Serve(listener) }()
	defer server.Close()
	go func() {
		select {
		case <-ctx.Done():
		case <-serverErr:
			cancel()
		}
	}()
	dir := filepath.Join(xdg.DataHome, "obot", "mcp-connect")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	handler, err := newOAuthHandler(ctx, dir, connectURL, redirectURL, callback, &http.Client{Timeout: 30 * time.Second})
	if err != nil {
		return err
	}
	if err := authenticate(ctx, connectURL, handler, &http.Client{Timeout: 30 * time.Second}); err != nil {
		return err
	}
	local, err := (&gomcp.StdioTransport{}).Connect(ctx)
	if err != nil {
		return err
	}
	return bridge(ctx, local, connectURL, handler, http.DefaultTransport)
}
