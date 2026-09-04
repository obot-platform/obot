package mcpservercatalogentry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpserver"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type credentialClient interface {
	RevealCredential(ctx context.Context, contexts []string, name string) (gatewaytypes.Credential, error)
	DeleteCredential(ctx context.Context, credentialContext, name string) (bool, error)
}

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
	currentHash := utils.Digest(entry.Spec.Manifest)
	if entry.Status.ManifestHash != currentHash {
		now := metav1.Now()
		entry.Status.ManifestHash = currentHash
		entry.Status.LastUpdated = &now
		slog.Info("Updated MCP catalog entry manifest hash", "entry", entry.Name, "hash", currentHash)
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
		slog.Info("Updated system MCP catalog entry manifest hash", "entry", entry.Name, "hash", currentHash)
		return req.Client.Status().Update(req.Ctx, entry)
	}

	return nil
}

// DetectCompositeDrift detects when a composite catalog entry's component snapshots have drifted
// from their source catalog entries or multi-user servers
func (h *Handler) DetectCompositeDrift(req router.Request, _ router.Response) error {
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

			hasDrifted, err := mcpserver.ConfigurationHasDrifted(req.Ctx, h.gatewayClient, &server, component.Manifest, false)
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

			// We added the EntryKey field, but it really shouldn't affect drift detection here.
			if component.Manifest.EntryKey == "" && componentEntry.Spec.Manifest.EntryKey != "" {
				component.Manifest.EntryKey = componentEntry.Spec.Manifest.EntryKey
			}

			// Same for serverUserType
			if component.Manifest.ServerUserType == "" && componentEntry.Spec.Manifest.ServerUserType != "" {
				component.Manifest.ServerUserType = componentEntry.Spec.Manifest.ServerUserType
			}

			// UpgradeNote is informational metadata and should not affect configuration drift.
			component.Manifest.UpgradeNote = ""
			componentEntry.Spec.Manifest.UpgradeNote = ""

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
		slog.Info("MCP catalog entry composite drift status changed", "entry", entry.Name, "needsUpdate", drifted)
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
	slog.Info("Pruned nested composite components from MCP catalog entry", "entry", entry.Name, "removedComponents", numComponents-len(components))
	return kclient.IgnoreNotFound(req.Client.Update(req.Ctx, entry))
}

// requiresStaticOAuth reports whether entry is configured to use a static OAuth client. Only
// entries in that shape ever have a static OAuth credential.
func requiresStaticOAuth(entry *v1.MCPServerCatalogEntry) bool {
	return entry.Spec.Manifest.Runtime == types.RuntimeRemote &&
		entry.Spec.Manifest.RemoteConfig != nil &&
		entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired
}

// ReconcileOAuthCredential keeps an entry's static OAuth credential and
// Status.OAuthCredentialConfigured in agreement, querying the credential store only when that
// status could be wrong.
//
// The delete and the status update must stay in one handler. nah runs every handler for a type
// even after an earlier one errors, so as two handlers a failed delete did not stop the status
// being cleared, and a cleared status meant the entry was never queried again.
func (h *Handler) ReconcileOAuthCredential(req router.Request, _ router.Response) error {
	return reconcileOAuthCredential(req, h.gatewayClient)
}

func reconcileOAuthCredential(req router.Request, creds credentialClient) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	// Set by the API after it writes or deletes a credential.
	_, recheck := entry.Annotations[v1.MCPServerCatalogEntrySyncAnnotation]

	configured, err := syncOAuthCredential(req.Ctx, creds, entry, recheck)
	if err != nil {
		return err
	}

	if entry.Status.OAuthCredentialConfigured != configured {
		entry.Status.OAuthCredentialConfigured = configured
		slog.Info("Updated static OAuth credential status for MCP catalog entry", "entry", entry.Name, "configured", configured)
		if err := req.Client.Status().Update(req.Ctx, entry); err != nil {
			return fmt.Errorf("failed to update OAuth credential status: %w", err)
		}
	}

	if !recheck {
		return nil
	}

	// Cleared last, so a failure above leaves the recheck pending.
	delete(entry.Annotations, v1.MCPServerCatalogEntrySyncAnnotation)
	if err := req.Client.Update(req.Ctx, entry); err != nil {
		return fmt.Errorf("failed to clear sync annotation: %w", err)
	}
	slog.Info("Cleared sync annotation for MCP catalog entry", "entry", entry.Name)

	return nil
}

// syncOAuthCredential brings the credential store in line with entry and reports whether a
// credential exists for it afterwards.
func syncOAuthCredential(ctx context.Context, creds credentialClient, entry *v1.MCPServerCatalogEntry, recheck bool) (bool, error) {
	credName := system.MCPOAuthCredentialName(entry.Name)

	if requiresStaticOAuth(entry) {
		if entry.Status.OAuthCredentialConfigured && !recheck {
			return true, nil
		}

		_, err := creds.RevealCredential(ctx, []string{credName}, system.StaticOAuthCredentialName)
		if err == nil {
			return true, nil
		}
		if !errors.As(err, &gclient.CredentialNotFoundError{}) {
			return false, fmt.Errorf("failed to check OAuth credential status: %w", err)
		}

		return false, nil
	}

	// Any credential here is left over from when the entry did use static OAuth. The runtime is
	// not checked, because an entry changed away from remote keeps its credential under the same
	// name and nothing else would remove it.
	if !entry.Status.OAuthCredentialConfigured && !recheck {
		return false, nil
	}

	deleted, err := creds.DeleteCredential(ctx, credName, system.StaticOAuthCredentialName)
	if err != nil {
		return false, fmt.Errorf("failed to delete OAuth credential: %w", err)
	}
	if deleted {
		slog.Info("Deleted unused static OAuth credential for MCP catalog entry", "entry", entry.Name)
	}

	return false, nil
}

// RemoveOAuthCredentials removes OAuth credentials when a catalog entry is deleted.
func (h *Handler) RemoveOAuthCredentials(req router.Request, _ router.Response) error {
	return removeOAuthCredentials(req, h.gatewayClient)
}

func removeOAuthCredentials(req router.Request, creds credentialClient) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	// An entry on its way out is always swept, since being wrong there strands the credential. The
	// rest of the guard only matters if this is ever called outside the deletion path.
	_, recheck := entry.Annotations[v1.MCPServerCatalogEntrySyncAnnotation]
	if entry.Spec.Manifest.Runtime != types.RuntimeRemote && !entry.Status.OAuthCredentialConfigured && !recheck && entry.DeletionTimestamp.IsZero() {
		return nil
	}

	// Build the credential name for this entry
	credName := system.MCPOAuthCredentialName(entry.Name)

	deleted, err := creds.DeleteCredential(req.Ctx, credName, system.StaticOAuthCredentialName)
	if err != nil {
		return fmt.Errorf("failed to delete OAuth credential: %w", err)
	}
	if deleted {
		slog.Info("Removed static OAuth credential for deleted MCP catalog entry", "entry", entry.Name)
	}

	return nil
}
