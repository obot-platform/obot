package mcpservercatalogentry

import (
	"fmt"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/router"
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
		if server.Spec.UserID == "" || !server.DeletionTimestamp.IsZero() {
			// A server should always have a user ID, but if it doesn't, don't count it.
			// Additionally, don't count servers that are being deleted.
			continue
		}

		uniqueUsers[server.Spec.UserID] = struct{}{}
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
