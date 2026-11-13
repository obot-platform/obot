package mcp

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const (
	// ToolPreviewLabel marks the resources of a temporary server launched to generate tool previews.
	ToolPreviewLabel = "obot.ai/mcp-tool-preview"

	// toolPreviewMaxDuration bounds a whole preview: launching the server, which is itself bounded by
	// MaxMCPServerStartupTimeout, and listing its tools.
	toolPreviewMaxDuration = MaxMCPServerStartupTimeout + 2*time.Minute
	// toolPreviewTTL is the age after which a tool preview server is swept. It exceeds
	// toolPreviewMaxDuration, and each preview launches a server of its own (see toolPreviewServerName),
	// so a server's age is its preview's and the sweep never removes one that a preview is still using.
	toolPreviewTTL           = toolPreviewMaxDuration + 3*time.Minute
	toolPreviewSweepInterval = 15 * time.Minute
	toolPreviewSweepTimeout  = 30 * time.Second
)

// toolPreviewServer is a deployed tool preview server, as reported by a backend.
type toolPreviewServer struct {
	Name    string
	Created time.Time
}

// GenerateToolPreviews creates a temporary MCP server from a catalog entry, lists its tools,
// then shuts it down and returns the tool preview data.
func (sm *SessionManager) GenerateToolPreviews(ctx context.Context, tempMCPServer v1.MCPServer, serverConfig ServerConfig) ([]types.MCPServerTool, error) {
	if serverConfig.MCPServerName == "" {
		return nil, fmt.Errorf("tool preview server has no name")
	}
	if serverConfig.Runtime != types.RuntimeRemote {
		name, err := toolPreviewServerName(serverConfig.MCPServerName)
		if err != nil {
			return nil, err
		}
		serverConfig.MCPServerName = name
	}
	// Keep the name apart from serverConfig: LaunchServer returns an empty config when it fails,
	// and cleanup must still know which server to remove.
	serverName := serverConfig.MCPServerName

	ctx, cancel := context.WithTimeout(ctx, toolPreviewMaxDuration)
	defer cancel()

	// Ensure cleanup happens regardless of success or failure
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if cleanupErr := sm.ShutdownServer(ctx, serverName); cleanupErr != nil {
			slog.Error("failed to clean up temporary instance", "name", tempMCPServer.Name, "error", cleanupErr)
		}
	}()

	serverConfig.ToolPreview = true
	launchedConfig, err := sm.LaunchServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	// Use "system" for the user ID to identify non-user MCP servers.
	launchedConfig.UserID = "system"

	// Create MCP client and list tools
	client, err := sm.clientForServerWithOptions(ctx, "default", launchedConfig, ClientOption{
		ClientName:   "Obot Tool Preview",
		TokenStorage: sm.globalTokenStore.ForUserAndMCP(launchedConfig.UserID, cmp.Or(launchedConfig.MCPServerInstanceID, launchedConfig.MCPServerName), launchedConfig.URL),
	})
	if err != nil {
		return nil, err
	}

	tools, err := collectAll(client.Tools(ctx, nil))
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	return ConvertTools(tools, nil)
}

// toolPreviewServerName gives one tool preview launch a server name of its own. The names callers
// pass are digests of the server's configuration, so without this a preview would reuse a server left
// by an earlier preview of the same configuration: the sweep, which ages a server by when it was
// created, would then take it for expired while the new preview uses it. Concurrent previews of one
// configuration also stop sharing a server that the first to finish would remove.
func toolPreviewServerName(name string) (string, error) {
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("failed to generate tool preview server name: %w", err)
	}
	return name + "-" + hex.EncodeToString(suffix), nil
}

// runToolPreviewSweeper removes expired tool preview servers at startup and then every
// toolPreviewSweepInterval until ctx is done. It is the safety net for a preview whose own cleanup did
// not run or did not finish, such as when Obot restarts mid-preview.
func (sm *SessionManager) runToolPreviewSweeper(ctx context.Context) {
	ticker := time.NewTicker(toolPreviewSweepInterval)
	defer ticker.Stop()

	for {
		sm.sweepToolPreviews(ctx, time.Now())

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sweepToolPreviews removes the tool preview servers created more than toolPreviewTTL before now.
func (sm *SessionManager) sweepToolPreviews(ctx context.Context, now time.Time) {
	ctx, cancel := context.WithTimeout(ctx, toolPreviewSweepTimeout)
	defer cancel()

	servers, err := sm.backend.listToolPreviewServers(ctx)
	if err != nil {
		slog.Warn("failed to list tool preview servers to sweep", "error", err)
		return
	}

	for _, server := range servers {
		if now.Sub(server.Created) < toolPreviewTTL {
			continue
		}

		if err := sm.ShutdownServer(ctx, server.Name); err != nil {
			slog.Error("failed to remove expired tool preview server", "name", server.Name, "error", err)
			continue
		}
		slog.Info("removed expired tool preview server", "name", server.Name, "created", server.Created)
	}
}
