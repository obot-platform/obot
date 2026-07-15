package nanobotagent

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/backend"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/controller/handlers/provider"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	sigsyaml "sigs.k8s.io/yaml"
)

const (
	nanobotTokenTTL      = 12 * time.Hour
	nanobotRefreshBefore = 2 * time.Hour
)

var log = logger.Package()

type Handler struct {
	gatewayClient      *client.Client
	localK8SBackend    backend.Backend
	nanobotImage       string
	serverURL          string
	mcpServerNamespace string
}

func New(gatewayClient *client.Client, localK8sRouter *router.Router, nanobotImage, serverURL, mcpServerNamespace string, mcpSessionManager *mcp.SessionManager) *Handler {
	var localK8SBackend backend.Backend
	if localK8sRouter != nil {
		localK8SBackend = localK8sRouter.Backend()
	}
	return &Handler{
		gatewayClient:      gatewayClient,
		localK8SBackend:    localK8SBackend,
		nanobotImage:       nanobotImage,
		serverURL:          mcpSessionManager.TransformObotHostname(serverURL),
		mcpServerNamespace: mcpServerNamespace,
	}
}

func (h *Handler) EnsureMCPServer(req router.Request, resp router.Response) error {
	agent := req.Object.(*v1.NanobotAgent)

	mcpServerName := system.MCPServerPrefix + agent.Name

	expectedArgs := []string{"run", "--state", ".nanobot/state/nanobot.db", "--config", ".nanobot/", "--config", "${NANOBOT_CONFIG_FILE}"}
	if agent.Spec.DefaultAgent != "" {
		expectedArgs = append(expectedArgs, "--agent", agent.Spec.DefaultAgent)
	}

	// Check if MCPServer already exists
	var existing v1.MCPServer
	err := req.Get(&existing, agent.Namespace, mcpServerName)
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

		if existing.Spec.Manifest.ContainerizedConfig.HealthzPath != "/healthz" {
			existing.Spec.Manifest.ContainerizedConfig.HealthzPath = "/healthz"
			needsUpdate = true
		}

		expectedEnv := []types.MCPEnv{
			{
				MCPHeader: types.MCPHeader{
					Name:        "NANOBOT_ENV_FILE",
					Description: "Environment variables file for Nanobot",
					Key:         "NANOBOT_ENV_FILE",
					Sensitive:   true,
					Required:    true,
				},
				File:        true,
				DynamicFile: true,
			},
			{
				MCPHeader: types.MCPHeader{
					Name:        "NANOBOT_CONFIG_FILE",
					Description: "Provider config YAML for Nanobot",
					Key:         "NANOBOT_CONFIG_FILE",
					Sensitive:   true,
					Required:    true,
				},
				File:        true,
				DynamicFile: true,
			},
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

		if !slices.Equal(existing.Spec.Manifest.Env, expectedEnv) {
			needsUpdate = true
		}

		if needsUpdate {
			log.Debugf("Updating nanobot MCP server config: agent=%s mcpServer=%s", agent.Name, mcpServerName)
			existing.Spec.Manifest.ContainerizedConfig.Args = expectedArgs
			existing.Spec.Manifest.Env = expectedEnv
			if err := req.Client.Update(req.Ctx, &existing); err != nil {
				return fmt.Errorf("failed to update MCPServer: %w", err)
			}
		}

		// Ensure credentials are up to date
		if err := h.ensureCredentials(req.Ctx, req, resp, agent, mcpServerName); err != nil {
			return fmt.Errorf("failed to ensure credentials: %w", err)
		}

		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check for existing MCPServer: %w", err)
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
					Image:       h.nanobotImage,
					Command:     "nanobot",
					Args:        expectedArgs,
					Port:        8080,
					Path:        "/mcp",
					HealthzPath: "/healthz",
				},
				Env: []types.MCPEnv{
					{
						MCPHeader: types.MCPHeader{
							Name:        "NANOBOT_ENV_FILE",
							Description: "Environment variables file for Nanobot",
							Key:         "NANOBOT_ENV_FILE",
							Sensitive:   true,
							Required:    true,
						},
						File:        true,
						DynamicFile: true,
					},
					{
						MCPHeader: types.MCPHeader{
							Name:        "NANOBOT_CONFIG_FILE",
							Description: "Provider config YAML for Nanobot",
							Key:         "NANOBOT_CONFIG_FILE",
							Sensitive:   true,
							Required:    true,
						},
						File:        true,
						DynamicFile: true,
					},
				},
			},
		},
	}

	if err := req.Client.Create(req.Ctx, mcpServer); err != nil {
		return fmt.Errorf("failed to create MCPServer: %w", err)
	}
	log.Infof("Created nanobot agent MCP server: agent=%s mcpServer=%s", agent.Name, mcpServerName)

	// Create credentials for the new server
	if err := h.ensureCredentials(req.Ctx, req, resp, agent, mcpServerName); err != nil {
		return fmt.Errorf("failed to create credentials: %w", err)
	}

	return nil
}

// ensureCredentials ensures that the MCP server has credentials with API keys that are valid
// and refreshes them when they are close to expiration.
func (h *Handler) ensureCredentials(ctx context.Context, req router.Request, resp router.Response, agent *v1.NanobotAgent, mcpServerName string) error {
	credCtx := fmt.Sprintf("%s-%s", agent.Spec.UserID, mcpServerName)

	llmModel, err := resolveModel(ctx, req.Client, req.Namespace, types.DefaultModelAliasTypeLLM)
	if err != nil {
		return err
	}
	llmProvider, llmDefault := h.parseModelProvider(llmModel)

	miniModel, err := resolveModel(ctx, req.Client, req.Namespace, types.DefaultModelAliasTypeLLMMini)
	if err != nil {
		return err
	}
	miniProvider, miniDefault := h.parseModelProvider(miniModel)

	providerYAML, err := buildNanobotProviderConfigYAML(llmProvider, miniProvider)
	if err != nil {
		return fmt.Errorf("failed to build nanobot provider config: %w", err)
	}

	// Check if credential exists and if the token needs refreshing
	var needsRefresh bool
	cred, err := h.gatewayClient.RevealCredential(ctx, []string{credCtx}, mcpServerName)
	// Parse the env file before checking the error.
	credEnvFileVars := parseEnvFile(cred.Secrets["NANOBOT_ENV_FILE"])
	if err != nil {
		if _, ok := errors.AsType[client.CredentialNotFoundError](err); !ok {
			return fmt.Errorf("failed to reveal credential: %w", err)
		}
		// Credential doesn't exist, needs to be created
		needsRefresh = true
		log.Debugf("Nanobot credential missing, creating: agent=%s mcpServer=%s", agent.Name, mcpServerName)
	} else {
		// Credential exists, check if token needs refreshing.
		// Use the configured provider's API key env var to find the token.
		llmEnvVarName := strings.TrimSuffix(strings.TrimPrefix(llmProvider.APIKey, "${"), "}")
		token := credEnvFileVars[llmEnvVarName]
		if token != "" {
			apiKey, err := h.gatewayClient.ValidateAPIKey(ctx, token)
			if err != nil {
				// Token is invalid, needs refresh
				needsRefresh = true
				log.Debugf("Nanobot credential token invalid, refreshing: agent=%s mcpServer=%s", agent.Name, mcpServerName)
			} else if apiKey.ExpiresAt != nil {
				if untilRefresh := time.Until(*apiKey.ExpiresAt) - nanobotRefreshBefore; untilRefresh <= 0 {
					// If the token expires soon, then refresh it
					needsRefresh = true
					resp.RetryAfter(time.Second)
					log.Debugf("Nanobot credential due for refresh: agent=%s mcpServer=%s expiresAt=%s", agent.Name, mcpServerName, apiKey.ExpiresAt.UTC().Format(time.RFC3339))
				} else {
					// Otherwise, look at the agent again around the time the refresh would be needed.
					resp.RetryAfter(untilRefresh)
				}
			} else {
				needsRefresh = true
				log.Debugf("API key set for no expiration, refreshing: agent=%s mcpServer=%s", agent.Name, mcpServerName)
			}
		} else {
			// No token in credential, needs refresh
			needsRefresh = true
			log.Debugf("Nanobot credential missing token, refreshing: agent=%s mcpServer=%s", agent.Name, mcpServerName)
		}
	}

	if !needsRefresh &&
		credEnvFileVars["NANOBOT_DEFAULT_MODEL"] == llmDefault &&
		credEnvFileVars["NANOBOT_DEFAULT_MINI_MODEL"] == miniDefault &&
		cred.Secrets["NANOBOT_CONFIG_FILE"] == providerYAML {
		// Credentials are up to date
		return nil
	}

	log.Debugf("Refreshing nanobot credentials: agent=%s mcpServer=%s model=%s miniModel=%s", agent.Name, mcpServerName, llmDefault, miniDefault)

	// Look up the gateway user to get the uint ID needed for API key creation
	gatewayUser, err := h.gatewayClient.UserByID(ctx, agent.Spec.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Delete old API key if present.
	// We're not deleting the key the container is currently using because it may take a few minutes for the volume
	// to update with the new credentials. We delete the previously used key instead to ensure that we don't leave orphaned keys around.
	apiKeyIDStr := credEnvFileVars["MCP_API_KEY_ID_PREV"]
	if apiKeyIDStr == "" {
		// Backward compatibility while migrating old credentials.
		apiKeyIDStr = cred.Secrets["MCP_API_KEY_ID"]
	}
	if apiKeyIDStr != "" {
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
		new(time.Now().Add(nanobotTokenTTL)),
		gatewaytypes.APIKeyScopes{
			MCPServerIDs:                []string{"*"},
			CanAccessSkills:             true,
			CanAccessLLMProxy:           true,
			CanAccessPublishedArtifacts: true,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	envFileLines := []string{
		fmt.Sprintf("OBOT_URL=%s", h.serverURL),
		fmt.Sprintf("MCP_API_KEY=%s", apiKeyResp.Key),
		fmt.Sprintf("MCP_API_KEY_ID=%d", apiKeyResp.ID),
		fmt.Sprintf("MCP_API_KEY_ID_PREV=%s", credEnvFileVars["MCP_API_KEY_ID"]),
		fmt.Sprintf("MCP_SERVER_SEARCH_URL=%s", system.MCPConnectURL(h.serverURL, system.ObotMCPServerName)),
		fmt.Sprintf("MCP_SERVER_SEARCH_API_KEY=%s", apiKeyResp.Key),
		fmt.Sprintf("NANOBOT_DEFAULT_MODEL=%s", llmDefault),
		fmt.Sprintf("NANOBOT_DEFAULT_MINI_MODEL=%s", miniDefault),
	}
	seenProviders := make(map[string]struct{}, 2)
	for _, p := range []nanobotLLMProvider{llmProvider, miniProvider} {
		if _, ok := seenProviders[p.Name]; ok {
			continue
		}
		seenProviders[p.Name] = struct{}{}
		envVarName := strings.TrimSuffix(strings.TrimPrefix(p.APIKey, "${"), "}")
		envFileLines = append(envFileLines, fmt.Sprintf("%s=%s", envVarName, apiKeyResp.Key))
	}

	// Create or update the credential with the new token, API key, and provider config.
	if err := h.gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: credCtx,
		Name:    mcpServerName,
		Secrets: map[string]string{
			"NANOBOT_ENV_FILE":    strings.Join(envFileLines, "\n"),
			"NANOBOT_CONFIG_FILE": providerYAML,
		},
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	if h.localK8SBackend != nil {
		// If local Kubernetes backend is available, trigger a sync to update the secret with the new credentials
		triggerKey := fmt.Sprintf("%s/%s", h.mcpServerNamespace, name.SafeConcatName(mcpServerName, "mcp", "files"))
		log.Debugf("Triggering local k8s secret sync: agent=%s mcpServer=%s key=%s", agent.Name, mcpServerName, triggerKey)
		if err := h.localK8SBackend.Trigger(
			ctx,
			corev1.SchemeGroupVersion.WithKind("Secret"),
			triggerKey,
			time.Second,
		); err != nil {
			return fmt.Errorf("failed to trigger local Kubernetes sync: %w", err)
		}
	}
	log.Infof("Nanobot credentials refreshed: agent=%s mcpServer=%s apiKeyID=%d", agent.Name, mcpServerName, apiKeyResp.ID)
	return nil
}

// resolvedLLMModel pairs the resolved model resource name with its configured provider reference
// and the dialect declared by that provider (if any).
type resolvedLLMModel struct {
	Name            string               // Kubernetes Model resource name
	ModelProvider   string               // e.g. "openai-model-provider", "anthropic-model-provider"
	ProviderDialect nanobottypes.Dialect // from resolved model manifest dialect or ProviderMeta.Dialect; empty if not declared
}

// nanobotLLMProvider describes how a single LLM provider should be configured in nanobot's YAML.
type nanobotLLMProvider struct {
	Name    string // key in llmProviders map (e.g. "openai", "anthropic")
	Dialect nanobottypes.Dialect
	APIKey  string // env var reference derived from Name, e.g. "${OPENAI_MODEL_PROVIDER_API_KEY}"
	BaseURL string // actual Obot proxy URL
}

// parseModelProvider returns the nanobot provider config and the fully-qualified
// model name (provider/model) for a resolved model.
//
// If the provider has declared a dialect via ProviderMeta.Dialect, that dialect
// is used and the base URL is derived from it. Otherwise the known built-in
// providers (openai, anthropic) supply both; everything else falls back to
// OpenResponses via the generic /api/llm-proxy dispatch.
func (h *Handler) parseModelProvider(model resolvedLLMModel) (nanobotLLMProvider, string) {
	name := model.ModelProvider
	envVarName := strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_API_KEY"

	dialect := model.ProviderDialect
	if dialect == "" {
		// No declared dialect — fall back to per-provider defaults.
		switch model.ModelProvider {
		case system.AnthropicModelProvider:
			dialect = nanobottypes.DialectAnthropicMessages
		case system.OpenAIModelProvider:
			dialect = nanobottypes.DialectOpenAIResponses
		default:
			dialect = nanobottypes.DialectOpenResponses
		}
	}

	baseURL := h.serverURL + "/api/llm-proxy"

	switch model.ModelProvider {
	case system.AmazonBedrockModelProvider:
		baseURL += "/aws-bedrock/v1"
	case system.AmazonBedrockAPIKeyModelProvider:
		baseURL += "/aws-bedrock-api-key/v1"
	default:
		switch dialect {
		case nanobottypes.DialectAnthropicMessages:
			baseURL += "/anthropic/v1"
		case nanobottypes.DialectOpenAIResponses:
			baseURL += "/openai/v1"
		case nanobottypes.DialectBifrostRequest:
			fallthrough // same as default
		default:
			baseURL = h.serverURL + "/api/llm-proxy"
		}
	}

	p := nanobotLLMProvider{
		Name:    name,
		Dialect: dialect,
		APIKey:  fmt.Sprintf("${%s}", envVarName),
		BaseURL: baseURL,
	}
	return p, fmt.Sprintf("%s/%s", p.Name, model.Name)
}

// buildNanobotProviderConfigYAML generates a nanobot Config YAML containing only the
// providers required by the given LLM and mini-LLM models.
func buildNanobotProviderConfigYAML(providers ...nanobotLLMProvider) (string, error) {
	llmProviders := make(map[string]nanobottypes.LLMProvider, len(providers))
	for _, p := range providers {
		if _, exists := llmProviders[p.Name]; exists {
			continue
		}
		llmProviders[p.Name] = nanobottypes.LLMProvider{
			Dialect: p.Dialect,
			APIKey:  p.APIKey,
			BaseURL: p.BaseURL,
		}
	}
	data, err := sigsyaml.Marshal(nanobottypes.Config{LLMProviders: llmProviders})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func getModelForAlias(ctx context.Context, client kclient.Client, namespace string, aliasName types.DefaultModelAliasType) (resolvedLLMModel, error) {
	llmModel, err := alias.GetFromScope(ctx, client, "Model", namespace, string(aliasName))
	if err != nil {
		return resolvedLLMModel{}, fmt.Errorf("failed to get default model alias %v: %w", aliasName, err)
	}

	modelAlias, ok := llmModel.(*v1.DefaultModelAlias)
	if !ok {
		return resolvedLLMModel{}, fmt.Errorf("alias %v is not of type Alias", aliasName)
	}

	var model v1.Model
	if err := alias.Get(ctx, client, &model, namespace, modelAlias.Spec.Manifest.Model); err != nil {
		return resolvedLLMModel{}, err
	}

	return resolvedLLMModel{
		Name:            model.Name,
		ModelProvider:   model.Spec.Manifest.ModelProvider,
		ProviderDialect: nanobottypes.Dialect(model.Spec.Manifest.Dialect),
	}, nil
}

// resolveModel returns a resolved model and its provider for a default alias.
//
// It prefers an explicitly configured alias target when one exists. If the
// alias is unset or cannot be resolved, it falls back to active LLM models in
// the namespace by first checking a short ordered list of preferred model names
// for that alias. The llm-mini alias falls back to the resolved llm model when
// no preferred mini model is available. All other aliases fall back to the
// first active LLM model available.
func resolveModel(ctx context.Context, client kclient.Client, namespace string, aliasName types.DefaultModelAliasType) (resolvedLLMModel, error) {
	if model, err := getModelForAlias(ctx, client, namespace, aliasName); err == nil {
		return model, nil
	}

	models, err := listActiveLLMModels(ctx, client, namespace)
	if err != nil {
		return resolvedLLMModel{}, err
	}

	return chooseModel(ctx, client, namespace, models, aliasName)
}

func listActiveLLMModels(ctx context.Context, client kclient.Client, namespace string) ([]v1.Model, error) {
	var models v1.ModelList
	if err := client.List(ctx, &models, &kclient.ListOptions{Namespace: namespace}); err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	result := make([]v1.Model, 0, len(models.Items))
	for _, model := range models.Items {
		if !model.Spec.Manifest.Active || model.Spec.Manifest.Usage != types.ModelUsageLLM {
			continue
		}
		if strings.TrimSpace(model.Spec.Manifest.TargetModel) == "" {
			continue
		}
		result = append(result, model)
	}

	slices.SortFunc(result, func(a, b v1.Model) int {
		return cmp.Or(
			cmp.Compare(a.Spec.Manifest.TargetModel, b.Spec.Manifest.TargetModel),
			cmp.Compare(a.Spec.Manifest.Name, b.Spec.Manifest.Name),
		)
	})

	return result, nil
}

func chooseModel(ctx context.Context, client kclient.Client, namespace string, models []v1.Model, aliasName types.DefaultModelAliasType) (resolvedLLMModel, error) {
	preferred := preferredModelsForAlias(aliasName)
	for _, preferredName := range preferred {
		for _, model := range models {
			if model.Spec.Manifest.TargetModel == preferredName || model.Spec.Manifest.Name == preferredName {
				return resolvedLLMModel{
					Name:            model.Name,
					ModelProvider:   model.Spec.Manifest.ModelProvider,
					ProviderDialect: nanobottypes.Dialect(model.Spec.Manifest.Dialect),
				}, nil
			}
		}
	}

	if aliasName == types.DefaultModelAliasTypeLLMMini {
		return resolveModel(ctx, client, namespace, types.DefaultModelAliasTypeLLM)
	}

	if len(models) > 0 {
		return resolvedLLMModel{
			Name:            models[0].Name,
			ModelProvider:   models[0].Spec.Manifest.ModelProvider,
			ProviderDialect: nanobottypes.Dialect(models[0].Spec.Manifest.Dialect),
		}, nil
	}

	return resolvedLLMModel{}, fmt.Errorf("failed to resolve default model for alias %s: no active llm models available", aliasName)
}

func preferredModelsForAlias(aliasName types.DefaultModelAliasType) []string {
	preferred := make([]string, 0, 2)
	for _, defaults := range []map[types.DefaultModelAliasType]string{
		provider.OpenAIDefaultModelAliases(),
		provider.AnthropicDefaultModelAliases(),
	} {
		if model := strings.TrimSpace(defaults[aliasName]); model != "" {
			preferred = append(preferred, model)
		}
	}
	return preferred
}

// deleteTokens deletes the API key and MCP token associated with the MCP server.
func (h *Handler) deleteTokens(ctx context.Context, agent *v1.NanobotAgent, mcpServerName string) error {
	credCtx := fmt.Sprintf("%s-%s", agent.Spec.UserID, mcpServerName)

	// Retrieve the credential to get the API key ID
	cred, err := h.gatewayClient.RevealCredential(ctx, []string{credCtx}, mcpServerName)
	if err != nil {
		if errors.As(err, &client.CredentialNotFoundError{}) {
			// Credential doesn't exist, nothing to delete
			return nil
		}
		return fmt.Errorf("failed to reveal credential: %w", err)
	}

	// Extract and delete the API key if present

	if apiKeyIDStr := parseEnvFile(cred.Secrets["NANOBOT_ENV_FILE"])["MCP_API_KEY_ID"]; apiKeyIDStr != "" {
		apiKeyID, err := strconv.ParseUint(apiKeyIDStr, 10, 32)
		if err != nil {
			return fmt.Errorf("failed to parse API key ID: %w", err)
		}

		// Look up the gateway user to get the uint ID needed for API key deletion
		gatewayUser, err := h.gatewayClient.UserByIDIncludeDeleted(ctx, agent.Spec.UserID)
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

func parseEnvFile(content string) map[string]string {
	result := map[string]string{}
	for line := range strings.SplitSeq(content, "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			result[key] = value
		}
	}

	return result
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
