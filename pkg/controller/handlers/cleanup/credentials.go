package cleanup

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/api/handlers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Credentials struct {
	gatewayClient     *gateway.Client
	mcpSessionManager *mcp.SessionManager
	serverURL         string
}

func NewCredentials(mcpSessionManager *mcp.SessionManager, gatewayClient *gateway.Client, serverURL string) *Credentials {
	return &Credentials{
		gatewayClient:     gatewayClient,
		mcpSessionManager: mcpSessionManager,
		serverURL:         serverURL,
	}
}

func (c *Credentials) RemoveMCPCredentials(req router.Request, _ router.Response) error {
	mcpServer := req.Object.(*v1.MCPServer)

	if err := c.gatewayClient.DeleteMCPOAuthTokenForAllUsers(req.Ctx, mcpServer.Name); err != nil {
		return err
	}

	// Cleanup the audit log token.
	if _, err := c.gatewayClient.DeleteCredential(req.Ctx, mcpServer.Name, mcpServer.Name+"-audit-log-token"); err != nil {
		return err
	}

	var credCtx string
	if mcpServer.Spec.IsCatalogServer() {
		credCtx = fmt.Sprintf("%s-%s", mcpServer.Spec.MCPCatalogID, mcpServer.Name)
	} else if mcpServer.Spec.IsPowerUserWorkspaceServer() {
		credCtx = fmt.Sprintf("%s-%s", mcpServer.Spec.PowerUserWorkspaceID, mcpServer.Name)
	} else {
		credCtx = fmt.Sprintf("%s-%s", mcpServer.Spec.UserID, mcpServer.Name)
	}

	creds, err := c.gatewayClient.ListCredentials(req.Ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		return err
	}

	for _, cred := range creds {
		if _, err = c.gatewayClient.DeleteCredential(req.Ctx, cred.Context, cred.Name); err != nil {
			return err
		}
	}

	if err = c.mcpSessionManager.ShutdownServer(req.Ctx, mcpServer.Name); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	return nil
}

func (c *Credentials) RemoveMCPInstanceCredentials(req router.Request, _ router.Response) error {
	mcpServerInstance := req.Object.(*v1.MCPServerInstance)

	if err := c.gatewayClient.DeleteMCPOAuthTokenForAllUsers(req.Ctx, mcpServerInstance.Name); err != nil {
		return err
	}

	creds, err := c.gatewayClient.ListCredentials(req.Ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{handlers.MCPServerInstanceCredentialContext(*mcpServerInstance)},
	})
	if err != nil {
		return err
	}

	for _, cred := range creds {
		if _, err = c.gatewayClient.DeleteCredential(req.Ctx, cred.Context, cred.Name); err != nil {
			return err
		}
	}

	return nil
}
