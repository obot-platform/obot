package mcpservercatalogentry

import (
	"fmt"
	"slices"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureUserCount ensures that the user count for an MCP server catalog entry is up to date.
func EnsureUserCount(req router.Request, _ router.Response) error {
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

func DeleteEntriesWithoutRuntime(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)
	if string(entry.Spec.Manifest.Runtime) == "" {
		return req.Client.Delete(req.Ctx, entry)
	}

	return nil
}

// UpdateManifestHashAndLastUpdated updates the manifest hash and last updated timestamp when configuration changes
func UpdateManifestHashAndLastUpdated(req router.Request, _ router.Response) error {
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
func DetectCompositeDrift(req router.Request, _ router.Response) error {
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

// DetectK8sSettingsDrift detects when servers created from this catalog entry need redeployment with new K8s settings
func DetectK8sSettingsDrift(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	// Only check for containerized or uvx/npx runtimes that might run in K8s
	if entry.Spec.Manifest.Runtime != types.RuntimeContainerized &&
		entry.Spec.Manifest.Runtime != types.RuntimeUVX &&
		entry.Spec.Manifest.Runtime != types.RuntimeNPX {
		// Remote and composite servers don't have K8s deployments
		if entry.Status.NeedsK8sUpdate {
			entry.Status.NeedsK8sUpdate = false
			return req.Client.Status().Update(req.Ctx, entry)
		}
		return nil
	}

	// Get current K8s settings
	var k8sSettings v1.K8sSettings
	if err := req.Get(&k8sSettings, req.Object.GetNamespace(), system.K8sSettingsName); err != nil {
		if apierrors.IsNotFound(err) {
			// K8s settings not found, mark as not needing update
			if entry.Status.NeedsK8sUpdate {
				entry.Status.NeedsK8sUpdate = false
				return req.Client.Status().Update(req.Ctx, entry)
			}
			return nil
		}
		return fmt.Errorf("failed to get K8s settings: %w", err)
	}

	// Compute current K8s settings hash
	currentHash := mcp.ComputeK8sSettingsHash(k8sSettings.Spec)

	// List all servers created from this catalog entry
	var mcpServers v1.MCPServerList
	if err := req.List(&mcpServers, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.mcpServerCatalogEntryName", entry.Name),
		Namespace:     req.Object.GetNamespace(),
	}); err != nil {
		return fmt.Errorf("failed to list MCP servers: %w", err)
	}

	// Check if any server has outdated K8s settings
	var needsK8sUpdate bool
	for _, server := range mcpServers.Items {
		// Skip servers being deleted
		if !server.DeletionTimestamp.IsZero() {
			continue
		}

		// Skip servers without K8s settings hash (non-K8s runtimes or not yet deployed)
		if server.Status.K8sSettingsHash == "" {
			continue
		}

		// Check if hash differs from current settings
		if server.Status.K8sSettingsHash != currentHash {
			needsK8sUpdate = true
			break
		}
	}

	if entry.Status.NeedsK8sUpdate != needsK8sUpdate {
		entry.Status.NeedsK8sUpdate = needsK8sUpdate
		return req.Client.Status().Update(req.Ctx, entry)
	}

	return nil
}

// CleanupNestedCompositeServers removes component servers with composite runtimes from composite catalog entries.
// This handler cleans up entries that were created before API validation to prevent nested composite servers.
func CleanupNestedCompositeEntries(req router.Request, _ router.Response) error {
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
