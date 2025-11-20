package cleanup

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (c *Credentials) ShutdownProjectMCP(req router.Request, _ router.Response) error {
	projectServer := req.Object.(*v1.ProjectMCPServer)

	config, err := mcp.ProjectServerToConfig(*projectServer, c.serverURL, c.internalServerURL, projectServer.Spec.UserID)
	if err != nil {
		return fmt.Errorf("failed to convert project server to config: %w", err)
	}

	if err = c.mcpSessionManager.CloseClient(req.Ctx, config, "default"); err != nil {
		return fmt.Errorf("failed to shutdown project server: %w", err)
	}

	return nil
}
