package k8ssettings

import (
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// UpdateAllServerK8sSettingsDrift updates the NeedsK8sUpdate status on all MCP servers when K8s settings change
func UpdateAllServerK8sSettingsDrift(req router.Request, _ router.Response) error {
	k8sSettings := req.Object.(*v1.K8sSettings)

	// Compute the new hash
	currentHash := mcp.ComputeK8sSettingsHash(k8sSettings.Spec)

	// List all MCP servers
	var servers v1.MCPServerList
	if err := req.List(&servers, &kclient.ListOptions{
		Namespace: req.Object.GetNamespace(),
	}); err != nil {
		return err
	}

	// Update each server's NeedsK8sUpdate status
	for i := range servers.Items {
		server := &servers.Items[i]
		// Skip servers without K8s settings hash (not deployed or non-K8s runtimes)
		if server.Status.K8sSettingsHash == "" {
			if server.Status.NeedsK8sUpdate {
				server.Status.NeedsK8sUpdate = false
				if err := req.Client.Status().Update(req.Ctx, server); err != nil {
					return err
				}
			}
			continue
		}

		// Check if the server needs update
		needsK8sUpdate := server.Status.K8sSettingsHash != currentHash

		if server.Status.NeedsK8sUpdate != needsK8sUpdate {
			server.Status.NeedsK8sUpdate = needsK8sUpdate
			if err := req.Client.Status().Update(req.Ctx, server); err != nil {
				return err
			}
		}
	}

	return nil
}

// UpdateAllCatalogEntryK8sSettingsDrift updates the NeedsK8sUpdate status on all catalog entries when K8s settings change
func UpdateAllCatalogEntryK8sSettingsDrift(req router.Request, _ router.Response) error {
	k8sSettings := req.Object.(*v1.K8sSettings)

	// Compute the new hash
	currentHash := mcp.ComputeK8sSettingsHash(k8sSettings.Spec)

	// List all catalog entries
	var entries v1.MCPServerCatalogEntryList
	if err := req.List(&entries, &kclient.ListOptions{
		Namespace: req.Object.GetNamespace(),
	}); err != nil {
		return err
	}

	// Update each catalog entry's NeedsK8sUpdate status
	for i := range entries.Items {
		entry := &entries.Items[i]
		// List all servers created from this catalog entry
		var mcpServers v1.MCPServerList
		if err := req.List(&mcpServers, &kclient.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("spec.mcpServerCatalogEntryName", entry.Name),
			Namespace:     system.DefaultNamespace,
		}); err != nil {
			return err
		}

		// Check if any server has outdated K8s settings
		var needsK8sUpdate bool
		for _, server := range mcpServers.Items {
			// Skip servers being deleted or without K8s settings hash
			if !server.DeletionTimestamp.IsZero() || server.Status.K8sSettingsHash == "" {
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
			if err := req.Client.Status().Update(req.Ctx, entry); err != nil {
				return err
			}
		}
	}

	return nil
}
