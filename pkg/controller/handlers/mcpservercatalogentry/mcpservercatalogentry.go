package mcpservercatalogentry

import (
	"fmt"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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
// from their source catalog entries or multi-user servers.
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
		var latestHash, currentHash string
		if component.MCPServerID != "" {
			var latestServer v1.MCPServer
			if err := kclient.IgnoreNotFound(req.Get(&latestServer, entry.Namespace, component.MCPServerID)); err != nil {
				return fmt.Errorf("failed to get multi-user server %s: %w", component.MCPServerID, err)
			}

			// Only compare the subset of fields that are relevant enough to warrant an upgrade.
			// We don't track the runtime configuration for multi-user servers since composites
			// reference existing multi-user servers, rather than standing one up.
			latestHash = hash.Digest(types.MCPServerCatalogEntryManifest{
				Metadata:    latestServer.Spec.Manifest.Metadata,
				Name:        latestServer.Spec.Manifest.Name,
				Description: latestServer.Spec.Manifest.Description,
				Icon:        latestServer.Spec.Manifest.Icon,
			})
			currentHash = hash.Digest(types.MCPServerCatalogEntryManifest{
				Metadata:    component.Manifest.Metadata,
				Name:        component.Manifest.Name,
				Description: component.Manifest.Description,
				Icon:        component.Manifest.Icon,
			})
		} else {
			// Handle catalog entry component drift
			var latestEntry v1.MCPServerCatalogEntry
			if err := kclient.IgnoreNotFound(req.Get(&latestEntry, entry.Namespace, component.CatalogEntryID)); err != nil {
				return fmt.Errorf("failed to get component catalog entry %s: %w", component.CatalogEntryID, err)
			}

			latestHash = hash.Digest(types.MCPServerCatalogEntryManifest{
				Metadata:            latestEntry.Spec.Manifest.Metadata,
				Name:                latestEntry.Spec.Manifest.Name,
				Description:         latestEntry.Spec.Manifest.Description,
				Icon:                latestEntry.Spec.Manifest.Icon,
				Runtime:             latestEntry.Spec.Manifest.Runtime,
				UVXConfig:           latestEntry.Spec.Manifest.UVXConfig,
				NPXConfig:           latestEntry.Spec.Manifest.NPXConfig,
				ContainerizedConfig: latestEntry.Spec.Manifest.ContainerizedConfig,
				RemoteConfig:        latestEntry.Spec.Manifest.RemoteConfig,
				Env:                 latestEntry.Spec.Manifest.Env,
			})
			currentHash = hash.Digest(types.MCPServerCatalogEntryManifest{
				Metadata:            component.Manifest.Metadata,
				Name:                component.Manifest.Name,
				Description:         component.Manifest.Description,
				Icon:                component.Manifest.Icon,
				Runtime:             component.Manifest.Runtime,
				UVXConfig:           component.Manifest.UVXConfig,
				NPXConfig:           component.Manifest.NPXConfig,
				ContainerizedConfig: component.Manifest.ContainerizedConfig,
				RemoteConfig:        component.Manifest.RemoteConfig,
				Env:                 component.Manifest.Env,
			})
		}

		if latestHash != currentHash {
			drifted = true
			break
		}
	}

	if entry.Status.NeedsUpdate != drifted {
		entry.Status.NeedsUpdate = drifted
		return req.Client.Status().Update(req.Ctx, entry)
	}

	return nil
}
