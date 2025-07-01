package mcpserverinstance

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PopulateStatus populates the status of the instance.
func PopulateStatus(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.MCPServerInstance)

	var server v1.MCPServer
	if err := req.Client.Get(req.Ctx, client.ObjectKey{
		Namespace: instance.Namespace,
		Name:      instance.Spec.MCPServerName,
	}, &server); err != nil {
		return err
	}

	if server.Spec.SharedWithinMCPCatalogName != "" {
		instance.Status.MCPCatalogName = server.Spec.SharedWithinMCPCatalogName
	} else {
		instance.Status.MCPServerCatalogEntryName = server.Spec.MCPServerCatalogEntryName

		var entry v1.MCPServerCatalogEntry
		if err := req.Client.Get(req.Ctx, client.ObjectKey{
			Namespace: instance.Namespace,
			Name:      instance.Status.MCPServerCatalogEntryName,
		}, &entry); err != nil {
			return err
		}

		instance.Status.MCPCatalogName = entry.Spec.MCPCatalogName
	}

	return req.Client.Status().Update(req.Ctx, instance)
}
