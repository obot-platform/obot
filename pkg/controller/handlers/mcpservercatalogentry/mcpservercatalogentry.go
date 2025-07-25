package mcpservercatalogentry

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func RemoveArgsOnRemoteEntries(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	if entry.Spec.SourceURL != "" {
		return nil
	}

	// The URLManifest should never have args on it, but there was a bug where sometimes they were created with one empty string as an argument.
	if entry.Spec.URLManifest.Args != nil {
		entry.Spec.URLManifest.Args = nil
		return req.Client.Update(req.Ctx, entry)
	}

	return nil
}

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
		if server.Spec.UserID == "" {
			// A server should always have a user ID, but if it doesn't, don't count it.
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
