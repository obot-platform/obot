package systemmcpserver

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/crypto/bcrypt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kwait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Handler struct {
	gptClient         *gptscript.GPTScript
	mcpSessionManager *mcp.SessionManager
	serverURL         string
}

func New(gptClient *gptscript.GPTScript, mcpLoader *mcp.SessionManager, serverURL string) *Handler {
	return &Handler{
		gptClient:         gptClient,
		mcpSessionManager: mcpLoader,
		serverURL:         serverURL,
	}
}

// EnsureSecretInfo ensures an OAuthClient and token exchange credentials exist for the system MCP server.
func (h *Handler) EnsureSecretInfo(req router.Request, _ router.Response) error {
	systemServer := req.Object.(*v1.SystemMCPServer)

	fieldSelector := fields.SelectorFromSet(map[string]string{
		"spec.mcpServerName": systemServer.Name,
	})
	var oauthClients v1.OAuthClientList
	if err := req.List(&oauthClients, &kclient.ListOptions{
		Namespace:     req.Namespace,
		FieldSelector: fieldSelector,
	}); err != nil {
		return err
	}

	if len(oauthClients.Items) == 0 {
		// Double-check with the uncached listing
		if err := req.List(untriggered.UncachedList(&oauthClients), &kclient.ListOptions{
			Namespace:     req.Namespace,
			FieldSelector: fieldSelector,
		}); err != nil {
			return err
		}
	}

	secretCredToolName := secretInfoToolName(systemServer.Name)

	if systemServer.Status.AuditLogTokenHash != "" {
		cred, err := h.gptClient.RevealCredential(req.Ctx, []string{systemServer.Name}, secretCredToolName)
		if err != nil {
			return fmt.Errorf("failed to get credential: %w", err)
		}

		if systemServer.Status.AuditLogTokenHash != hash.Digest(cred.Env["AUDIT_LOG_TOKEN"]) {
			// Reset the audit log token hash to reset the credential.
			systemServer.Status.AuditLogTokenHash = ""
		}
	}

	if len(oauthClients.Items) > 0 && systemServer.Status.AuditLogTokenHash != "" {
		return nil
	}

	clientID := system.OAuthClientPrefix + strings.ToLower(rand.Text())
	clientSecret := strings.ToLower(rand.Text() + rand.Text())
	hashedClientSecretHash, err := bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash client secret: %w", err)
	}

	auditLogToken := strings.ToLower(rand.Text() + rand.Text())

	if err := h.gptClient.CreateCredential(req.Ctx, gptscript.Credential{
		Context:  systemServer.Name,
		ToolName: secretCredToolName,
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			"TOKEN_EXCHANGE_CLIENT_ID":     fmt.Sprintf("%s:%s", req.Namespace, clientID),
			"TOKEN_EXCHANGE_CLIENT_SECRET": clientSecret,
			"AUDIT_LOG_TOKEN":              auditLogToken,
		},
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	oauthClient := v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:       clientID,
			Namespace:  req.Namespace,
			Finalizers: []string{v1.OAuthClientFinalizer},
		},
		Spec: v1.OAuthClientSpec{
			Manifest: types.OAuthClientManifest{
				GrantTypes: []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
			},
			ClientSecretHash: hashedClientSecretHash,
			MCPServerName:    systemServer.Name,
		},
	}

	if err := req.Client.Create(req.Ctx, &oauthClient); err != nil {
		return fmt.Errorf("failed to create OAuth client: %w", err)
	}

	systemServer.Status.AuditLogTokenHash = hash.Digest(auditLogToken)

	return nil
}

// EnsureDeployment automatically deploys the server if Enabled=true and fully configured
func (h *Handler) EnsureDeployment(req router.Request, _ router.Response) error {
	systemServer := req.Object.(*v1.SystemMCPServer)

	log.Infof("EnsureDeployment called for system MCP server %s (enabled=%v, runtime=%s)",
		systemServer.Name, systemServer.Spec.Manifest.Enabled, systemServer.Spec.Manifest.Runtime)

	// Check if server should be deployed
	if !systemServer.Spec.Manifest.Enabled {
		log.Infof("System MCP server %s is disabled, shutting down any existing deployment", systemServer.Name)
		// Server is disabled, ensure any existing deployment is removed
		err := h.mcpSessionManager.ShutdownServer(req.Ctx, systemServer.Name)
		if err != nil {
			return fmt.Errorf("failed to shutdown disabled system MCP server: %w", err)
		}
		return nil
	}

	// Check if server is fully configured
	if !isSystemServerConfigured(req.Ctx, h.gptClient, *systemServer) {
		log.Infof("System MCP server %s is not fully configured, shutting down any existing deployment", systemServer.Name)
		// Server is not fully configured, ensure any existing deployment is removed
		err := h.mcpSessionManager.ShutdownServer(req.Ctx, systemServer.Name)
		if err != nil {
			return fmt.Errorf("failed to shutdown unconfigured system MCP server: %w", err)
		}
		return nil
	}

	// Get credentials for deployment
	credCtx := systemServer.Name
	creds, err := h.gptClient.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	secretToolName := secretInfoToolName(systemServer.Name)
	credEnv := make(map[string]string)
	for _, cred := range creds {
		// Skip the secret info credential — those vars go to the shim only, not the MCP server.
		if cred.ToolName == secretToolName {
			continue
		}
		// Get credential details
		credDetail, err := h.gptClient.RevealCredential(req.Ctx, []string{credCtx}, cred.ToolName)
		if err != nil {
			continue
		}
		for k, v := range credDetail.Env {
			credEnv[k] = v
		}
	}

	// Retrieve the token exchange credential
	var (
		tokenExchangeCred gptscript.Credential
		tokenCredErr      error
	)
	if err = retry.OnError(kwait.Backoff{
		Steps:    10,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
	}, func(err error) bool {
		return errors.As(err, &gptscript.ErrNotFound{})
	}, func() error {
		tokenExchangeCred, tokenCredErr = h.gptClient.RevealCredential(req.Ctx, []string{systemServer.Name}, secretToolName)
		return tokenCredErr
	}); err != nil {
		return fmt.Errorf("failed to find token exchange credential: %w", tokenCredErr)
	}

	secretsCred := tokenExchangeCred.Env

	audiences := systemServer.ValidConnectURLs(h.serverURL)

	// Transform to ServerConfig
	serverConfig, missingRequired, err := mcp.SystemServerToServerConfig(*systemServer, audiences, h.serverURL, credEnv, secretsCred)
	if err != nil {
		return fmt.Errorf("failed to transform system server to config: %w", err)
	}

	if len(missingRequired) > 0 {
		log.Infof("System MCP server %s still has missing required configuration: %v",
			systemServer.Name, missingRequired)
		// Still missing required configuration
		return nil
	}

	log.Infof("Launching system MCP server %s (runtime=%s, image=%s)",
		systemServer.Name, serverConfig.Runtime, serverConfig.ContainerImage)

	// Deploy the system server via backend
	// System servers don't use webhooks, so pass nil
	_, err = h.mcpSessionManager.LaunchServer(req.Ctx, serverConfig)
	if err != nil {
		return fmt.Errorf("failed to deploy system MCP server: %w", err)
	}

	log.Infof("System MCP server %s launched successfully", systemServer.Name)

	return nil
}

// CleanupDeployment handles cleanup when SystemMCPServer is deleted
func (h *Handler) CleanupDeployment(req router.Request, _ router.Response) error {
	systemServer := req.Object.(*v1.SystemMCPServer)

	// Shutdown deployment via backend
	// The backend's shutdownServer will remove the deployment (Docker container or K8s deployment)
	err := h.mcpSessionManager.ShutdownServer(req.Ctx, systemServer.Name)
	if err != nil {
		return fmt.Errorf("failed to shutdown system MCP server: %w", err)
	}

	return nil
}

// isSystemServerConfigured checks if all required configuration is present
func isSystemServerConfigured(ctx context.Context, gptClient *gptscript.GPTScript, server v1.SystemMCPServer) bool {
	// Check if all required env vars are configured
	credCtx := server.Name
	creds, err := gptClient.ListCredentials(ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		log.Infof("Failed to list credentials for system MCP server %s configuration check: %v",
			server.Name, err)
		return false
	}

	secretToolName := secretInfoToolName(server.Name)
	credEnv := make(map[string]string)
	for _, cred := range creds {
		if cred.ToolName == secretToolName {
			continue
		}
		credDetail, err := gptClient.RevealCredential(ctx, []string{credCtx}, cred.ToolName)
		if err != nil {
			continue
		}
		for k, v := range credDetail.Env {
			credEnv[k] = v
		}
	}

	for _, env := range server.Spec.Manifest.Env {
		if env.Required && env.Value == "" && credEnv[env.Key] == "" {
			log.Infof("System MCP server %s missing required env var %s",
				server.Name, env.Key)
			return false
		}
	}

	return true
}

func secretInfoToolName(serverName string) string {
	return serverName + "-secret-info"
}

// SecretInfoToolName returns the credential toolName used to store token exchange secrets
// for the given system MCP server. Exported for use by API handlers.
func SecretInfoToolName(serverName string) string {
	return secretInfoToolName(serverName)
}
