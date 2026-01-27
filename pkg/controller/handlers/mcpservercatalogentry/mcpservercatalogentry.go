package mcpservercatalogentry

import (
	"errors"
	"fmt"
	"slices"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Handler handles operations for MCP server catalog entries
type Handler struct {
	gClient *gptscript.GPTScript
}

// NewHandler creates a new Handler with the given GPTScript client
func NewHandler(gClient *gptscript.GPTScript) *Handler {
	return &Handler{
		gClient: gClient,
	}
}

// EnsureUserCount ensures that the user count for an MCP server catalog entry is up to date.
func (*Handler) EnsureUserCount(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	var mcpServers v1.MCPServerList
	if err := req.List(&mcpServers, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.mcpServerCatalogEntryName", entry.Name),
		Namespace:     system.DefaultNamespace,
	}); err != nil {
		return fmt.Errorf("failed to list MCP servers: %w", err)
	}

	uniqueUsers := make(map[string]struct{}, len(mcpServers.Items))
	for _, server := range mcpServers.Items {
		// Don't count servers that don't have a user ID, are being deleted, or are part of a composite server.
		if server.Spec.UserID != "" && server.DeletionTimestamp.IsZero() && server.Spec.CompositeName == "" {
			uniqueUsers[server.Spec.UserID] = struct{}{}
		}
	}

	if newUserCount := len(uniqueUsers); entry.Status.UserCount != newUserCount {
		entry.Status.UserCount = newUserCount
		return req.Client.Status().Update(req.Ctx, entry)
	}

	return nil
}

func (h *Handler) DeleteEntriesWithoutRuntime(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)
	if string(entry.Spec.Manifest.Runtime) == "" {
		return req.Client.Delete(req.Ctx, entry)
	}

	return nil
}

// UpdateManifestHashAndLastUpdated updates the manifest hash and last updated timestamp when configuration changes
func (*Handler) UpdateManifestHashAndLastUpdated(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	// Compute current config hash
	currentHash := hash.Digest(entry.Spec.Manifest)

	// Only update if hash has changed
	if entry.Status.ManifestHash != currentHash {
		now := metav1.Now()
		entry.Status.ManifestHash = currentHash
		entry.Status.LastUpdated = &now
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

			var (
				snapshotHash = hash.Digest(component.Manifest)
				currentHash  = hash.Digest(server.Spec.Manifest)
			)
			if snapshotHash != currentHash {
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
				snapshotHash = hash.Digest(component.Manifest)
				currentHash  = hash.Digest(componentEntry.Spec.Manifest)
			)
			if snapshotHash != currentHash {
				drifted = true
				break
			}
		}
	}

	if entry.Status.NeedsUpdate != drifted {
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

	if err := h.gClient.DeleteCredential(req.Ctx, system.MCPOAuthCredentialName(entry.Name), "oauth"); err != nil {
		if errors.As(err, &gptscript.ErrNotFound{}) {
			return nil
		}
		return fmt.Errorf("failed to delete OAuth credential: %w", err)
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
	}

	// Only process remote entries that require static OAuth
	if entry.Spec.Manifest.Runtime != types.RuntimeRemote ||
		entry.Spec.Manifest.RemoteConfig == nil ||
		!entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
		// Clear status if not applicable
		if entry.Status.OAuthCredentialConfigured {
			entry.Status.OAuthCredentialConfigured = false
			return req.Client.Status().Update(req.Ctx, entry)
		}

		return nil
	}

	// Check if credentials exist
	credName := system.MCPOAuthCredentialName(entry.Name)
	_, err := h.gClient.RevealCredential(req.Ctx, []string{credName}, "oauth")

	var configured bool
	if err == nil {
		configured = true
	} else if !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to check credential status: %w", err)
	}

	if entry.Status.OAuthCredentialConfigured != configured {
		entry.Status.OAuthCredentialConfigured = configured
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

	// Delete the OAuth credential if it exists
	if err := h.gClient.DeleteCredential(req.Ctx, credName, "oauth"); err != nil {
		if errors.As(err, &gptscript.ErrNotFound{}) {
			// Already deleted or never existed, that's fine
			return nil
		}
		return fmt.Errorf("failed to delete OAuth credential: %w", err)
	}

	return nil
}
