package mcpserver

import (
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

// DeleteOrphans deletes non-shared MCP servers that have no MCPServerInstances at least one hour after creation.
func DeleteOrphans(req router.Request, resp router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.ThreadName != "" || server.Spec.SharedWithinMCPCatalogName != "" {
		return nil
	} else if time.Since(server.CreationTimestamp.Time) < 2*time.Minute {
		resp.RetryAfter(2 * time.Minute)
		return nil
	}

	var instances v1.MCPServerInstanceList
	if err := req.List(&instances, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.mcpServerName": server.Name,
		}),
		Namespace: req.Namespace,
	}); err != nil {
		return err
	}

	if len(instances.Items) == 0 {
		log.Infof("Deleting orphaned MCP server %s/%s", req.Namespace, server.Name)
		return req.Delete(server)
	} else {
		// TODO: remove this after testing, and change the time from 2 minutes to 1 hour
		log.Infof("Found %d MCP server instances for MCP server %s/%s", len(instances.Items), req.Namespace, server.Name)
	}

	return nil
}
