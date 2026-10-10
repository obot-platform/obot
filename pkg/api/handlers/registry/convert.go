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
	mimeFetcher *mimeFetcher,
) (obottypes.RegistryServerResponse, error) {
	// Use existing conversion function to get types.MCPServer
	convertedServer := handlers.ConvertMCPServer(server, credEnv)

	// Generate registry server name
	displayName := convertedServer.MCPServerManifest.Name
	if displayName == "" {
		displayName = convertedServer.ID
	}

	if server.Spec.Alias != "" {
		displayName = server.Spec.Alias
	}

	registryName := FormatRegistryServerName(reverseDNS, slug)

	serverDetail := obottypes.RegistryServerDetail{
		Name:        registryName,
		Description: convertedServer.MCPServerManifest.ShortDescription,
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

	markVMCPConnectionRequired(&serverDetail, &meta, serverURL)

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

	serverDetail := obottypes.RegistryServerDetail{
		Name:        registryName,
		Description: manifest.ShortDescription,
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

	// Create metadata
	meta := obottypes.RegistryMeta{
		Official: obottypes.RegistryOfficialMeta{
			IsLatest:  true,
			CreatedAt: entry.CreationTimestamp.Format(time.RFC3339),
			Status:    "active",
		},
	}

	markVMCPConnectionRequired(&serverDetail, &meta, serverURL)

	return obottypes.RegistryServerResponse{
		Server:        serverDetail,
		Meta:          meta,
		CreatedAtUnix: entry.CreationTimestamp.Unix(),
	}, nil
}

// Helper functions

func markVMCPConnectionRequired(detail *obottypes.RegistryServerDetail, meta *obottypes.RegistryMeta, serverURL string) {
	meta.Obot = &obottypes.RegistryObotMeta{
		ConfigurationRequired: true,
		ConfigurationMessage:  "Add this server to a vMCP in Obot to connect.",
	}
	detail.Meta.PublisherProvided.GitHub.Readme = fmt.Sprintf("> Note: Add this server to a vMCP in [Obot](%s) to connect.\n\n%s", serverURL, detail.Meta.PublisherProvided.GitHub.Readme)
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
