package cleanup

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/jwt"
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
	tokenService      *jwt.TokenService
	serverURL         string
}

func NewCredentials(gClient *gptscript.GPTScript, mcpSessionManager *mcp.SessionManager, gatewayClient *gateway.Client, tokenService *jwt.TokenService, serverURL string) *Credentials {
	return &Credentials{
		gClient:           gClient,
		gatewayClient:     gatewayClient,
		mcpSessionManager: mcpSessionManager,
		tokenService:      tokenService,
		serverURL:         serverURL,
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
		FieldSelector: fields.SelectorFromSet(map[string]string{"spec.type": string(types.ToolReferenceTypeModelProvider)}),
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

func (c *Credentials) removeMCPCredentialsForProject(req router.Request, _ router.Response) error {
	mcpServer := req.Object.(*v1.MCPServer)

	var projects v1.ThreadList
	if err := req.List(&projects, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.parentThreadName": mcpServer.Spec.ThreadName,
		}),
		Namespace: req.Namespace,
	}); err != nil {
		return err
	}

	projectNames := make([]string, 0, len(projects.Items)+1)
	for _, project := range projects.Items {
		if project.Spec.Project {
			projectNames = append(projectNames, project.Name)
		}
	}
	projectNames = append(projectNames, mcpServer.Spec.ThreadName)

	for _, projectName := range projectNames {
		creds, err := c.gClient.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
			CredentialContexts: []string{
				fmt.Sprintf("%s-%s", projectName, mcpServer.Name),
				fmt.Sprintf("%s-%s-shared", projectName, mcpServer.Name),
			},
		})
		if err != nil {
			return err
		}

		for _, cred := range creds {
			// Have to reveal the credential to get the values
			cred, err = c.gClient.RevealCredential(req.Ctx, []string{cred.Context}, cred.ToolName)
			if err != nil {
				return err
			}

			// Shutdown the server
			serverConfig, _, err := mcp.ServerToServerConfig(*mcpServer, projectName, cred.Env)
			if err != nil {
				return fmt.Errorf("failed to create server config: %w", err)
			}

			if err = c.mcpSessionManager.ShutdownServer(req.Ctx, serverConfig); err != nil {
				return fmt.Errorf("failed to shutdown server: %w", err)
			}

			if err = c.gClient.DeleteCredential(req.Ctx, cred.Context, cred.ToolName); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return err
			}
		}

		// Shutdown a potential server running without any configuration. We wouldn't detect its existence with a credential.
		serverConfig, _, err := mcp.ServerToServerConfig(*mcpServer, projectName, nil)
		if err != nil {
			return fmt.Errorf("failed to create server config: %w", err)
		}

		if err = c.mcpSessionManager.ShutdownServer(req.Ctx, serverConfig); err != nil {
			return fmt.Errorf("failed to shutdown server: %w", err)
		}
	}

	return nil
}

func (c *Credentials) RemoveMCPCredentials(req router.Request, resp router.Response) error {
	mcpServer := req.Object.(*v1.MCPServer)

	if err := c.gatewayClient.DeleteMCPOAuthTokenForAllUsers(req.Ctx, req.Object.GetName()); err != nil {
		return err
	}

	if mcpServer.Spec.ThreadName != "" {
		return c.removeMCPCredentialsForProject(req, resp)
	}

	var credCtx, scope string
	if mcpServer.Spec.SharedWithinMCPCatalogName != "" {
		credCtx = fmt.Sprintf("%s-%s", mcpServer.Spec.SharedWithinMCPCatalogName, mcpServer.Name)
		scope = mcpServer.Spec.SharedWithinMCPCatalogName
	} else {
		credCtx = fmt.Sprintf("%s-%s", mcpServer.Spec.UserID, mcpServer.Name)
		scope = mcpServer.Spec.UserID
	}

	creds, err := c.gClient.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		return err
	}

	for _, cred := range creds {
		// Have to reveal the credential to get the values
		cred, err = c.gClient.RevealCredential(req.Ctx, []string{cred.Context}, cred.ToolName)
		if err != nil {
			return err
		}

		// Shutdown the server
		serverConfig, _, err := mcp.ServerToServerConfig(*mcpServer, scope, cred.Env)
		if err != nil {
			return fmt.Errorf("failed to create server config: %w", err)
		}

		if err = c.mcpSessionManager.ShutdownServer(req.Ctx, serverConfig); err != nil {
			return fmt.Errorf("failed to shutdown server: %w", err)
		}

		if err = c.gClient.DeleteCredential(req.Ctx, cred.Context, cred.ToolName); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return err
		}
	}

	// Shutdown a potential server running without any configuration. We wouldn't detect its existence with a credential.
	serverConfig, _, err := mcp.ServerToServerConfig(*mcpServer, scope, nil)
	if err != nil {
		return fmt.Errorf("failed to create server config: %w", err)
	}

	if err = c.mcpSessionManager.ShutdownServer(req.Ctx, serverConfig); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	return nil
}
