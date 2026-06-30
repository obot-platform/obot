package systemmcpserver

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"golang.org/x/crypto/bcrypt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kwait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Handler struct {
	gatewayClient     *gateway.Client
	mcpSessionManager *mcp.SessionManager
	serverURL         string
}

func New(gatewayClient *gateway.Client, mcpLoader *mcp.SessionManager, serverURL string) *Handler {
	return &Handler{
		gatewayClient:     gatewayClient,
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

	secretCredToolName := SecretInfoToolName(systemServer.Name)

	if systemServer.Status.AuditLogTokenHash != "" {
		cred, err := h.gatewayClient.RevealCredential(req.Ctx, []string{systemServer.Name}, secretCredToolName)
		if err != nil {
			return fmt.Errorf("failed to get credential: %w", err)
		}

		if systemServer.Status.AuditLogTokenHash != utils.Digest(cred.Secrets["AUDIT_LOG_TOKEN"]) {
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

	if err := h.gatewayClient.UpsertCredential(req.Ctx, gatewaytypes.Credential{
		Context: systemServer.Name,
		Name:    secretCredToolName,
		Secrets: map[string]string{
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

	systemServer.Status.AuditLogTokenHash = utils.Digest(auditLogToken)

	return nil
}

// EnsureDeployment automatically deploys the server if Enabled=true and fully configured
func (h *Handler) EnsureDeployment(req router.Request, _ router.Response) error {
	systemServer := req.Object.(*v1.SystemMCPServer)

	log.Infof("EnsureDeployment called for system MCP server %s (enabled=%v, runtime=%s)",
		systemServer.Name, systemServer.Spec.Manifest.Enabled, systemServer.Spec.Manifest.Runtime)

	// Check if server should be deployed
	if systemServer.Spec.Manifest.Enabled != nil && !*systemServer.Spec.Manifest.Enabled {
		log.Infof("System MCP server %s is disabled, shutting down any existing deployment", systemServer.Name)
		// Server is disabled, ensure any existing deployment is removed
		err := h.mcpSessionManager.ShutdownIdleServer(req.Ctx, systemServer.Name)
		if err != nil {
			return fmt.Errorf("failed to shutdown disabled system MCP server: %w", err)
		}
		return nil
	}

	// Check if server is fully configured
	if !IsSystemServerConfigured(req.Ctx, h.gatewayClient, *systemServer) {
		log.Infof("System MCP server %s is not fully configured, shutting down any existing deployment", systemServer.Name)
		// Server is not fully configured, ensure any existing deployment is removed
		err := h.mcpSessionManager.ShutdownIdleServer(req.Ctx, systemServer.Name)
		if err != nil {
			return fmt.Errorf("failed to shutdown unconfigured system MCP server: %w", err)
		}
		return nil
	}

	// Get credentials for deployment
	credCtx := systemServer.Name
	creds, err := h.gatewayClient.ListCredentials(req.Ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	secretToolName := SecretInfoToolName(systemServer.Name)
	credEnv := make(map[string]string)
	for _, cred := range creds {
		// Skip the secret info credential — those vars go to the shim only, not the MCP server.
		if cred.Name == secretToolName {
			continue
		}
		// Get credential details
		credDetail, err := h.gatewayClient.RevealCredential(req.Ctx, []string{credCtx}, cred.Name)
		if err != nil {
			continue
		}

		maps.Copy(credEnv, credDetail.Secrets)
	}

	// Retrieve the token exchange credential
	var (
		tokenExchangeCred gatewaytypes.Credential
		tokenCredErr      error
	)
	if err = retry.OnError(kwait.Backoff{
		Steps:    10,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
	}, func(err error) bool {
		return errors.As(err, &gateway.CredentialNotFoundError{})
	}, func() error {
		tokenExchangeCred, tokenCredErr = h.gatewayClient.RevealCredential(req.Ctx, []string{systemServer.Name}, secretToolName)
		return tokenCredErr
	}); err != nil {
		return fmt.Errorf("failed to find token exchange credential: %w", tokenCredErr)
	}

	secretsCred := tokenExchangeCred.Secrets

	audiences := systemServer.ValidConnectURLs(h.serverURL)

	// Transform to ServerConfig
	serverConfig, missingRequired, err := mcp.SystemServerToServerConfig(*systemServer, audiences, h.serverURL, "", credEnv, secretsCred)
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
	creds, err := h.gatewayClient.ListCredentials(req.Ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{systemServer.Name},
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials for %s system server cleanup: %w", systemServer.Name, err)
	}

	for _, cred := range creds {
		if _, err := h.gatewayClient.DeleteCredential(req.Ctx, cred.Context, cred.Name); err != nil {
			return fmt.Errorf("failed to delete credential %s: %w", cred.Name, err)
		}
	}

	// Shutdown deployment via backend
	// The backend's shutdownServer will remove the deployment (Docker container or K8s deployment)
	if err = h.mcpSessionManager.ShutdownServer(req.Ctx, systemServer.Name); err != nil {
		return fmt.Errorf("failed to shutdown system MCP server %s: %w", systemServer.Name, err)
	}

	return nil
}

// IsSystemServerConfigured checks if all required configuration is present
func IsSystemServerConfigured(ctx context.Context, gatewayClient *gateway.Client, server v1.SystemMCPServer) bool {
	credEnv, err := GetCredentialsForSystemServer(ctx, gatewayClient, server)
	if err != nil {
		log.Errorf("Failed to get credentials for system MCP server %s: %v", server.Name, err)
		return false
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

// GetCredentialsForSystemServer retrieves all credentials for the given system MCP server and returns them as a single map of env vars.
func GetCredentialsForSystemServer(ctx context.Context, gatewayClient *gateway.Client, server v1.SystemMCPServer) (map[string]string, error) {
	credCtx := server.Name
	creds, err := gatewayClient.ListCredentials(ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		return nil, err
	}

	secretToolName := SecretInfoToolName(server.Name)
	credEnv := make(map[string]string)
	for _, cred := range creds {
		// Skip the secret info credential — those vars go to the shim only, not the MCP server.
		if cred.Name == secretToolName {
			continue
		}
		credDetail, err := gatewayClient.RevealCredential(ctx, []string{credCtx}, cred.Name)
		if err != nil {
			continue
		}

		maps.Copy(credEnv, credDetail.Secrets)
	}

	return credEnv, nil
}

// SecretInfoToolName returns the credential toolName used to store token exchange secrets
// for the given system MCP server. Exported for use by API handlers.
func SecretInfoToolName(serverName string) string {
	return serverName + "-secret-info"
}
