package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

type backend interface {
	ensureServerDeployment(ctx context.Context, server ServerConfig, id, mcpServerDisplayName, mcpServerName string) (ServerConfig, error)
	transformConfig(ctx context.Context, id string, serverConfig ServerConfig) (*ServerConfig, error)
	streamServerLogs(ctx context.Context, id string) (io.ReadCloser, error)
	getServerDetails(ctx context.Context, id string) (types.MCPServerDetails, error)
	restartServer(ctx context.Context, id string, serverConfig ServerConfig) error
	shutdownServer(ctx context.Context, id string) error
}

type ErrNotSupportedByBackend struct {
	Feature, Backend string
}

func (e *ErrNotSupportedByBackend) Error() string {
	return fmt.Sprintf("feature %s is not supported by %s backend", e.Feature, e.Backend)
}

func ensureServerReady(ctx context.Context, url string, server ServerConfig) error {
	// Ensure we can actually hit the service URL.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client := &http.Client{
		Timeout: time.Second,
	}

	if server.Runtime != types.RuntimeContainerized {
		// This server is using nanobot as long as it is not the containerized runtime,
		// so we can reach out to nanobot's healthz path.
		url = fmt.Sprintf("%s/healthz", url)
		for {
			resp, err := client.Get(url)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					break
				}
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("timed out waiting for MCP server to be ready")
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	if server.ContainerPath != "" {
		// Try making a standard POST call to this MCP server to see if it responds.
		url = fmt.Sprintf("%s/%s", strings.TrimSuffix(url, "/"), strings.TrimPrefix(server.ContainerPath, "/"))
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for containerized MCP server to be ready")
		case <-time.After(100 * time.Millisecond):
		}

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(streamableHTTPHealthcheckBody))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Accept", "application/json,text/event-stream")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}

		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if sessionID := resp.Header.Get("Mcp-Session-Id"); sessionID != "" {
				// Send a cancellation, since we don't need this session.
				// If we get any errors, ignore them, because it doesn't matter for us.
				req, err := http.NewRequest(http.MethodDelete, url, nil)
				if err == nil {
					req.Header.Set("Mcp-Session-Id", sessionID)
					_, _ = http.DefaultClient.Do(req)
				}
			}
			return nil
		}

		// Fallback to trying SSE.
		req, err = http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Accept", "text/event-stream")

		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			// Start looking for an event with "endpoint".
			scanner := bufio.NewScanner(resp.Body)
		scannerLoop:
			for scanner.Scan() {
				select {
				case <-ctx.Done():
					break scannerLoop
				default:
					if strings.Contains(scanner.Text(), "endpoint") {
						resp.Body.Close()
						cancel()
						return nil
					}
				}
			}
			resp.Body.Close()
			cancel()
		}
	}
}
