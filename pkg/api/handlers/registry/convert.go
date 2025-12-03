package registry

import (
	"context"
	"fmt"
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
	mimeFetcher *mimeFetcher,
) (obottypes.RegistryServerResponse, error) {
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
	serverDetail := obottypes.RegistryServerDetail{
		Name:        registryName,
		Description: convertedServer.MCPServerManifest.Description,
		Title:       displayName,
		Version:     "latest",
		Schema:      "https://static.modelcontextprotocol.io/schemas/2025-09-29/server.schema.json",
		Meta: obottypes.RegistryServerMeta{
			PublisherProvided: &obottypes.RegistryPublisherProvidedMeta{
				GitHub: &obottypes.RegistryGitHubMeta{
					Readme: server.Spec.Manifest.Description,
				},
			},
		},
	}

	// Add icon if present
	if convertedServer.MCPServerManifest.Icon != "" {
		serverDetail.Icons = []obottypes.RegistryServerIcon{
			{
				Src:      convertedServer.MCPServerManifest.Icon,
				MimeType: mimeFetcher.guessMimeType(ctx, convertedServer.MCPServerManifest.Icon),
			},
		}
	}

	// Create metadata
	meta := obottypes.RegistryMeta{
		Official: obottypes.RegistryOfficialMeta{
			IsLatest:  true,
			CreatedAt: server.CreationTimestamp.Format(time.RFC3339),
			Status:    "active",
		},
	}

	// Determine if server should show connection URL
	isPersonalServer := convertedServer.UserID == userID && convertedServer.MCPCatalogID == "" && convertedServer.PowerUserWorkspaceID == ""
	isMultiUserServer := convertedServer.MCPCatalogID != "" || convertedServer.PowerUserWorkspaceID != ""

	// For configured servers, add remote with mcp-connect URL
	// All Obot servers are exposed as streamable-http remotes regardless of underlying runtime
	if isPersonalServer && convertedServer.Configured && !convertedServer.NeedsURL && convertedServer.ConnectURL != "" {
		// This is a personal server that is configured and ready to go.
		serverDetail.Remotes = []obottypes.RegistryServerRemote{
			{
				Type: "streamable-http",
				URL:  convertedServer.ConnectURL,
			},
		}
	} else if isMultiUserServer {
		// Multi-user servers are pre-configured by admins, so they always get a connection URL
		connectURL := fmt.Sprintf("%s/mcp-connect/%s", serverURL, server.Name)
		serverDetail.Remotes = []obottypes.RegistryServerRemote{
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
		Server:        serverDetail,
		Meta:          meta,
		CreatedAtUnix: server.CreationTimestamp.Unix(),
	}, nil
}

// ConvertMCPServerCatalogEntryToRegistry converts a catalog entry to Registry format
func ConvertMCPServerCatalogEntryToRegistry(
	ctx context.Context,
	entry v1.MCPServerCatalogEntry,
	serverURL string,
	reverseDNS string,
	mimeFetcher *mimeFetcher,
) (obottypes.RegistryServerResponse, error) {
	manifest := entry.Spec.Manifest

	// Generate registry server name
	displayName := manifest.Name
	if displayName == "" {
		displayName = entry.Name
	}
	registryName := FormatRegistryServerName(reverseDNS, entry.Name)

	// Create ServerDetail
	serverDetail := obottypes.RegistryServerDetail{
		Name:        registryName,
		Description: manifest.Description,
		Title:       displayName,
		Version:     "latest",
		Schema:      "https://static.modelcontextprotocol.io/schemas/2025-09-29/server.schema.json",
		Meta: obottypes.RegistryServerMeta{
			PublisherProvided: &obottypes.RegistryPublisherProvidedMeta{
				GitHub: &obottypes.RegistryGitHubMeta{
					Readme: entry.Spec.Manifest.Description,
				},
			},
		},
	}

	// Add icon if present
	if manifest.Icon != "" {
		serverDetail.Icons = []obottypes.RegistryServerIcon{
			{
				Src:      manifest.Icon,
				MimeType: mimeFetcher.guessMimeType(ctx, manifest.Icon),
			},
		}
	}

	// Add repository if present
	if manifest.RepoURL != "" {
		source := guessRepoSource(manifest.RepoURL)
		if source != "" {
			serverDetail.Repository = &obottypes.RegistryServerRepository{
				URL:    manifest.RepoURL,
				Source: source,
			}
		}
	}

	// Check if the catalog entry requires configuration.
	// Composite servers always require configuration in the UI before they can be used.
	requiresConfiguration := manifest.Runtime == obottypes.RuntimeComposite

	// Check for required environment variables
	if !requiresConfiguration {
		for _, env := range manifest.Env {
			if env.Required {
				requiresConfiguration = true
				break
			}
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
	meta := obottypes.RegistryMeta{
		Official: obottypes.RegistryOfficialMeta{
			IsLatest:  true,
			CreatedAt: entry.CreationTimestamp.Format(time.RFC3339),
			Status:    "active",
		},
	}

	if requiresConfiguration {
		// Requires configuration - show configuration message
		meta.Obot = &obottypes.RegistryObotMeta{
			ConfigurationRequired: true,
			ConfigurationMessage:  "This server needs to be configured before use. Please visit the Obot UI to set it up.",
		}
	} else {
		// No configuration required - provide connection URL
		serverDetail.Remotes = []obottypes.RegistryServerRemote{
			{
				Type: "streamable-http",
				URL:  fmt.Sprintf("%s/mcp-connect/%s", serverURL, entry.Name),
			},
		}
	}

	return obottypes.RegistryServerResponse{
		Server:        serverDetail,
		Meta:          meta,
		CreatedAtUnix: entry.CreationTimestamp.Unix(),
	}, nil
}

// Helper functions

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
