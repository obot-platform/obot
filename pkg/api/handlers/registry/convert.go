package registry

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	obottypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// ConvertMCPServerToRegistry converts an Obot MCPServer to a Registry ServerResponse
// Uses the existing ConvertMCPServer function to ensure consistency with the rest of the codebase
func ConvertMCPServerToRegistry(
	ctx context.Context,
	server v1.MCPServer,
	credEnv map[string]string,
	serverURL string,
	slug string,
	reverseDNS string,
	userID string,
) (obottypes.RegistryServerResponse, error) {
	// Skip composite servers
	if server.Spec.Manifest.Runtime == obottypes.RuntimeComposite {
		return obottypes.RegistryServerResponse{}, fmt.Errorf("composite servers are not supported")
	}

	// Use existing conversion function to get types.MCPServer
	convertedServer := handlers.ConvertMCPServer(server, credEnv, serverURL, slug)

	// Generate registry server name
	displayName := convertedServer.MCPServerManifest.Name
	if displayName == "" {
		displayName = convertedServer.ID
	}

	if server.Spec.Alias != "" {
		displayName = server.Spec.Alias
	}

	registryName := FormatRegistryServerName(reverseDNS, slug)

	// Create ServerDetail
	serverDetail := obottypes.ServerDetail{
		Name:        registryName,
		Description: convertedServer.MCPServerManifest.Description,
		Title:       displayName,
		Version:     "latest",
	}

	// Add icon if present
	if convertedServer.MCPServerManifest.Icon != "" {
		serverDetail.Icons = []obottypes.RegistryIcon{
			{
				Src:      convertedServer.MCPServerManifest.Icon,
				MimeType: guessMimeType(ctx, convertedServer.MCPServerManifest.Icon),
			},
		}
	}

	// Create metadata
	meta := obottypes.RegistryMeta{}

	// Determine if server should show connection URL
	isPersonalServer := convertedServer.UserID == userID && convertedServer.MCPCatalogID == "" && convertedServer.PowerUserWorkspaceID == ""
	isMultiUserServer := convertedServer.MCPCatalogID != "" || convertedServer.PowerUserWorkspaceID != ""

	// For configured servers, add remote with mcp-connect URL
	// All Obot servers are exposed as streamable-http remotes regardless of underlying runtime
	if isPersonalServer && convertedServer.Configured && !convertedServer.NeedsURL && convertedServer.ConnectURL != "" {
		// This is a personal server that is configured and ready to go.
		serverDetail.Remotes = []obottypes.RegistryRemote{
			{
				Type: "streamable-http",
				URL:  convertedServer.ConnectURL,
			},
		}
	} else if isMultiUserServer {
		// Multi-user servers are pre-configured by admins, so they always get a connection URL
		connectURL := fmt.Sprintf("%s/mcp-connect/%s", serverURL, server.Name)
		serverDetail.Remotes = []obottypes.RegistryRemote{
			{
				Type: "streamable-http",
				URL:  connectURL,
			},
		}
	} else {
		// Personal server that is not configured
		meta.Obot = &obottypes.RegistryObotMeta{
			ConfigurationRequired: true,
			ConfigurationMessage:  "This server requires configuration. Please visit the Obot UI to configure it.",
		}
	}

	return obottypes.RegistryServerResponse{
		Server: serverDetail,
		Meta:   meta,
	}, nil
}

// ConvertMCPServerCatalogEntryToRegistry converts a catalog entry to Registry format
func ConvertMCPServerCatalogEntryToRegistry(
	ctx context.Context,
	entry v1.MCPServerCatalogEntry,
	serverURL string,
	reverseDNS string,
) (obottypes.RegistryServerResponse, error) {
	manifest := entry.Spec.Manifest

	// Skip composite catalog entries
	if manifest.Runtime == obottypes.RuntimeComposite {
		return obottypes.RegistryServerResponse{}, fmt.Errorf("composite catalog entries are not supported")
	}

	// Generate registry server name
	displayName := manifest.Name
	if displayName == "" {
		displayName = entry.Name
	}
	registryName := FormatRegistryServerName(reverseDNS, entry.Name)

	// Create ServerDetail
	serverDetail := obottypes.ServerDetail{
		Name:        registryName,
		Description: manifest.Description,
		Title:       displayName,
		Version:     "latest",
	}

	// Add icon if present
	if manifest.Icon != "" {
		serverDetail.Icons = []obottypes.RegistryIcon{
			{
				Src:      manifest.Icon,
				MimeType: guessMimeType(ctx, manifest.Icon),
			},
		}
	}

	// Add repository if present
	if manifest.RepoURL != "" {
		source := guessRepoSource(manifest.RepoURL)
		if source != "" {
			serverDetail.Repository = &obottypes.RegistryRepository{
				URL:    manifest.RepoURL,
				Source: source,
			}
		}
	}

	// Check if the catalog entry requires configuration
	var requiresConfiguration bool

	// Check for required environment variables
	for _, env := range manifest.Env {
		if env.Required {
			requiresConfiguration = true
			break
		}
	}

	// Check for required headers (for remote runtime)
	if !requiresConfiguration && manifest.Runtime == obottypes.RuntimeRemote && manifest.RemoteConfig != nil {
		for _, header := range manifest.RemoteConfig.Headers {
			if header.Required {
				requiresConfiguration = true
				break
			}
		}
	}

	// Create metadata
	meta := obottypes.RegistryMeta{}

	if requiresConfiguration {
		// Requires configuration - show configuration message
		meta.Obot = &obottypes.RegistryObotMeta{
			ConfigurationRequired: true,
			ConfigurationMessage:  "This server needs to be configured before use. Please visit the Obot UI to set it up.",
		}
	} else {
		// No configuration required - provide connection URL
		serverDetail.Remotes = []obottypes.RegistryRemote{
			{
				Type: "streamable-http",
				URL:  fmt.Sprintf("%s/mcp-connect/%s", serverURL, entry.Name),
			},
		}
	}

	return obottypes.RegistryServerResponse{
		Server: serverDetail,
		Meta:   meta,
	}, nil
}

// Helper functions

func guessMimeType(ctx context.Context, iconURL string) string {
	// First, try to guess from the file extension
	lower := strings.ToLower(iconURL)
	if strings.HasSuffix(lower, ".png") {
		return "image/png"
	}
	if strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") {
		return "image/jpeg"
	}
	if strings.HasSuffix(lower, ".svg") {
		return "image/svg+xml"
	}
	if strings.HasSuffix(lower, ".webp") {
		return "image/webp"
	}

	// If we couldn't guess from the extension, try to fetch the URL
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return fetchAndDetectMimeType(ctx, iconURL)
	}

	return ""
}

func fetchAndDetectMimeType(ctx context.Context, url string) string {
	// Create a context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ""
	}

	// Perform the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	// First, check the Content-Type header
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		// Extract just the MIME type (before any semicolon/parameters)
		if idx := strings.Index(contentType, ";"); idx > 0 {
			contentType = contentType[:idx]
		}
		contentType = strings.TrimSpace(contentType)

		// Validate it's an image MIME type
		if strings.HasPrefix(contentType, "image/") {
			return contentType
		}
	}

	// If header wasn't useful, read first 512 bytes to detect content type
	buffer := make([]byte, 512)
	n, err := resp.Body.Read(buffer)
	if err != nil && n == 0 {
		return ""
	}

	// Detect content type from the actual data
	detectedType := http.DetectContentType(buffer[:n])

	// Only return if it's an image type
	if strings.HasPrefix(detectedType, "image/") {
		return detectedType
	}

	return ""
}

func guessRepoSource(repoURL string) string {
	lower := strings.ToLower(repoURL)
	if strings.Contains(lower, "github.com") {
		return "github"
	}
	if strings.Contains(lower, "gitlab.com") {
		return "gitlab"
	}
	if strings.Contains(lower, "bitbucket.org") {
		return "bitbucket"
	}
	return ""
}
