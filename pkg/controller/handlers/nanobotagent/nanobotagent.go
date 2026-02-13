package nanobotagent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Handler struct {
	gptClient     *gptscript.GPTScript
	tokenService  *persistent.TokenService
	gatewayClient *client.Client
	nanobotImage  string
	serverURL     string
}

func New(gptClient *gptscript.GPTScript, tokenService *persistent.TokenService, gatewayClient *client.Client, nanobotImage, serverURL string, mcpSessionManager *mcp.SessionManager) *Handler {
	return &Handler{
		gptClient:     gptClient,
		tokenService:  tokenService,
		gatewayClient: gatewayClient,
		// For now, this is hardcoded to the main tag, but we will switch this out before release.
		// TODO(thedadams): Change this to the nanobotImage prior to release.
		nanobotImage: "ghcr.io/nanobot-ai/nanobot:main",
		serverURL:    mcpSessionManager.TransformObotHostname(serverURL),
	}
}

func (h *Handler) CreateMCPServer(req router.Request, resp router.Response) error {
	agent := req.Object.(*v1.NanobotAgent)

	mcpServerName := system.MCPServerPrefix + agent.Name

	// Check if MCPServer already exists
	existing := &v1.MCPServer{}
	err := req.Get(existing, agent.Namespace, mcpServerName)
	if err == nil {
		// MCP Server already exists, update it if needed
		var needsUpdate bool

		// Check if display name changed
		if existing.Spec.Manifest.ShortDescription != agent.Spec.DisplayName {
			existing.Spec.Manifest.ShortDescription = agent.Spec.DisplayName
			needsUpdate = true
		}

		// Check if description changed
		if existing.Spec.Manifest.Description != agent.Spec.Description {
			existing.Spec.Manifest.Description = agent.Spec.Description
			needsUpdate = true
		}

		// Check the image
		if existing.Spec.Manifest.ContainerizedConfig.Image != h.nanobotImage {
			existing.Spec.Manifest.ContainerizedConfig.Image = h.nanobotImage
			needsUpdate = true
		}

		// Check if default agent changed
		expectedArgs := []string{"run", "--state", ".nanobot/state/nanobot.db"}
		if agent.Spec.DefaultAgent != "" {
			expectedArgs = append(expectedArgs, "--agent", agent.Spec.DefaultAgent)
		}

		currentArgs := existing.Spec.Manifest.ContainerizedConfig.Args
		if len(currentArgs) != len(expectedArgs) {
			needsUpdate = true
		} else {
			for i, arg := range expectedArgs {
				if currentArgs[i] != arg {
					needsUpdate = true
					break
				}
			}
		}

		if needsUpdate {
			existing.Spec.Manifest.ContainerizedConfig.Args = expectedArgs
			if err := req.Client.Update(req.Ctx, existing); err != nil {
				return fmt.Errorf("failed to update MCPServer: %w", err)
			}
		}

		// Ensure credentials are up to date
		if err := h.ensureCredentials(req.Ctx, agent, mcpServerName, resp); err != nil {
			return fmt.Errorf("failed to ensure credentials: %w", err)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check for existing MCPServer: %w", err)
	}

	// Create new MCPServer
	args := []string{"run"}
	if agent.Spec.DefaultAgent != "" {
		args = append(args, "--agent", agent.Spec.DefaultAgent)
	}

	mcpServer := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServerName,
			Namespace: agent.Namespace,
		},
		Spec: v1.MCPServerSpec{
			UserID:         agent.Spec.UserID,
			NanobotAgentID: agent.Name,
			Manifest: types.MCPServerManifest{
				Name:             agent.Name,
				ShortDescription: agent.Spec.DisplayName,
				Description:      agent.Spec.Description,
				Runtime:          types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image:   h.nanobotImage,
					Command: "nanobot",
					Args:    args,
					Port:    8080,
					Path:    "/mcp?ui=true",
				},
				Env: []types.MCPEnv{
					{
						MCPHeader: types.MCPHeader{
							Name:        "ANTHROPIC_BASE_URL",
							Description: "Base URL for Anthropic API proxy",
							Key:         "ANTHROPIC_BASE_URL",
							Sensitive:   false,
							Required:    true,
						},
					},
					{
						MCPHeader: types.MCPHeader{
							Name:        "OPENAI_BASE_URL",
							Description: "Base URL for OpenAI API proxy",
							Key:         "OPENAI_BASE_URL",
							Sensitive:   false,
							Required:    true,
						},
					},
					{
						MCPHeader: types.MCPHeader{
							Name:        "ANTHROPIC_API_KEY",
							Description: "API key for Anthropic proxy authentication",
							Key:         "ANTHROPIC_API_KEY",
							Sensitive:   true,
							Required:    true,
						},
					},
					{
						MCPHeader: types.MCPHeader{
							Name:        "OPENAI_API_KEY",
							Description: "API key for OpenAI proxy authentication",
							Key:         "OPENAI_API_KEY",
							Sensitive:   true,
							Required:    true,
						},
					},
					{
						MCPHeader: types.MCPHeader{
							Name:        "MCP_API_KEY",
							Description: "API key for MCP server access",
							Key:         "MCP_API_KEY",
							Sensitive:   true,
							Required:    true,
						},
					},
					{
						MCPHeader: types.MCPHeader{
							Name:        "MCP_SERVER_SEARCH_URL",
							Description: "URL for MCP server search",
							Key:         "MCP_SERVER_SEARCH_URL",
							Sensitive:   false,
							Required:    true,
						},
					},
					{
						MCPHeader: types.MCPHeader{
							Name:        "MCP_SERVER_SEARCH_API_KEY",
							Description: "API key for MCP server search",
							Key:         "MCP_SERVER_SEARCH_API_KEY",
							Sensitive:   true,
							Required:    true,
						},
					},
				},
			},
		},
	}

	if err := req.Client.Create(req.Ctx, mcpServer); err != nil {
		return fmt.Errorf("failed to create MCPServer: %w", err)
	}

	// Create credentials for the new server
	if err := h.ensureCredentials(req.Ctx, agent, mcpServerName, resp); err != nil {
		return fmt.Errorf("failed to create credentials: %w", err)
	}

	return nil
}

// ensureCredentials ensures that the MCP server has credentials with API keys that are valid
// and refreshes them if they expire within 2 hours.
func (h *Handler) ensureCredentials(ctx context.Context, agent *v1.NanobotAgent, mcpServerName string, resp router.Response) error {
	credCtx := fmt.Sprintf("%s-%s", agent.Spec.UserID, mcpServerName)

	// Check if credential exists and if the token needs refreshing
	var needsRefresh bool
	cred, err := h.gptClient.RevealCredential(ctx, []string{credCtx}, mcpServerName)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to reveal credential: %w", err)
		}
		// Credential doesn't exist, needs to be created
		needsRefresh = true
	} else {
		// Credential exists, check if token needs refreshing
		token := cred.Env["OPENAI_API_KEY"]
		if token != "" {
			tokenCtx, err := h.tokenService.DecodeToken(ctx, token)
			if err != nil {
				// Token is invalid, needs refresh
				needsRefresh = true
			} else {
				if untilRefresh := time.Until(tokenCtx.ExpiresAt) - 2*time.Hour; untilRefresh <= 0 {
					// If the token expires in the next 2 hours, then refresh it
					needsRefresh = true
				} else {
					// Otherwise, look at the agent again around the time the refresh would be needed.
					resp.RetryAfter(untilRefresh)
				}
			}
		} else {
			// No token in credential, needs refresh
			needsRefresh = true
		}
	}

	searchServerURL := system.MCPConnectURL(h.serverURL, system.ObotMCPServerName)
	if !needsRefresh && cred.Env["OBOT_URL"] == h.serverURL && cred.Env["MCP_SERVER_SEARCH_URL"] == searchServerURL {
		// Credentials are up to date
		return nil
	}

	// Generate a new token that expires in 12 hours
	now := time.Now()
	expiresAt := now.Add(12 * time.Hour)
	token, err := h.tokenService.NewToken(ctx, persistent.TokenContext{
		Audience:   h.serverURL,
		IssuedAt:   now,
		ExpiresAt:  expiresAt,
		UserID:     agent.Spec.UserID,
		UserGroups: types.RoleBasic.Groups(),
		Namespace:  agent.Namespace,
	})
	if err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}

	// Look up the gateway user to get the uint ID needed for API key creation
	gatewayUser, err := h.gatewayClient.UserByID(ctx, agent.Spec.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Delete old API key if present
	// Extract old API key ID if present for cleanup
	if apiKeyIDStr := cred.Env["MCP_API_KEY_ID"]; apiKeyIDStr != "" {
		if id, err := strconv.ParseUint(apiKeyIDStr, 10, 32); err == nil {
			if err = h.gatewayClient.DeleteAPIKey(ctx, gatewayUser.ID, uint(id)); err != nil {
				return fmt.Errorf("failed to delete old API key: %w", err)
			}
		}
	}

	// Create a new API key with 12-hour expiration and access to all servers
	apiKeyResp, err := h.gatewayClient.CreateAPIKey(
		ctx,
		gatewayUser.ID,
		fmt.Sprintf("nanobot-agent-%s", mcpServerName),
		fmt.Sprintf("API key for nanobot agent %s", agent.Name),
		&expiresAt,
		[]string{"*"}, // Access to all servers
	)
	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	// Create or update the credential with the new token and API key
	if err := h.gptClient.CreateCredential(ctx, gptscript.Credential{
		Context:  credCtx,
		ToolName: mcpServerName,
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			"OBOT_URL":                  h.serverURL,
			"ANTHROPIC_BASE_URL":        fmt.Sprintf("%s/api/llm-proxy/anthropic", h.serverURL),
			"OPENAI_BASE_URL":           fmt.Sprintf("%s/api/llm-proxy/openai", h.serverURL),
			"ANTHROPIC_API_KEY":         token,
			"OPENAI_API_KEY":            token,
			"MCP_API_KEY":               apiKeyResp.Key,
			"MCP_API_KEY_ID":            strconv.FormatUint(uint64(apiKeyResp.ID), 10),
			"MCP_SERVER_SEARCH_URL":     searchServerURL,
			"MCP_SERVER_SEARCH_API_KEY": apiKeyResp.Key,
		},
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	return nil
}

func (h *Handler) DeleteMCPServer(req router.Request, _ router.Response) error {
	agent := req.Object.(*v1.NanobotAgent)

	mcpServerName := system.MCPServerPrefix + agent.Name

	// Delete the MCPServer object
	var mcpServer v1.MCPServer
	err := req.Get(&mcpServer, agent.Namespace, mcpServerName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// MCPServer doesn't exist, nothing to delete
			return nil
		}
		return fmt.Errorf("failed to get MCPServer: %w", err)
	}

	// Delete associated tokens before deleting the server
	if err := h.deleteTokens(req.Ctx, agent, mcpServerName); err != nil {
		return fmt.Errorf("failed to delete tokens: %w", err)
	}

	// Delete the MCPServer object (credential will be automatically cleaned up)
	if err := req.Client.Delete(req.Ctx, &mcpServer); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete MCPServer: %w", err)
		}
	}

	return nil
}

// deleteTokens deletes the API key and MCP token associated with the MCP server.
func (h *Handler) deleteTokens(ctx context.Context, agent *v1.NanobotAgent, mcpServerName string) error {
	credCtx := fmt.Sprintf("%s-%s", agent.Spec.UserID, mcpServerName)

	// Retrieve the credential to get the API key ID
	cred, err := h.gptClient.RevealCredential(ctx, []string{credCtx}, mcpServerName)
	if err != nil {
		if errors.As(err, &gptscript.ErrNotFound{}) {
			// Credential doesn't exist, nothing to delete
			return nil
		}
		return fmt.Errorf("failed to reveal credential: %w", err)
	}

	// Extract and delete the API key if present
	if apiKeyIDStr := cred.Env["MCP_API_KEY_ID"]; apiKeyIDStr != "" {
		apiKeyID, err := strconv.ParseUint(apiKeyIDStr, 10, 32)
		if err != nil {
			return fmt.Errorf("failed to parse API key ID: %w", err)
		}

		// Look up the gateway user to get the uint ID needed for API key deletion
		gatewayUser, err := h.gatewayClient.UserByID(ctx, agent.Spec.UserID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}

		// Delete the API key
		if err := h.gatewayClient.DeleteAPIKey(ctx, gatewayUser.ID, uint(apiKeyID)); err != nil {
			return fmt.Errorf("failed to delete API key: %w", err)
		}
	}

	return nil
}

// Cleanup is a finalizer handler that cleans up tokens when a NanobotAgent is deleted.
func (h *Handler) Cleanup(req router.Request, _ router.Response) error {
	agent := req.Object.(*v1.NanobotAgent)
	mcpServerName := system.MCPServerPrefix + agent.Name

	// Delete associated tokens
	if err := h.deleteTokens(req.Ctx, agent, mcpServerName); err != nil {
		return fmt.Errorf("failed to delete tokens: %w", err)
	}

	return nil
}
