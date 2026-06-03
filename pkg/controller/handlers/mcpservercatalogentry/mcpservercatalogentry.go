package mcpservercatalogentry

import (
	"errors"
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpserver"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

// Handler handles operations for MCP server catalog entries
type Handler struct {
	gatewayClient *gclient.Client
}

// NewHandler creates a new Handler with the given gateway client.
func NewHandler(gatewayClient *gclient.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

// EnsureUserCount ensures that the user count for an MCP server catalog entry is up to date.
// For single-user entries, this counts unique users who have an MCPServer created from the entry.
// For multi-user entries, this sums the user count status from each MCPServer created from the entry.
func (*Handler) EnsureUserCount(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)
	userCount, err := userCountForEntry(req, *entry)
	if err != nil {
		return err
	}

	return updateEntryUserCount(req, entry, userCount)
}

func userCountForEntry(req router.Request, entry v1.MCPServerCatalogEntry) (int, error) {
	var mcpServers v1.MCPServerList
	if err := req.List(&mcpServers, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.mcpServerCatalogEntryName", entry.Name),
		Namespace:     system.DefaultNamespace,
	}); err != nil {
		return 0, fmt.Errorf("failed to list MCP servers: %w", err)
	}

	isSingleUser := entry.Spec.Manifest.ServerUserType.IsSingleUser()
	uniqueUsers := make(map[string]struct{}, len(mcpServers.Items))
	userCount := 0
	for _, server := range mcpServers.Items {
		if !server.DeletionTimestamp.IsZero() || server.Spec.CompositeName != "" {
			continue
		}
		if isSingleUser && server.Spec.UserID != "" {
			uniqueUsers[server.Spec.UserID] = struct{}{}
		} else if !isSingleUser {
			if server.Status.MCPServerInstanceUserCount != nil {
				userCount += *server.Status.MCPServerInstanceUserCount
			}
		}
	}
	if isSingleUser {
		userCount = len(uniqueUsers)
	}

	return userCount, nil
}

func updateEntryUserCount(req router.Request, entry *v1.MCPServerCatalogEntry, newUserCount int) error {
	if entry.Status.UserCount != newUserCount {
		log.Infof("Updated MCP catalog entry user count: entry=%s oldCount=%d newCount=%d", entry.Name, entry.Status.UserCount, newUserCount)
		entry.Status.UserCount = newUserCount
		return req.Client.Status().Update(req.Ctx, entry)
	}

	return nil
}

// EnsureServerUserType backfills the serverUserType field to "singleUser" for
// existing catalog entries that were created before the field was introduced.
func (*Handler) EnsureServerUserType(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)
	if entry.Spec.Manifest.ServerUserType != "" {
		return nil
	}
	entry.Spec.Manifest.ServerUserType = types.ServerUserTypeSingleUser
	return kclient.IgnoreNotFound(req.Client.Update(req.Ctx, entry))
}

func (h *Handler) DeleteEntriesWithoutRuntime(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)
	if string(entry.Spec.Manifest.Runtime) == "" {
		log.Infof("Deleting MCP catalog entry with empty runtime: entry=%s", entry.Name)
		return req.Client.Delete(req.Ctx, entry)
	}

	return nil
}

// UpdateManifestHashAndLastUpdated updates the manifest hash and last updated timestamp when configuration changes
func (*Handler) UpdateManifestHashAndLastUpdated(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)
	currentHash := utils.Digest(entry.Spec.Manifest)
	if entry.Status.ManifestHash != currentHash {
		now := metav1.Now()
		entry.Status.ManifestHash = currentHash
		entry.Status.LastUpdated = &now
		log.Infof("Updated MCP catalog entry manifest hash: entry=%s hash=%s", entry.Name, currentHash)
		return req.Client.Status().Update(req.Ctx, entry)
	}

	return nil
}

func (*Handler) UpdateSystemManifestHashAndLastUpdated(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.SystemMCPServerCatalogEntry)
	currentHash := utils.Digest(entry.Spec.Manifest)
	if entry.Status.ManifestHash != currentHash {
		now := metav1.Now()
		entry.Status.ManifestHash = currentHash
		entry.Status.LastUpdated = &now
		log.Infof("Updated system MCP catalog entry manifest hash: entry=%s hash=%s", entry.Name, currentHash)
		return req.Client.Status().Update(req.Ctx, entry)
	}

	return nil
}

// DetectCompositeDrift detects when a composite catalog entry's component snapshots have drifted
// from their source catalog entries or multi-user servers
func (*Handler) DetectCompositeDrift(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	if entry.Spec.Manifest.Runtime != types.RuntimeComposite {
		if entry.Status.NeedsUpdate {
			entry.Status.NeedsUpdate = false
			return req.Client.Status().Update(req.Ctx, entry)
		}
		return nil
	}

	// Check each component for drift
	var drifted bool
	for _, component := range entry.Spec.Manifest.CompositeConfig.ComponentServers {
		// Handle multi-user component drift
		if component.MCPServerID != "" {
			var server v1.MCPServer
			if err := req.Get(&server, entry.Namespace, component.MCPServerID); err != nil {
				if apierrors.IsNotFound(err) {
					drifted = true
					break
				}
				return fmt.Errorf("failed to get multi-user server %s: %w", component.MCPServerID, err)
			}

			hasDrifted, err := mcpserver.ConfigurationHasDrifted(server.Spec.Manifest, component.Manifest, false)
			if err != nil {
				return fmt.Errorf("failed to detect drift for multi-user server %s: %w", component.MCPServerID, err)
			}
			if hasDrifted {
				drifted = true
				break
			}
		} else {
			// Handle catalog entry component drift
			var componentEntry v1.MCPServerCatalogEntry
			if err := req.Get(&componentEntry, entry.Namespace, component.CatalogEntryID); err != nil {
				if apierrors.IsNotFound(err) {
					drifted = true
					break
				}
				return fmt.Errorf("failed to get component catalog entry %s: %w", component.CatalogEntryID, err)
			}

			var (
				snapshotHash = utils.Digest(component.Manifest)
				currentHash  = utils.Digest(componentEntry.Spec.Manifest)
			)
			if snapshotHash != currentHash {
				drifted = true
				break
			}
		}
	}

	if entry.Status.NeedsUpdate != drifted {
		log.Infof("MCP catalog entry composite drift status changed: entry=%s needsUpdate=%v", entry.Name, drifted)
		entry.Status.NeedsUpdate = drifted
		return req.Client.Status().Update(req.Ctx, entry)
	}

	return nil
}

// CleanupNestedCompositeServers removes component servers with composite runtimes from composite catalog entries.
// This handler cleans up entries that were created before API validation to prevent nested composite servers.
func (*Handler) CleanupNestedCompositeEntries(req router.Request, _ router.Response) error {
	var (
		entry    = req.Object.(*v1.MCPServerCatalogEntry)
		manifest = entry.Spec.Manifest
	)

	if manifest.Runtime != types.RuntimeComposite ||
		manifest.CompositeConfig == nil {
		return nil
	}

	// Remove all composite components from the server's manifest
	var (
		components    = manifest.CompositeConfig.ComponentServers
		numComponents = len(components)
	)
	components = slices.DeleteFunc(components, func(component types.CatalogComponentServer) bool {
		return component.Manifest.Runtime == types.RuntimeComposite
	})

	if numComponents == len(components) {
		// No components were removed, so no need to update the manifest.
		return nil
	}

	entry.Spec.Manifest.CompositeConfig.ComponentServers = components
	log.Infof("Pruned nested composite components from MCP catalog entry: entry=%s removedComponents=%d", entry.Name, numComponents-len(components))
	return kclient.IgnoreNotFound(req.Client.Update(req.Ctx, entry))
}

// CleanupUnusedOAuthCredentials removes OAuth credentials for remote catalog entries
// that no longer require static OAuth configuration.
func (h *Handler) CleanupUnusedOAuthCredentials(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	// Only process remote entries
	if entry.Spec.Manifest.Runtime != types.RuntimeRemote {
		return nil
	}

	// Only cleanup if RemoteConfig exists and StaticOAuthRequired is false
	if entry.Spec.Manifest.RemoteConfig != nil && entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
		return nil
	}

	deleted, err := h.gatewayClient.DeleteCredential(req.Ctx, system.MCPOAuthCredentialName(entry.Name), "oauth")
	if err != nil {
		return fmt.Errorf("failed to delete OAuth credential: %w", err)
	}
	if deleted {
		log.Infof("Deleted unused static OAuth credential for MCP catalog entry: entry=%s", entry.Name)
	}

	return nil
}

// EnsureOAuthCredentialStatus updates the OAuthCredentialConfigured status field
// for remote catalog entries that require static OAuth.
func (h *Handler) EnsureOAuthCredentialStatus(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	// Clear sync annotation if present
	if _, exists := entry.Annotations[v1.MCPServerCatalogEntrySyncAnnotation]; exists {
		delete(entry.Annotations, v1.MCPServerCatalogEntrySyncAnnotation)
		if err := req.Client.Update(req.Ctx, entry); err != nil {
			return fmt.Errorf("failed to clear sync annotation: %w", err)
		}
		log.Infof("Cleared sync annotation for MCP catalog entry: entry=%s", entry.Name)
	}

	// Only process remote entries that require static OAuth
	if entry.Spec.Manifest.Runtime != types.RuntimeRemote ||
		entry.Spec.Manifest.RemoteConfig == nil ||
		!entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
		// Clear status if not applicable
		if entry.Status.OAuthCredentialConfigured {
			entry.Status.OAuthCredentialConfigured = false
			log.Infof("Cleared static OAuth credential status for MCP catalog entry: entry=%s", entry.Name)
			return req.Client.Status().Update(req.Ctx, entry)
		}

		return nil
	}

	// Check if credentials exist
	credName := system.MCPOAuthCredentialName(entry.Name)
	_, err := h.gatewayClient.RevealCredential(req.Ctx, []string{credName}, "oauth")

	var configured bool
	if err == nil {
		configured = true
	} else if !errors.As(err, &gclient.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to check credential status: %w", err)
	}

	if entry.Status.OAuthCredentialConfigured != configured {
		entry.Status.OAuthCredentialConfigured = configured
		log.Infof("Updated static OAuth credential status for MCP catalog entry: entry=%s configured=%v", entry.Name, configured)
		return req.Client.Status().Update(req.Ctx, entry)
	}

	return nil
}

// RemoveOAuthCredentials removes OAuth credentials when a catalog entry is deleted.
func (h *Handler) RemoveOAuthCredentials(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	// Only process remote entries
	if entry.Spec.Manifest.Runtime != types.RuntimeRemote {
		return nil
	}

	// Build the credential name for this entry
	credName := system.MCPOAuthCredentialName(entry.Name)

	deleted, err := h.gatewayClient.DeleteCredential(req.Ctx, credName, "oauth")
	if err != nil {
		return fmt.Errorf("failed to delete OAuth credential: %w", err)
	}
	if deleted {
		log.Infof("Removed static OAuth credential for deleted MCP catalog entry: entry=%s", entry.Name)
	}

	return nil
}
