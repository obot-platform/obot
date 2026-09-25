package registry

import (
	"context"
	"fmt"
	"strings"
	"time"

	obottypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ConvertMCPServerToRegistry converts an Obot MCPServer to a Registry ServerResponse
// Uses the existing ConvertMCPServer function to ensure consistency with the rest of the codebase
func ConvertMCPServerToRegistry(
	ctx context.Context,
	storage kclient.Reader,
	server v1.MCPServer,
	credEnv map[string]string,
	serverURL string,
	slug string,
	reverseDNS string,
	userID string,
	mimeFetcher *mimeFetcher,
) (obottypes.RegistryServerResponse, error) {
	localhostCallback, err := serverRequiresLocalhostCallback(ctx, storage, server)
	if err != nil {
		return obottypes.RegistryServerResponse{}, err
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

	// Determine if server should show connection URL
	isPersonalServer := convertedServer.UserID == userID && convertedServer.IsSingleUser()
	isMultiUserServer := !convertedServer.IsSingleUser()

	// Advertise configured servers through HTTP unless OAuth requires the CLI relay.
	if localhostCallback {
		meta.Obot = &obottypes.RegistryObotMeta{
			ConfigurationRequired: true,
			ConfigurationMessage:  "This server requires the Obot CLI. Please visit the Obot UI for connection instructions.",
		}
	} else if isPersonalServer && convertedServer.Configured && !convertedServer.NeedsURL && convertedServer.ConnectURL != "" {
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

		serverDetail.Meta.PublisherProvided.GitHub.Readme = fmt.Sprintf("> Note: This server requires configuration and cannot be installed directly from your client. Please visit [Obot](%s) to to configure this server and obtain a connection URL.\n\n%s", serverURL, serverDetail.Meta.PublisherProvided.GitHub.Readme)
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

	requiresConfiguration := catalogEntryRequiresConfiguration(entry)

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

		serverDetail.Meta.PublisherProvided.GitHub.Readme = fmt.Sprintf("> Note: This server requires configuration and cannot be installed directly from your client. Please visit [Obot](%s) to to configure this server and obtain a connection URL.\n\n%s", serverURL, serverDetail.Meta.PublisherProvided.GitHub.Readme)
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

func catalogEntryRequiresConfiguration(entry v1.MCPServerCatalogEntry) bool {
	manifest := entry.Spec.Manifest

	for _, env := range manifest.Config {
		// Required env values without a secret binding must be configured
		if env.Required && env.Value == "" && env.SecretBinding == nil {
			return true
		}
	}

	if manifest.Runtime == obottypes.RuntimeRemote && manifest.RemoteConfig != nil {
		// Localhost OAuth requires the Obot CLI callback relay. Registry clients
		// cannot use this entry as a direct HTTP remote.
		if manifest.RemoteConfig.LocalhostCallbackEnabled {
			return true
		}

		if manifest.RemoteConfig.StaticOAuthRequired && !entry.Status.OAuthCredentialConfigured {
			return true
		}

		// Without a fixed URL, the user must supply a connection URL.
		if manifest.RemoteConfig.FixedURL == "" {
			return true
		}
	}

	return false
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

// serverRequiresLocalhostCallback uses the same component snapshots as runtime
// resolution, including snapshots retained by migrated vMCP connections.
func serverRequiresLocalhostCallback(ctx context.Context, storage kclient.Reader, server v1.MCPServer) (bool, error) {
	if remote := server.Spec.Manifest.RemoteConfig; remote != nil && remote.LocalhostCallbackEnabled {
		return true, nil
	}
	if server.Spec.Manifest.Runtime != obottypes.RuntimeVMCP {
		return false, nil
	}
	vmcpID := server.Spec.VMCPID
	var instance v1.VMCPInstance
	if server.Spec.VMCPInstanceID != "" {
		if err := storage.Get(ctx, kclient.ObjectKey{Namespace: server.Namespace, Name: server.Spec.VMCPInstanceID}, &instance); err != nil {
			return false, fmt.Errorf("resolve registry vMCP instance: %w", err)
		}
		vmcpID = instance.Spec.Manifest.VMCPID
	}
	var vmcp v1.VMCP
	if err := storage.Get(ctx, kclient.ObjectKey{Namespace: server.Namespace, Name: vmcpID}, &vmcp); err != nil {
		return false, fmt.Errorf("resolve registry vMCP: %w", err)
	}
	components := vmcp.Spec.Manifest.Components
	if instance.Name != "" {
		components = vmcpconfig.ComponentsForInstance(vmcp, instance)
	}
	for _, component := range components {
		if remote := component.CatalogEntry.Manifest.RemoteConfig; remote != nil && remote.LocalhostCallbackEnabled {
			return true, nil
		}
	}
	return false, nil
}
