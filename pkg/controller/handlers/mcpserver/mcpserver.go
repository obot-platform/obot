package mcpserver

import (
	"crypto/rand"
	"fmt"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"golang.org/x/crypto/bcrypt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gptClient         *gptscript.GPTScript
	mcpSessionManager *mcp.SessionManager
	baseURL           string
}

func New(gptClient *gptscript.GPTScript, mcpSessionManager *mcp.SessionManager, baseURL string) *Handler {
	return &Handler{
		gptClient:         gptClient,
		mcpSessionManager: mcpSessionManager,
		baseURL:           baseURL,
	}
}

func (h *Handler) DetectDrift(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.MCPServerCatalogEntryName == "" || server.Spec.CompositeName != "" {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, server.Namespace, server.Spec.MCPServerCatalogEntryName); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	drifted, err := configurationHasDrifted(server.Spec.Manifest, entry.Spec.Manifest)
	if err != nil {
		return err
	}

	if server.Status.NeedsUpdate != drifted {
		server.Status.NeedsUpdate = drifted
		return req.Client.Status().Update(req.Ctx, server)
	}
	return nil
}

// DetectK8sSettingsDrift detects when a server needs redeployment with new K8s settings
// Note: This handler only sets NeedsK8sUpdate based on K8sSettings hash drift.
// PSA compliance checking is handled separately in the deployment handler since it
// requires access to the actual Deployment object to inspect container security contexts.
func (h *Handler) DetectK8sSettingsDrift(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	// Skip if server doesn't have K8s settings hash (not yet deployed)
	if server.Status.K8sSettingsHash == "" {
		return nil
	}

	// Get current K8s settings
	var k8sSettings v1.K8sSettings
	if err := req.Get(&k8sSettings, server.Namespace, system.K8sSettingsName); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get K8s settings: %w", err)
	}

	// Compute current K8s settings hash
	currentHash := mcp.ComputeK8sSettingsHash(k8sSettings.Spec)

	if server.Status.K8sSettingsHash != currentHash && !server.Status.NeedsK8sUpdate {
		server.Status.NeedsK8sUpdate = true
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

func configurationHasDrifted(serverManifest types.MCPServerManifest, entryManifest types.MCPServerCatalogEntryManifest) (bool, error) {
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
		drifted = remoteConfigHasDrifted(serverManifest.RemoteConfig, entryManifest.RemoteConfig)
	case types.RuntimeComposite:
		var err error
		drifted, err = compositeConfigHasDrifted(serverManifest.CompositeConfig, entryManifest.CompositeConfig)
		if err != nil {
			return false, err
		}
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
func remoteConfigHasDrifted(serverConfig *types.RemoteRuntimeConfig, entryConfig *types.RemoteCatalogConfig) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}

	if entryConfig.Hostname != serverConfig.Hostname ||
		entryConfig.URLTemplate != serverConfig.URLTemplate {
		return true
	}

	// For remote runtime, we need to check if the server URL matches what the catalog entry expects
	if entryConfig.FixedURL != "" {
		// If catalog entry has a fixed URL, server URL should match exactly
		if serverConfig.URL != entryConfig.FixedURL {
			return true
		}
	}

	// Check if headers have drifted
	return !utils.SlicesEqualIgnoreOrder(serverConfig.Headers, entryConfig.Headers)
}

// compositeConfigHasDrifted checks if the composite configuration has drifted
func compositeConfigHasDrifted(serverConfig *types.CompositeRuntimeConfig, entryConfig *types.CompositeCatalogConfig) (bool, error) {
	if serverConfig == nil && entryConfig == nil {
		return false, nil
	}
	if serverConfig == nil || entryConfig == nil {
		return true, nil
	}

	// Fast length check
	if len(serverConfig.ComponentServers) != len(entryConfig.ComponentServers) {
		return true, nil
	}

	entryComponents := make(map[string]types.CatalogComponentServer, len(entryConfig.ComponentServers))
	for _, entryComponent := range entryConfig.ComponentServers {
		if id := entryComponent.ComponentID(); id != "" {
			entryComponents[id] = entryComponent
		}
	}

	for _, serverComponent := range serverConfig.ComponentServers {
		entryComponent, ok := entryComponents[serverComponent.ComponentID()]
		if !ok {
			return true, nil
		}

		// Compare tool overrides
		if hash.Digest(serverComponent.ToolOverrides) != hash.Digest(entryComponent.ToolOverrides) {
			return true, nil
		}

		// Compare manifests
		drifted, err := configurationHasDrifted(serverComponent.Manifest, entryComponent.Manifest)
		if err != nil || drifted {
			return drifted, err
		}
	}

	return false, nil
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
		if userID := instance.Spec.UserID; userID != "" && instance.DeletionTimestamp.IsZero() {
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

func (h *Handler) DeleteServersForAnonymousUser(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if server.Spec.UserID == "anonymous" {
		return req.Client.Delete(req.Ctx, server)
	}

	return nil
}

func (h *Handler) EnsureMCPCatalogID(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if (server.Status.MCPCatalogID == "" || server.Status.MCPCatalogID == server.Spec.MCPServerCatalogEntryName) && server.Spec.MCPCatalogID == "" && server.Spec.MCPServerCatalogEntryName != "" {
		var mcpCatalogEntry v1.MCPServerCatalogEntry
		if err := req.Get(&mcpCatalogEntry, server.Namespace, server.Spec.MCPServerCatalogEntryName); err != nil {
			// Don't return an error here if the entry isn't found.
			// This will prevent the MCPServer from being requeued repeatedly when the catalog entry doesn't exist.
			return kclient.IgnoreNotFound(err)
		}

		server.Status.MCPCatalogID = mcpCatalogEntry.Spec.MCPCatalogName
		return req.Client.Status().Update(req.Ctx, server)
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

func (h *Handler) EnsureMCPServerSecretInfo(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	fieldSelector := fields.SelectorFromSet(map[string]string{
		"spec.mcpServerName": server.Name,
	})
	var oauthClients v1.OAuthClientList
	if err := req.List(&oauthClients, &kclient.ListOptions{
		Namespace:     req.Namespace,
		FieldSelector: fieldSelector,
	}); err != nil {
		return err
	}

	if len(oauthClients.Items) == 0 {
		// If listing with the cache doesn't return anything, double-check with the uncached listing
		if err := req.List(untriggered.UncachedList(&oauthClients), &kclient.ListOptions{
			Namespace:     req.Namespace,
			FieldSelector: fieldSelector,
		}); err != nil {
			return err
		}
	}

	if server.Status.AuditLogTokenHash != "" {
		cred, err := h.gptClient.RevealCredential(req.Ctx, []string{server.Name}, server.Name)
		if err != nil {
			return fmt.Errorf("failed to get credential: %w", err)
		}

		if server.Status.AuditLogTokenHash != hash.Digest(cred.Env["AUDIT_LOG_TOKEN"]) {
			// Reset the audit log token hash to reset the credential.
			server.Status.AuditLogTokenHash = ""
		}
	}

	if len(oauthClients.Items) > 0 && (server.Status.AuditLogTokenHash != "" || server.Spec.CompositeName != "") {
		// Nothing else to do here.
		return nil
	}

	clientID := system.OAuthClientPrefix + strings.ToLower(rand.Text())
	clientSecret := strings.ToLower(rand.Text() + rand.Text())
	hashedClientSecretHash, err := bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash client secret: %w", err)
	}

	auditLogToken := strings.ToLower(rand.Text() + rand.Text())

	if err := h.gptClient.CreateCredential(req.Ctx, gptscript.Credential{
		Context:  server.Name,
		ToolName: server.Name,
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			"TOKEN_EXCHANGE_CLIENT_ID":     fmt.Sprintf("%s:%s", req.Namespace, clientID),
			"TOKEN_EXCHANGE_CLIENT_SECRET": clientSecret,
			"AUDIT_LOG_TOKEN":              auditLogToken,
		},
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	oauthClient := v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:       clientID,
			Namespace:  req.Namespace,
			Finalizers: []string{v1.OAuthClientFinalizer},
		},
		Spec: v1.OAuthClientSpec{
			Manifest: types.OAuthClientManifest{
				GrantTypes: []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
			},
			ClientSecretHash: hashedClientSecretHash,
			MCPServerName:    server.Name,
		},
	}

	if err := req.Client.Create(req.Ctx, &oauthClient); err != nil {
		return fmt.Errorf("failed to create OAuth client: %w", err)
	}

	server.Status.AuditLogTokenHash = hash.Digest(auditLogToken)

	return nil
}

// CleanupNestedCompositeServers removes component servers with composite runtimes from composite MCP servers.
// This handler cleans up servers that were created before API validation to prevent nested composite servers.
func (h *Handler) CleanupNestedCompositeServers(req router.Request, _ router.Response) error {
	var (
		server   = req.Object.(*v1.MCPServer)
		manifest = server.Spec.Manifest
	)

	if manifest.Runtime != types.RuntimeComposite ||
		manifest.CompositeConfig == nil {
		return nil
	}

	// Delete component servers with composite runtimes
	if server.Spec.CompositeName != "" {
		return kclient.IgnoreNotFound(req.Client.Delete(req.Ctx, server))
	}
	// Remove all composite components from the server's manifest
	var (
		components    = manifest.CompositeConfig.ComponentServers
		numComponents = len(components)
	)
	components = slices.DeleteFunc(components, func(component types.ComponentServer) bool {
		return component.Manifest.Runtime == types.RuntimeComposite
	})

	if numComponents == len(components) {
		return nil
	}

	server.Spec.Manifest.CompositeConfig.ComponentServers = components
	return kclient.IgnoreNotFound(req.Client.Update(req.Ctx, server))
}

func (h *Handler) EnsureCompositeComponents(req router.Request, _ router.Response) error {
	var (
		compositeServer = req.Object.(*v1.MCPServer)
		manifest        = compositeServer.Spec.Manifest
	)

	if manifest.Runtime != types.RuntimeComposite ||
		manifest.CompositeConfig == nil ||
		len(manifest.CompositeConfig.ComponentServers) < 1 {
		return nil
	}

	// Load all existing component servers
	var componentServers v1.MCPServerList
	if err := req.List(&componentServers, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.compositeName", compositeServer.Name),
		Namespace:     compositeServer.Namespace,
	}); err != nil {
		return fmt.Errorf("failed to list component servers: %w", err)
	}

	// Load all existing component instances (for multi-user components)
	var componentInstances v1.MCPServerInstanceList
	if err := req.List(&componentInstances, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.compositeName", compositeServer.Name),
		Namespace:     compositeServer.Namespace,
	}); err != nil {
		return fmt.Errorf("failed to list component instances: %w", err)
	}

	// Create index of existing catalog entry components by ID
	existingServers := make(map[string]v1.MCPServer, len(componentServers.Items))
	for _, existing := range componentServers.Items {
		if id := existing.Spec.MCPServerCatalogEntryName; id != "" {
			existingServers[id] = existing
		}
	}

	// Create index of existing multi-user component instances by MCPServerID
	existingInstances := make(map[string]v1.MCPServerInstance, len(componentInstances.Items))
	for _, existing := range componentInstances.Items {
		if id := existing.Spec.MCPServerName; id != "" {
			existingInstances[existing.Spec.MCPServerName] = existing
		}
	}

	// withNeedsURL returns the given MCP server with a NeedsURL field set according to its hostname constraint and url.
	// If the server is not remote, or does not have a hostname constraint, it returns the unmodified server.
	withNeedsURL := func(server v1.MCPServer) v1.MCPServer {
		remoteConfig := compositeServer.Spec.Manifest.RemoteConfig
		if compositeServer.Spec.Manifest.Runtime != types.RuntimeRemote || remoteConfig == nil || remoteConfig.Hostname == "" {
			return server
		}

		server.Spec.NeedsURL = types.ValidateURLHostname(remoteConfig.URL, remoteConfig.Hostname) != nil
		return server
	}

	// Ensuring a composite server is up-to-date has 3 steps:
	// 1. Create new component servers and instances
	// 2. Update existing component servers (no-op on existing instances, since there's nothing to change)
	// 3. Delete removed component servers and instances
	for _, component := range manifest.CompositeConfig.ComponentServers {
		if component.MCPServerID != "" {
			// Multi-user component
			if _, exists := existingInstances[component.MCPServerID]; !exists {
				// New instance, create it
				var multiUserServer v1.MCPServer
				if err := req.Get(&multiUserServer, compositeServer.Namespace, component.MCPServerID); err != nil {
					return fmt.Errorf("failed to get multi-user server %s: %w", component.MCPServerID, err)
				}

				if err := req.Client.Create(req.Ctx, &v1.MCPServerInstance{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: system.MCPServerInstancePrefix,
						Namespace:    compositeServer.Namespace,
						Finalizers:   []string{v1.MCPServerInstanceFinalizer},
					},
					Spec: v1.MCPServerInstanceSpec{
						MCPServerName:        component.MCPServerID,
						MCPCatalogName:       multiUserServer.Spec.MCPCatalogID,
						PowerUserWorkspaceID: multiUserServer.Spec.PowerUserWorkspaceID,
						UserID:               compositeServer.Spec.UserID,
						CompositeName:        compositeServer.Name,
					},
				}); err != nil {
					return fmt.Errorf("failed to create instance for multi-user component: %w", err)
				}
			}

			// Remove the instance to build the list of existing instances to delete
			delete(existingInstances, component.MCPServerID)
			continue
		}

		// Catalog entry component
		if existingServer, exists := existingServers[component.CatalogEntryID]; !exists {
			// New server, create it
			newServer := withNeedsURL(v1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: system.MCPServerPrefix,
					Namespace:    compositeServer.Namespace,
					Finalizers:   []string{v1.MCPServerFinalizer},
				},
				Spec: v1.MCPServerSpec{
					Manifest:                  component.Manifest,
					MCPServerCatalogEntryName: component.CatalogEntryID,
					UserID:                    compositeServer.Spec.UserID,
					CompositeName:             compositeServer.Name,
				},
			})

			if err := req.Client.Create(req.Ctx, &newServer); err != nil {
				return fmt.Errorf("failed to create new component server: %w", err)
			}
		} else if hash.Digest(existingServer.Spec.Manifest) != hash.Digest(component.Manifest) {
			// Ensure the server is shut down before updating it
			if err := h.mcpSessionManager.ShutdownServer(req.Ctx, existingServer.Name); err != nil {
				return err
			}

			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				var latestServer v1.MCPServer
				if err := req.Get(&latestServer, compositeServer.Namespace, existingServer.Name); err != nil {
					return err
				}

				latestServer.Spec.Manifest = component.Manifest
				latestServer = withNeedsURL(latestServer)
				return req.Client.Update(req.Ctx, &latestServer)
			}); err != nil {
				return fmt.Errorf("failed to update existing component server: %w", err)
			}
		}

		// Remove the server to build the list of existing servers to delete
		delete(existingServers, component.CatalogEntryID)
	}

	// Delete existing instances that were not in the updated manifest
	for _, instance := range existingInstances {
		if err := req.Delete(&instance); kclient.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete instance %s: %w", instance.Name, err)
		}
	}

	// Delete existing servers that were not in the updated manifest
	for _, server := range existingServers {
		if err := req.Delete(&server); kclient.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete server %s: %w", server.Name, err)
		}
	}

	// All of the component MCP servers should now match the manifest of the composite.
	// Update the status hash to reflect the observed state.
	if manifestHash := hash.Digest(manifest); compositeServer.Status.ObservedCompositeManifestHash != manifestHash {
		compositeServer.Status.ObservedCompositeManifestHash = manifestHash
		if err := req.Client.Status().Update(req.Ctx, compositeServer); err != nil {
			return fmt.Errorf("failed to update composite server status: %w", err)
		}
	}

	return nil
}

// SyncOAuthCredentialStatus syncs the OAuthCredentialConfigured status from the catalog entry.
// This replaces the push-based propagation logic with a pull-based approach where each MCP server
// is responsible for syncing its own status from its parent catalog entry.
func (h *Handler) SyncOAuthCredentialStatus(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	// Only relevant for servers created from catalog entries
	if server.Spec.MCPServerCatalogEntryName == "" {
		return clearOAuthStatusIfSet(req, server)
	}

	// Look up the catalog entry
	var catalogEntry v1.MCPServerCatalogEntry
	if err := req.Get(&catalogEntry, server.Namespace, server.Spec.MCPServerCatalogEntryName); err != nil {
		if apierrors.IsNotFound(err) {
			// Catalog entry deleted, this server itself will soon be cleaned up
			return nil
		}
		return fmt.Errorf("failed to get catalog entry: %w", err)
	}

	// Check if catalog entry requires static OAuth
	requiresStaticOAuth := catalogEntry.Spec.Manifest.Runtime == types.RuntimeRemote &&
		catalogEntry.Spec.Manifest.RemoteConfig != nil &&
		catalogEntry.Spec.Manifest.RemoteConfig.StaticOAuthRequired

	if !requiresStaticOAuth {
		return clearOAuthStatusIfSet(req, server)
	}

	// Sync status from catalog entry
	if server.Status.OAuthCredentialConfigured != catalogEntry.Status.OAuthCredentialConfigured {
		server.Status.OAuthCredentialConfigured = catalogEntry.Status.OAuthCredentialConfigured
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

// clearOAuthStatusIfSet clears the OAuthCredentialConfigured status if it is currently set.
func clearOAuthStatusIfSet(req router.Request, server *v1.MCPServer) error {
	if server.Status.OAuthCredentialConfigured {
		server.Status.OAuthCredentialConfigured = false
		return req.Client.Status().Update(req.Ctx, server)
	}
	return nil
}
