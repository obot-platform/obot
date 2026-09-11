package cleanup

import (
	"fmt"
	"log/slog"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/api/handlers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/vmcp"
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

	creds, err := c.gatewayClient.ListCredentials(req.Ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{mcpServer.CredentialContext(mcpServer.Spec.UserID)},
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

func (c *Credentials) RemoveVMCPStaticConfigurationCredentials(req router.Request, _ router.Response) error {
	_, err := c.gatewayClient.DeleteCredential(req.Ctx, vmcp.StaticConfigurationCredentialContext(req.Name), vmcp.ConfigurationCredentialName())
	return err
}

func (c *Credentials) RemoveVMCPInstanceConfigurationCredentials(req router.Request, _ router.Response) error {
	_, err := c.gatewayClient.DeleteCredential(req.Ctx, vmcp.InstanceConfigurationCredentialContext(req.Name), vmcp.ConfigurationCredentialName())
	return err
}

// RemoveAuditLogCred removes the credential an older Obot stored per server for token exchange
// and external audit log submitting. Nothing creates it any more, so a server is swept once and
// the annotation records that it has been.
func (c *Credentials) RemoveAuditLogCred(req router.Request, _ router.Response) error {
	if _, swept := req.Object.GetAnnotations()[v1.AuditLogCredentialRemovedAnnotation]; swept {
		return nil
	}

	credentialName := req.Name
	if _, ok := req.Object.(*v1.SystemMCPServer); ok {
		credentialName += "-secret-info"
	}

	deleted, err := c.gatewayClient.DeleteCredential(req.Ctx, req.Name, credentialName)
	if err != nil {
		return err
	}
	if deleted {
		slog.Info("Removed legacy token exchange and audit log credential", "server", req.Name)
	}

	// Recorded only after the delete succeeds, so a failure retries.
	annotations := req.Object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[v1.AuditLogCredentialRemovedAnnotation] = "true"
	req.Object.SetAnnotations(annotations)

	return req.Client.Update(req.Ctx, req.Object)
}
