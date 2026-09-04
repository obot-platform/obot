package cleanup

import (
	"fmt"
	"log/slog"

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

// RemoveAuditLogCred removes the credential an older Obot stored per server for token exchange
// and external audit log submitting, at context and name both equal to the server's name. That
// credential was written by EnsureOAuthClient, which no longer exists, so a server only ever needs
// sweeping once.
//
// The sweep cannot be a migration, because Obot is deployed by others and an upgrade arrives
// whenever a deployer takes it. So it stays a handler, and an annotation on the server records
// that it has run. Without that annotation it is a DELETE against the credentials table on every
// reconcile of every server, matching no rows, which is one of the largest sources of slow query
// log lines in a busy deployment.
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

	// Recorded only after the delete succeeds, so a failure leaves the sweep pending rather than
	// marking a server clean that still holds the credential.
	annotations := req.Object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[v1.AuditLogCredentialRemovedAnnotation] = "true"
	req.Object.SetAnnotations(annotations)

	return req.Client.Update(req.Ctx, req.Object)
}
