package mcpservercatalogentry

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

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
		slog.Info("Updated MCP catalog entry user count", "entry", entry.Name, "oldCount", entry.Status.UserCount, "newCount", newUserCount)
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
		slog.Info("Deleting MCP catalog entry with empty runtime", "entry", entry.Name)
		return req.Client.Delete(req.Ctx, entry)
	}

	return nil
}

// UpdateManifestHashAndLastUpdated updates the manifest hash and last updated timestamp when configuration changes
func (*Handler) UpdateManifestHashAndLastUpdated(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)
	currentHash := manifestHash(entry.Spec.Manifest)
	if entry.Status.ManifestHash != currentHash {
		now := metav1.Now()
		entry.Status.ManifestHash = currentHash
		entry.Status.LastUpdated = &now
		slog.Info("Updated MCP catalog entry manifest hash", "entry", entry.Name, "hash", currentHash)
		return req.Client.Status().Update(req.Ctx, entry)
	}

	return nil
}

// manifestHash digests a catalog entry manifest with every composite component's SourceDigest
// cleared, so regenerating tool overrides that come back identical does not move ManifestHash or
// LastUpdated.
func manifestHash(manifest types.MCPServerCatalogEntryManifest) string {
	if manifest.Runtime != types.RuntimeComposite || manifest.CompositeConfig == nil {
		return utils.Digest(manifest)
	}

	manifest = *manifest.DeepCopy()
	for i := range manifest.CompositeConfig.ComponentServers {
		manifest.CompositeConfig.ComponentServers[i].SourceDigest = ""
	}

	return utils.Digest(manifest)
}

func (*Handler) UpdateSystemManifestHashAndLastUpdated(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.SystemMCPServerCatalogEntry)
	currentHash := utils.Digest(entry.Spec.Manifest)
	if entry.Status.ManifestHash != currentHash {
		now := metav1.Now()
		entry.Status.ManifestHash = currentHash
		entry.Status.LastUpdated = &now
		slog.Info("Updated system MCP catalog entry manifest hash", "entry", entry.Name, "hash", currentHash)
		return req.Client.Status().Update(req.Ctx, entry)
	}

	return nil
}

// DetectCompositeDrift stamps each component of a composite catalog entry with what its upstream
// currently reports. Nothing is copied into the entry's manifest: Name and Icon are status only,
// kept at their last values once the upstream stops resolving.
func (h *Handler) DetectCompositeDrift(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	if entry.Spec.Manifest.Runtime != types.RuntimeComposite || entry.Spec.Manifest.CompositeConfig == nil {
		if entry.Status.NeedsUpdate || entry.Status.Components != nil {
			entry.Status.NeedsUpdate = false
			entry.Status.Components = nil
			return req.Client.Status().Update(req.Ctx, entry)
		}
		return nil
	}

	// Index what the last pass stamped, so a component whose upstream is gone keeps its name and icon.
	stamped := make(map[string]v1.CatalogComponentServerStatus, len(entry.Status.Components))
	for _, component := range entry.Status.Components {
		if id := component.ComponentID(); id != "" {
			stamped[id] = component
		}
	}

	var (
		components  = make([]v1.CatalogComponentServerStatus, 0, len(entry.Spec.Manifest.CompositeConfig.ComponentServers))
		needsUpdate bool
	)
	for _, component := range entry.Spec.Manifest.CompositeConfig.ComponentServers {
		upstream, err := h.resolveComponent(req, entry.Namespace, component)
		if err != nil {
			return err
		}

		status := v1.CatalogComponentServerStatus{CatalogEntryID: component.CatalogEntryID, MCPServerID: component.MCPServerID}
		if previous, ok := stamped[component.ComponentID()]; ok {
			status.Name, status.Icon = previous.Name, previous.Icon
		}

		if upstream.Missing {
			status.Missing = true
		} else {
			status.Name, status.Icon = upstream.Manifest.Name, upstream.Manifest.Icon
			status.ToolOverridesStale = mcp.ComponentToolOverridesStale(component, upstream)
		}

		needsUpdate = needsUpdate || status.Missing || status.ToolOverridesStale
		components = append(components, status)
	}

	if entry.Status.NeedsUpdate == needsUpdate && utils.Digest(entry.Status.Components) == utils.Digest(components) {
		return nil
	}

	slog.Info("MCP catalog entry composite component status changed", "entry", entry.Name, "needsUpdate", needsUpdate)
	entry.Status.NeedsUpdate = needsUpdate
	entry.Status.Components = components
	return req.Client.Status().Update(req.Ctx, entry)
}

// resolveComponent reads the upstream one component reference points at. Reading through the
// router registers a watch trigger on it, including when the read 404s, so this handler is woken
// if a missing upstream appears.
func (*Handler) resolveComponent(req router.Request, namespace string, component types.CatalogComponentServer) (mcp.ResolvedComponent, error) {
	switch {
	case component.MCPServerID != "":
		var server v1.MCPServer
		if err := req.Get(&server, namespace, component.MCPServerID); err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.ResolvedComponent{Missing: true}, nil
			}
			return mcp.ResolvedComponent{}, fmt.Errorf("failed to get multi-user server %s: %w", component.MCPServerID, err)
		}

		return mcp.ResolvedComponent{Manifest: server.Spec.Manifest.ConvertToCatalogEntry()}, nil
	case component.CatalogEntryID != "":
		var componentEntry v1.MCPServerCatalogEntry
		if err := req.Get(&componentEntry, namespace, component.CatalogEntryID); err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.ResolvedComponent{Missing: true}, nil
			}
			return mcp.ResolvedComponent{}, fmt.Errorf("failed to get component catalog entry %s: %w", component.CatalogEntryID, err)
		}

		return mcp.ResolvedComponent{Manifest: componentEntry.Spec.Manifest}, nil
	}

	return mcp.ResolvedComponent{Missing: true}, nil
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

	deleted, err := h.gatewayClient.DeleteCredential(req.Ctx, system.MCPOAuthCredentialName(entry.Name), system.StaticOAuthCredentialName)
	if err != nil {
		return fmt.Errorf("failed to delete OAuth credential: %w", err)
	}
	if deleted {
		slog.Info("Deleted unused static OAuth credential for MCP catalog entry", "entry", entry.Name)
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
		slog.Info("Cleared sync annotation for MCP catalog entry", "entry", entry.Name)
	}

	// Only process remote entries that require static OAuth
	if entry.Spec.Manifest.Runtime != types.RuntimeRemote ||
		entry.Spec.Manifest.RemoteConfig == nil ||
		!entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
		// Clear status if not applicable
		if entry.Status.OAuthCredentialConfigured {
			entry.Status.OAuthCredentialConfigured = false
			slog.Info("Cleared static OAuth credential status for MCP catalog entry", "entry", entry.Name)
			return req.Client.Status().Update(req.Ctx, entry)
		}

		return nil
	}

	// Check if credentials exist
	credName := system.MCPOAuthCredentialName(entry.Name)
	_, err := h.gatewayClient.RevealCredential(req.Ctx, []string{credName}, system.StaticOAuthCredentialName)

	var configured bool
	if err == nil {
		configured = true
	} else if !errors.As(err, &gclient.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to check credential status: %w", err)
	}

	if entry.Status.OAuthCredentialConfigured != configured {
		entry.Status.OAuthCredentialConfigured = configured
		slog.Info("Updated static OAuth credential status for MCP catalog entry", "entry", entry.Name, "configured", configured)
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

	deleted, err := h.gatewayClient.DeleteCredential(req.Ctx, credName, system.StaticOAuthCredentialName)
	if err != nil {
		return fmt.Errorf("failed to delete OAuth credential: %w", err)
	}
	if deleted {
		slog.Info("Removed static OAuth credential for deleted MCP catalog entry", "entry", entry.Name)
	}

	return nil
}
