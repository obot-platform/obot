package mcpserverinstance

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Migrate makes sure that all spec fields are set properly.
func Migrate(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.MCPServerInstance)

	var server v1.MCPServer
	if err := req.Client.Get(req.Ctx, client.ObjectKey{
		Namespace: instance.Namespace,
		Name:      instance.Spec.MCPServerName,
	}, &server); err != nil {
		return err
	}

	if server.Spec.SharedWithinMCPCatalogName != "" {
		instance.Spec.MCPCatalogName = server.Spec.SharedWithinMCPCatalogName
	} else {
		instance.Spec.MCPServerCatalogEntryName = server.Spec.MCPServerCatalogEntryName

		var entry v1.MCPServerCatalogEntry
		if err := req.Client.Get(req.Ctx, client.ObjectKey{
			Namespace: instance.Namespace,
			Name:      instance.Spec.MCPServerCatalogEntryName,
		}, &entry); err != nil {
			return err
		}

		instance.Spec.MCPCatalogName = entry.Spec.MCPCatalogName
	}

	return req.Client.Update(req.Ctx, instance)
}
