package mcpcatalog

import (
	"context"
	"fmt"

	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// storeCatalogEntryStaticConfiguration moves the literal configuration values of synced entries
// into credentials before the entries are applied or snapshotted by vMCPs. The source is the only
// way to configure a synced entry, so a value removed from the source is removed from the entry.
// An entry whose configuration cannot be stored is left out of the sync and reported.
func storeCatalogEntryStaticConfiguration(ctx context.Context, c kclient.Client, store mcp.StaticConfigurationStore, objs []kclient.Object) ([]kclient.Object, map[string]string, error) {
	result := make([]kclient.Object, 0, len(objs))
	syncErrors := make(map[string]string)
	for _, obj := range objs {
		entry, ok := obj.(*v1.MCPServerCatalogEntry)
		if !ok {
			result = append(result, obj)
			continue
		}

		var existing v1.MCPServerCatalogEntry
		if err := c.Get(ctx, kclient.ObjectKeyFromObject(entry), &existing); err != nil && !apierrors.IsNotFound(err) {
			return nil, nil, fmt.Errorf("failed to get catalog entry %q: %w", entry.Name, err)
		}

		if err := mcp.StoreStaticConfiguration(ctx, store, entry.Name, &entry.Spec.Manifest, existing.Spec.Manifest.StaticConfigurationRevision); err != nil {
			addSyncError(syncErrors, entry.Spec.SourceURL, fmt.Sprintf("catalog entry %q: %v", entry.Spec.Manifest.Name, err))
			continue
		}
		result = append(result, entry)
	}
	return result, syncErrors, nil
}
