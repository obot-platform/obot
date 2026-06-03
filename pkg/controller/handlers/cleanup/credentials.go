package cleanup

import (
	"fmt"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/api/handlers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Credentials struct {
	gatewayClient     *gateway.Client
	mcpSessionManager *mcp.SessionManager
	serverURL         string
	internalServerURL string
}

func NewCredentials(mcpSessionManager *mcp.SessionManager, gatewayClient *gateway.Client, serverURL, internalServerURL string) *Credentials {
	return &Credentials{
		gatewayClient:     gatewayClient,
		mcpSessionManager: mcpSessionManager,
		serverURL:         serverURL,
		internalServerURL: internalServerURL,
	}
}

func (c *Credentials) Remove(req router.Request, _ router.Response) error {
	creds, err := c.gatewayClient.ListCredentials(req.Ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{req.Object.GetName()},
	})
	if err != nil {
		return err
	}
	localCreds, err := c.gatewayClient.ListCredentials(req.Ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{req.Object.GetName() + "-local"},
	})
	if err != nil {
		return err
	}

	creds = append(creds, localCreds...)

	// Credentials for model providers
	var modelProviders v1.ModelProviderList
	if err = req.List(&modelProviders, &kclient.ListOptions{
		Namespace: req.Namespace,
	}); err != nil {
		return err
	}

	projectName := strings.Replace(req.Name, system.ThreadPrefix, system.ProjectPrefix, 1)
	modelProviderCredContexts := make([]string, 0, len(modelProviders.Items))
	for _, modelProvider := range modelProviders.Items {
		modelProviderCredContexts = append(modelProviderCredContexts, fmt.Sprintf("%s-%s", projectName, modelProvider.Name))
	}

	mpCreds, err := c.gatewayClient.ListCredentials(req.Ctx, gateway.ListCredentialsOptions{
		CredentialContexts: modelProviderCredContexts,
	})
	if err != nil {
		return err
	}

	creds = append(creds, mpCreds...)

	for _, cred := range creds {
		if _, err := c.gatewayClient.DeleteCredential(req.Ctx, cred.Context, cred.Name); err != nil {
			return err
		}
	}

	return nil
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
