package mcpserver

import (
	"fmt"
	"slices"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	baseURL string
}

func New(baseURL string) *Handler {
	return &Handler{
		baseURL: baseURL,
	}
}

// TODO(cmcp): Implement composite server drift detection

func (h *Handler) DetectDrift(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.MCPServerCatalogEntryName == "" {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, server.Namespace, server.Spec.MCPServerCatalogEntryName); err != nil {
		return err
	}

	var entryManifest types.MCPServerCatalogEntryManifest
	if compositeName := server.Spec.CompositeName; compositeName != "" {
		// The server belongs to a composite server, so we should get the entry from the runtime of the composite entry that this server was created with.
		var compositeServer v1.MCPServer
		if err := req.Get(&compositeServer, server.Namespace, compositeName); err != nil {
			return fmt.Errorf("failed to get composite server %s: %w", compositeName, err)
		}

		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, compositeServer.Namespace, compositeServer.Spec.MCPServerCatalogEntryName); err != nil {
			return fmt.Errorf("failed to get composite server catalog entry %s: %w", compositeServer.Spec.MCPServerCatalogEntryName, err)
		}

		var found bool
		for _, component := range entry.Spec.Manifest.CompositeConfig.ComponentServers {
			if component.CatalogEntryID == server.Spec.MCPServerCatalogEntryName {
				entryManifest = component.Manifest
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("component server %s not found in composite server catalog entry %s", server.Spec.MCPServerCatalogEntryName, compositeServer.Spec.MCPServerCatalogEntryName)
		}
	} else {
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, server.Namespace, server.Spec.MCPServerCatalogEntryName); err != nil {
			return err
		}
		entryManifest = entry.Spec.Manifest
	}

	drifted, err := configurationHasDrifted(server.Spec.NeedsURL, server.Spec.Manifest, entryManifest)
	if err != nil {
		return err
	}

	if server.Status.NeedsUpdate != drifted {
		server.Status.NeedsUpdate = drifted
		return req.Client.Status().Update(req.Ctx, server)
	}
	return nil
}

func configurationHasDrifted(needsURL bool, serverManifest types.MCPServerManifest, entryManifest types.MCPServerCatalogEntryManifest) (bool, error) {
	// Check if runtime types differ
	if serverManifest.Runtime != entryManifest.Runtime {
		return true, nil
	}

	// Check runtime-specific configurations
	var drifted bool
	switch serverManifest.Runtime {
	case types.RuntimeUVX:
		drifted = uvxConfigHasDrifted(serverManifest.UVXConfig, entryManifest.UVXConfig)
	case types.RuntimeNPX:
		drifted = npxConfigHasDrifted(serverManifest.NPXConfig, entryManifest.NPXConfig)
	case types.RuntimeContainerized:
		drifted = containerizedConfigHasDrifted(serverManifest.ContainerizedConfig, entryManifest.ContainerizedConfig)
	case types.RuntimeRemote:
		drifted = remoteConfigHasDrifted(needsURL, serverManifest.RemoteConfig, entryManifest.RemoteConfig)
	case types.RuntimeComposite:
		drifted = compositeConfigHasDrifted(serverManifest.CompositeConfig, entryManifest.CompositeConfig)
	default:
		return false, fmt.Errorf("unknown runtime type: %s", serverManifest.Runtime)
	}

	if drifted {
		return true, nil
	}

	// Check environment
	return !utils.SlicesEqualIgnoreOrder(serverManifest.Env, entryManifest.Env), nil
}

// uvxConfigHasDrifted checks if UVX configuration has drifted
func uvxConfigHasDrifted(serverConfig, entryConfig *types.UVXRuntimeConfig) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}

	return serverConfig.Package != entryConfig.Package ||
		serverConfig.Command != entryConfig.Command ||
		!slices.Equal(serverConfig.Args, entryConfig.Args)
}

// npxConfigHasDrifted checks if NPX configuration has drifted
func npxConfigHasDrifted(serverConfig, entryConfig *types.NPXRuntimeConfig) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}

	return serverConfig.Package != entryConfig.Package ||
		!slices.Equal(serverConfig.Args, entryConfig.Args)
}

// containerizedConfigHasDrifted checks if containerized configuration has drifted
func containerizedConfigHasDrifted(serverConfig, entryConfig *types.ContainerizedRuntimeConfig) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}

	return serverConfig.Image != entryConfig.Image ||
		serverConfig.Command != entryConfig.Command ||
		serverConfig.Port != entryConfig.Port ||
		serverConfig.Path != entryConfig.Path ||
		!slices.Equal(serverConfig.Args, entryConfig.Args)
}

// remoteConfigHasDrifted checks if remote configuration has drifted
func remoteConfigHasDrifted(needsURL bool, serverConfig *types.RemoteRuntimeConfig, entryConfig *types.RemoteCatalogConfig) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}

	// For remote runtime, we need to check if the server URL matches what the catalog entry expects
	if entryConfig.FixedURL != "" {
		// If catalog entry has a fixed URL, server URL should match exactly
		if serverConfig.URL != entryConfig.FixedURL {
			return true
		}
	}

	// We skip the hostname check if needsURL is already set to true.
	// NeedsURL is true if the admin already triggered an update for this server, and the user has not yet fixed the URL to make it match the hostname.
	// If NeedsURL is false, then we can check the hostname, and if it doesn't match, that means that admin does have an update available to trigger.
	if entryConfig.Hostname != "" && !needsURL {
		// If catalog entry has a hostname constraint, check if server URL uses that hostname
		if err := types.ValidateURLHostname(serverConfig.URL, entryConfig.Hostname); err != nil {
			// Hostname failed to validate, so we consider it drifted
			return true
		}
	}

	// Check if headers have drifted
	return !utils.SlicesEqualIgnoreOrder(serverConfig.Headers, entryConfig.Headers)
}

func compositeConfigHasDrifted(serverConfig *types.CompositeRuntimeConfig, entryConfig *types.CompositeCatalogConfig) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}

	// Fast length check
	if len(serverConfig.ComponentServers) != len(entryConfig.ComponentServers) {
		return true
	}

	// Index entry components by catalogEntryID for quick lookup
	entryByID := make(map[string]types.CatalogComponentServer, len(entryConfig.ComponentServers))
	for _, entryComponent := range entryConfig.ComponentServers {
		entryByID[entryComponent.CatalogEntryID] = entryComponent
	}

	for _, serverComponent := range serverConfig.ComponentServers {
		entryComponent, ok := entryByID[serverComponent.CatalogEntryID]
		if !ok {
			// Component not present in catalog entry
			return true
		}

		// Compare overrides (order-insensitive)
		if hash.Digest(serverComponent.ToolOverrides) != hash.Digest(entryComponent.ToolOverrides) {
			return true
		}

		// Compare manifests for non-remote components
		switch serverComponent.Manifest.Runtime {
		case types.RuntimeRemote:
			// Skip remote manifest comparison in composites
		default:
			drifted, err := configurationHasDrifted(false, serverComponent.Manifest, entryComponent.Manifest)
			if err != nil || drifted {
				return true
			}
		}
	}

	return false
}

// EnsureMCPServerInstanceUserCount ensures that mcp server instance user count for multi-user MCP servers is up to date.
func (*Handler) EnsureMCPServerInstanceUserCount(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if server.Spec.MCPCatalogID == "" && server.Spec.PowerUserWorkspaceID == "" {
		// Server is not multi-user, ensure we're not tracking the instance user count
		if server.Status.MCPServerInstanceUserCount == nil {
			return nil
		}

		// Corrupt state, drop the field to fix it
		server.Status.MCPServerInstanceUserCount = nil
		return req.Client.Status().Update(req.Ctx, server)
	}

	// Get the set of unique users with server instances pointing to this MCP server
	var mcpServerInstances v1.MCPServerInstanceList
	if err := req.List(&mcpServerInstances, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.mcpServerName", server.Name),
		Namespace:     system.DefaultNamespace,
	}); err != nil {
		return fmt.Errorf("failed to list MCP server instances: %w", err)
	}

	uniqueUsers := make(map[string]struct{}, len(mcpServerInstances.Items))
	for _, instance := range mcpServerInstances.Items {
		if userID := instance.Spec.UserID; userID != "" {
			uniqueUsers[userID] = struct{}{}
		}
	}

	if oldUserCount, newUserCount := server.Status.MCPServerInstanceUserCount, len(uniqueUsers); oldUserCount == nil || *oldUserCount != newUserCount {
		server.Status.MCPServerInstanceUserCount = &newUserCount
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

func (h *Handler) DeleteServersWithoutRuntime(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if string(server.Spec.Manifest.Runtime) == "" {
		return req.Client.Delete(req.Ctx, server)
	}

	return nil
}

func (h *Handler) MigrateSharedWithinMCPCatalogName(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.SharedWithinMCPCatalogName != "" && server.Spec.MCPCatalogID == "" {
		server.Spec.MCPCatalogID = server.Spec.SharedWithinMCPCatalogName
		server.Spec.SharedWithinMCPCatalogName = ""
		return req.Client.Update(req.Ctx, server)
	}

	return nil
}
