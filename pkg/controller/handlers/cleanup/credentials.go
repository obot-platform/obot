package cleanup

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/api/handlers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Credentials struct {
	gClient           *gptscript.GPTScript
	gatewayClient     *gateway.Client
	mcpSessionManager *mcp.SessionManager
	serverURL         string
	internalServerURL string
}

func NewCredentials(gClient *gptscript.GPTScript, mcpSessionManager *mcp.SessionManager, gatewayClient *gateway.Client, serverURL, internalServerURL string) *Credentials {
	return &Credentials{
		gClient:           gClient,
		gatewayClient:     gatewayClient,
		mcpSessionManager: mcpSessionManager,
		serverURL:         serverURL,
		internalServerURL: internalServerURL,
	}
}

func (c *Credentials) Remove(req router.Request, _ router.Response) error {
	creds, err := c.gClient.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{req.Object.GetName()},
	})
	if err != nil {
		return err
	}
	localCreds, err := c.gClient.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{req.Object.GetName() + "-local"},
	})
	if err != nil {
		return err
	}

	creds = append(creds, localCreds...)

	// Credentials for model providers
	var modelProviders v1.ToolReferenceList
	if err = req.List(&modelProviders, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{"spec.type": string(v1.ToolReferenceTypeModelProvider)}),
		Namespace:     req.Namespace,
	}); err != nil {
		return err
	}

	projectName := strings.Replace(req.Name, system.ThreadPrefix, system.ProjectPrefix, 1)
	modelProviderCredContexts := make([]string, 0, len(modelProviders.Items))
	for _, modelProvider := range modelProviders.Items {
		modelProviderCredContexts = append(modelProviderCredContexts, fmt.Sprintf("%s-%s", projectName, modelProvider.Name))
	}

	mpCreds, err := c.gClient.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: modelProviderCredContexts,
	})
	if err != nil {
		return err
	}

	creds = append(creds, mpCreds...)

	for _, cred := range creds {
		if err := c.gClient.DeleteCredential(req.Ctx, cred.Context, cred.ToolName); err != nil {
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
	if err := c.gClient.DeleteCredential(req.Ctx, mcpServer.Name, mcpServer.Name+"-audit-log-token"); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
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

	creds, err := c.gClient.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		return err
	}

	for _, cred := range creds {
		if err = c.gClient.DeleteCredential(req.Ctx, cred.Context, cred.ToolName); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
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

	creds, err := c.gClient.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{handlers.MCPServerInstanceCredentialContext(*mcpServerInstance)},
	})
	if err != nil {
		return err
	}

	for _, cred := range creds {
		if err = c.gClient.DeleteCredential(req.Ctx, cred.Context, cred.ToolName); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return err
		}
	}

	return nil
}
