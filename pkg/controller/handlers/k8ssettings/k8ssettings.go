package k8ssettings

import (
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
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
