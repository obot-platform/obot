package mcpserver

import (
	"fmt"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/errors"
)

var log = logger.Package()

// DeleteOrphans deletes non-shared MCPServer that have no MCPServerInstances at least one hour after creation.
func DeleteOrphans(req router.Request, resp router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.ThreadName != "" || server.Spec.SharedWithinMCPCatalogName != "" {
		return nil
	} else if time.Since(server.CreationTimestamp.Time) < time.Hour {
		resp.RetryAfter(time.Hour)
		return nil
	}

	var instance v1.MCPServerInstance
	if err := req.Get(&instance, server.Namespace, fmt.Sprintf("%s-%s-%s", system.MCPServerInstancePrefix, server.Spec.UserID, server.Name)); errors.IsNotFound(err) {
		log.Infof("Deleting orphaned MCP server %s/%s", req.Namespace, server.Name)
		return req.Delete(server)
	} else if err != nil {
		return err
	}

	return nil
}
