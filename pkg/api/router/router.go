package router

import (
	"context"
	"net/http"

	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/api/handlers/mcpgateway"
	"github.com/obot-platform/obot/pkg/api/handlers/mcpgateway/oauth"
	"github.com/obot-platform/obot/pkg/api/handlers/registry"
	"github.com/obot-platform/obot/pkg/api/handlers/setup"
	"github.com/obot-platform/obot/pkg/api/handlers/wellknown"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/obot-platform/obot/ui"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/component-base/metrics/legacyregistry"
)

func Router(ctx context.Context, services *services.Services) (http.Handler, error) {
	mux := services.APIServer

	version, err := handlers.NewVersionHandler(ctx,
		services.GatewayClient,
		services.PostgresDSN,
		services.MCPRuntimeBackend,
		services.MCPNetworkPolicyEnabled,
		services.MCPDefaultDenyAllEgress,
		services.AuthEnabled,
		services.DisableUpdateCheck,
		services.MessagePoliciesEnabled)
	if err != nil {
		return nil, err
	}

	oauthChecker := oauth.NewMCPOAuthHandlerFactory(services.ServerURL, services.MCPLoader, services.StorageClient, services.GPTClient, services.GatewayClient, services.MCPOAuthTokenStorage)

	models := handlers.NewModelHandler(services.ModelAccessPolicyHelper)
	mcpCatalogs := handlers.NewMCPCatalogHandler(services.DefaultMCPCatalogPath, services.ServerURL, services.MCPRuntimeBackend, services.MCPLoader, oauthChecker, services.GatewayClient, services.AccessControlRuleHelper)
	systemMCPCatalogs := handlers.NewSystemMCPCatalogHandler(services.DefaultSystemMCPCatalogPath)
	accessControlRules := handlers.NewAccessControlRuleHandler()
	skillRepositories := handlers.NewSkillRepositoryHandler()
	skillAccessRules := handlers.NewSkillAccessRuleHandler()
	skills := handlers.NewSkillHandler(services.SkillAccessRuleHelper)
	powerUserWorkspaces := handlers.NewPowerUserWorkspaceHandler(services.ServerURL, services.AccessControlRuleHelper)
	mcpWebhookValidations := handlers.NewMCPWebhookValidationHandler(services.MCPLoader)
	availableModels := handlers.NewAvailableModelsHandler(services.ProviderDispatcher)
	modelProviders := handlers.NewModelProviderHandler(services.ProviderDispatcher, services.Invoker)
	modelAccessPolicies := handlers.NewModelAccessPolicyHandler()
	messagePolicies := handlers.NewMessagePolicyHandler()
	policyViolations := handlers.NewMessagePolicyViolationHandler()
	deviceScans := handlers.NewDeviceScansHandler()
	authProviders := handlers.NewAuthProviderHandler(services.ProviderDispatcher, services.PostgresDSN)
	defaultModelAliases := handlers.NewDefaultModelAliasHandler()
	images := handlers.NewImageHandler()
	mcp := handlers.NewMCPHandler(services.MCPLoader, services.AccessControlRuleHelper, oauthChecker, services.MCPImagePullSecrets, services.ServerURL)
	mcpGateway := mcpgateway.NewHandler(services.MCPLoader)
	mcpAuditLogs := mcpgateway.NewAuditLogHandler()
	auditLogExports := handlers.NewAuditLogExportHandler(services.GPTClient)
	serverInstances := handlers.NewServerInstancesHandler(services.AccessControlRuleHelper, services.ServerURL)
	systemMCPServers := handlers.NewSystemMCPServerHandler(services.MCPLoader)
	userDefaultRoleSettings := handlers.NewUserDefaultRoleSettingHandler()
	setupHandler := setup.NewHandler(services.ServerURL)
	registryHandler := registry.NewHandler(services.AccessControlRuleHelper, services.ServerURL, services.RegistryNoAuth)
	oauthClients := handlers.NewOAuthClientsHandler(services.OAuthServerConfig, services.ServerURL)
	publishedArtifacts := handlers.NewPublishedArtifactHandler(services.ArtifactBlobStore, services.ArtifactBlobBucket)
	imagePullSecretsHandler := handlers.NewImagePullSecretHandler(services.MCPRuntimeBackend, services.MCPImagePullSecrets, services.MCPServerNamespace, services.ServiceNamespace, services.ServiceAccountName, services.LocalK8sClient, services.ServiceAccountIssuerURL, services.ServiceAccountIssuerError)

	// Version
	mux.HandleFunc("GET /api/version", version.GetVersion)

	// MCP Catalog Entries (user routes to access single-user and remote MCP servers from all sources)
	mux.HandleFunc("GET /api/all-mcps/entries", mcp.ListEntriesFromAllSources)
	mux.HandleFunc("GET /api/all-mcps/entries/{entry_id}", mcp.GetEntryFromAllSources)

	// MCP Shared Servers (user routes to access multi-user MCP servers from all sources)
	mux.HandleFunc("GET /api/all-mcps/servers", mcp.ListServersFromAllSources)
	mux.HandleFunc("GET /api/all-mcps/servers/{mcp_server_id}", mcp.GetServerFromAllSources)
	mux.HandleFunc("GET /api/all-mcps/servers/{mcp_server_id}/tools", mcp.GetTools)
	mux.HandleFunc("GET /api/all-mcps/servers/{mcp_server_id}/resources", mcp.GetResources)
	mux.HandleFunc("GET /api/all-mcps/servers/{mcp_server_id}/resources/{resource_uri}", mcp.ReadResource)
	mux.HandleFunc("GET /api/all-mcps/servers/{mcp_server_id}/prompts", mcp.GetPrompts)
	mux.HandleFunc("GET /api/all-mcps/servers/{mcp_server_id}/prompts/{prompt_name}", mcp.GetPrompt)

	// User-Deployed MCP Servers (single-user, remote, and composite)
	mux.HandleFunc("GET /api/mcp-servers", mcp.ListServer)
	mux.HandleFunc("GET /api/mcp-servers/{mcp_server_id}", mcp.GetServer)
	mux.HandleFunc("POST /api/mcp-servers", mcp.CreateServer)
	mux.HandleFunc("PUT /api/mcp-servers/{mcp_server_id}", mcp.UpdateServer)
	mux.HandleFunc("PUT /api/mcp-servers/{mcp_server_id}/alias", mcp.UpdateServerAlias)
	mux.HandleFunc("DELETE /api/mcp-servers/{mcp_server_id}", mcp.DeleteServer)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/launch", mcp.LaunchServer)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/check-oauth", mcp.CheckOAuth)
	mux.HandleFunc("GET /api/mcp-servers/{mcp_server_id}/oauth-url", mcp.GetOAuthURL)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/oauth-debugger/client", mcp.RegisterOAuthDebuggerClient)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/oauth-debugger/authorization-url", mcp.GetOAuthDebuggerAuthorizationURL)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/oauth-debugger/token", mcp.ExchangeOAuthDebuggerToken)
	mux.HandleFunc("DELETE /api/mcp-servers/{mcp_server_id}/oauth", mcp.ClearOAuthCredentials)
	mux.HandleFunc("GET /api/mcp-servers/{mcp_server_id}/details", mcp.GetServerDetails)
	mux.HandleFunc("GET /api/mcp-servers/{mcp_server_id}/logs", mcp.StreamServerLogs)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/restart", mcp.RestartServerDeployment)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/configure", mcp.ConfigureServer)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/deconfigure", mcp.DeconfigureServer)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/reveal", mcp.Reveal)
	mux.HandleFunc("GET /api/mcp-servers/{mcp_server_id}/tools", mcp.GetTools)
	mux.HandleFunc("GET /api/mcp-servers/{mcp_server_id}/resources", mcp.GetResources)
	mux.HandleFunc("GET /api/mcp-servers/{mcp_server_id}/resources/{resource_uri}", mcp.ReadResource)
	mux.HandleFunc("GET /api/mcp-servers/{mcp_server_id}/prompts", mcp.GetPrompts)
	mux.HandleFunc("GET /api/mcp-servers/{mcp_server_id}/prompts/{prompt_name}", mcp.GetPrompt)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/update-url", mcp.UpdateURL)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/trigger-update", mcp.TriggerUpdate)

	// MCPServerInstances
	mux.HandleFunc("GET /api/mcp-server-instances", serverInstances.ListServerInstances)
	mux.HandleFunc("GET /api/mcp-server-instances/{mcp_server_instance_id}", serverInstances.GetServerInstance)
	mux.HandleFunc("POST /api/mcp-server-instances", serverInstances.CreateServerInstance)
	mux.HandleFunc("POST /api/mcp-server-instances/{mcp_server_instance_id}/reveal", serverInstances.RevealConfig)
	mux.HandleFunc("POST /api/mcp-server-instances/{mcp_server_instance_id}/configure", serverInstances.ConfigureServerInstance)
	mux.HandleFunc("POST /api/mcp-server-instances/{mcp_server_instance_id}/deconfigure", serverInstances.DeconfigureServerInstance)
	mux.HandleFunc("DELETE /api/mcp-server-instances/{mcp_server_instance_id}", serverInstances.DeleteServerInstance)
	mux.HandleFunc("DELETE /api/mcp-server-instances/{mcp_server_instance_id}/oauth", serverInstances.ClearOAuthCredentials)

	// MCP Catalogs (admin only)
	mux.HandleFunc("GET /api/mcp-catalogs", mcpCatalogs.List)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}", mcpCatalogs.Get)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/categories", mcpCatalogs.ListCategoriesForCatalog)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/refresh", mcpCatalogs.Refresh)
	mux.HandleFunc("PUT /api/mcp-catalogs/{catalog_id}", mcpCatalogs.Update)

	// MCPServerCatalogEntries (admin only, for single-user and remote MCP servers)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/entries", mcpCatalogs.ListEntries)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/entries/{entry_id}", mcpCatalogs.GetEntry)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/entries", mcpCatalogs.CreateEntry)
	mux.HandleFunc("PUT /api/mcp-catalogs/{catalog_id}/entries/{entry_id}", mcpCatalogs.UpdateEntry)
	mux.HandleFunc("DELETE /api/mcp-catalogs/{catalog_id}/entries/{entry_id}", mcpCatalogs.DeleteEntry)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/servers", mcpCatalogs.AdminListServersForEntryInCatalog)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/servers/{mcp_server_id}/k8s-settings-status", mcp.CheckK8sSettingsStatus)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/servers/{mcp_server_id}/redeploy-with-k8s-settings", mcp.RedeployWithK8sSettings)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/entries/all-servers", mcpCatalogs.AdminListServersForAllEntriesInCatalog)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/generate-tool-previews", mcpCatalogs.GenerateToolPreviews)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/generate-tool-previews/oauth-url", mcpCatalogs.GenerateToolPreviewsOAuthURL)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/{component_id}/generate-tool-previews", mcpCatalogs.GenerateComponentToolPreviews)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/{component_id}/generate-tool-previews/oauth-url", mcpCatalogs.GenerateComponentToolPreviewsOAuthURL)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/refresh-components", mcpCatalogs.RefreshCompositeComponents)

	// MCP Catalog Entry OAuth Credentials (admin only)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/oauth-credentials", mcpCatalogs.GetOAuthCredentials)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/oauth-credentials", mcpCatalogs.SetOAuthCredentials)
	mux.HandleFunc("DELETE /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/oauth-credentials", mcpCatalogs.DeleteOAuthCredentials)

	// MCPServers within the catalog (admin only, for multi-user MCP servers)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers", mcp.ListServer)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}", mcp.GetServer)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers", mcp.CreateServer)
	mux.HandleFunc("PUT /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}", mcp.UpdateServer)
	mux.HandleFunc("PUT /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/alias", mcp.UpdateServerAlias)
	mux.HandleFunc("DELETE /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}", mcp.DeleteServer)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/launch", mcp.LaunchServer)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/check-oauth", mcp.CheckOAuth)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/oauth-url", mcp.GetOAuthURL)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/oauth-debugger/client", mcp.RegisterOAuthDebuggerClient)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/oauth-debugger/authorization-url", mcp.GetOAuthDebuggerAuthorizationURL)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/oauth-debugger/token", mcp.ExchangeOAuthDebuggerToken)
	mux.HandleFunc("DELETE /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/oauth", mcp.ClearOAuthCredentials)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/details", mcp.GetServerDetails)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/logs", mcp.StreamServerLogs)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/restart", mcp.RestartServerDeployment)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/trigger-update", mcp.TriggerUpdate)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/configure", mcp.ConfigureServer)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/deconfigure", mcp.DeconfigureServer)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/reveal", mcp.Reveal)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/instances", serverInstances.ListServerInstancesForServer)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/k8s-settings-status", mcp.CheckK8sSettingsStatus)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/redeploy-with-k8s-settings", mcp.RedeployWithK8sSettings)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers-needing-k8s-update", mcp.ListServersNeedingK8sUpdateInCatalog)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers/all-instances", mcp.ListServerInstances)

	// Access Control Rules (admin only, scoped to catalogs)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/access-control-rules", accessControlRules.List)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/access-control-rules/{access_control_rule_id}", accessControlRules.Get)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/access-control-rules", accessControlRules.Create)
	mux.HandleFunc("PUT /api/mcp-catalogs/{catalog_id}/access-control-rules/{access_control_rule_id}", accessControlRules.Update)
	mux.HandleFunc("DELETE /api/mcp-catalogs/{catalog_id}/access-control-rules/{access_control_rule_id}", accessControlRules.Delete)

	// Power User Workspaces (read-only)
	mux.HandleFunc("GET /api/workspaces", powerUserWorkspaces.List)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}", powerUserWorkspaces.Get)

	mux.HandleFunc("GET /api/workspaces/all-entries", powerUserWorkspaces.ListAllEntries)
	mux.HandleFunc("GET /api/workspaces/all-servers", powerUserWorkspaces.ListAllServers)
	mux.HandleFunc("GET /api/workspaces/all-entries/all-servers", powerUserWorkspaces.ListAllServersForAllEntries)
	mux.HandleFunc("GET /api/workspaces/all-access-control-rules", powerUserWorkspaces.ListAllAccessControlRules)
	mux.HandleFunc("GET /api/workspaces/all-servers/all-instances", powerUserWorkspaces.ListAllServerInstances)

	// Workspace-scoped Access Control Rules (PowerUserPlus only)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/access-control-rules", accessControlRules.List)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}", accessControlRules.Get)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/access-control-rules", accessControlRules.Create)
	mux.HandleFunc("PUT /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}", accessControlRules.Update)
	mux.HandleFunc("DELETE /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}", accessControlRules.Delete)

	// Workspace-scoped MCP Server Catalog Entries (PowerUser and higher only)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/entries", mcpCatalogs.ListEntries)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/entries/{entry_id}", mcpCatalogs.GetEntry)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/entries", mcpCatalogs.CreateEntry)
	mux.HandleFunc("PUT /api/workspaces/{workspace_id}/entries/{entry_id}", mcpCatalogs.UpdateEntry)
	mux.HandleFunc("DELETE /api/workspaces/{workspace_id}/entries/{entry_id}", mcpCatalogs.DeleteEntry)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/entries/{entry_id}/servers", mcpCatalogs.ListServersForEntry)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcp_server_id}", mcpCatalogs.GetServerFromEntry)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcp_server_id}/details", mcp.GetServerDetails)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcp_server_id}/logs", mcp.StreamServerLogs)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcp_server_id}/restart", mcp.RestartServerDeployment)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcp_server_id}/trigger-update", mcp.TriggerUpdate)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcp_server_id}/k8s-settings-status", mcp.CheckK8sSettingsStatus)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcp_server_id}/redeploy-with-k8s-settings", mcp.RedeployWithK8sSettings)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/entries/{entry_id}/generate-tool-previews", mcpCatalogs.GenerateToolPreviews)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/entries/{entry_id}/generate-tool-previews/oauth-url", mcpCatalogs.GenerateToolPreviewsOAuthURL)

	// Workspace-scoped MCP Server Catalog Entry OAuth Credentials (PowerUser and higher only)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials", mcpCatalogs.GetOAuthCredentials)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials", mcpCatalogs.SetOAuthCredentials)
	mux.HandleFunc("DELETE /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials", mcpCatalogs.DeleteOAuthCredentials)

	// Workspace-scoped MCP Servers (PowerUserPlus and higher only)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers", mcp.ListServer)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}", mcp.GetServer)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers", mcp.CreateServer)
	mux.HandleFunc("PUT /api/workspaces/{workspace_id}/servers/{mcp_server_id}", mcp.UpdateServer)
	mux.HandleFunc("PUT /api/workspaces/{workspace_id}/servers/{mcp_server_id}/alias", mcp.UpdateServerAlias)
	mux.HandleFunc("DELETE /api/workspaces/{workspace_id}/servers/{mcp_server_id}", mcp.DeleteServer)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/launch", mcp.LaunchServer)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/check-oauth", mcp.CheckOAuth)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth-url", mcp.GetOAuthURL)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth-debugger/client", mcp.RegisterOAuthDebuggerClient)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth-debugger/authorization-url", mcp.GetOAuthDebuggerAuthorizationURL)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth-debugger/token", mcp.ExchangeOAuthDebuggerToken)
	mux.HandleFunc("DELETE /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth", mcp.ClearOAuthCredentials)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/configure", mcp.ConfigureServer)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/deconfigure", mcp.DeconfigureServer)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/reveal", mcp.Reveal)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/details", mcp.GetServerDetails)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/logs", mcp.StreamServerLogs)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/restart", mcp.RestartServerDeployment)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/k8s-settings-status", mcp.CheckK8sSettingsStatus)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/redeploy-with-k8s-settings", mcp.RedeployWithK8sSettings)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/instances", serverInstances.ListServerInstancesForServer)
	mux.HandleFunc("GET /api/workspaces/servers-needing-k8s-update", mcp.ListServersNeedingK8sUpdateAcrossWorkspaces)

	// MCP Webhook Validations (admin only)
	mux.HandleFunc("GET /api/mcp-webhook-validations", mcpWebhookValidations.List)
	mux.HandleFunc("GET /api/mcp-webhook-validations/{mcp_webhook_validation_id}", mcpWebhookValidations.Get)
	mux.HandleFunc("POST /api/mcp-webhook-validations", mcpWebhookValidations.Create)
	mux.HandleFunc("PUT /api/mcp-webhook-validations/{mcp_webhook_validation_id}", mcpWebhookValidations.Update)
	mux.HandleFunc("DELETE /api/mcp-webhook-validations/{mcp_webhook_validation_id}", mcpWebhookValidations.Delete)
	mux.HandleFunc("POST /api/mcp-webhook-validations/{mcp_webhook_validation_id}/configure", mcpWebhookValidations.Configure)
	mux.HandleFunc("POST /api/mcp-webhook-validations/{mcp_webhook_validation_id}/deconfigure", mcpWebhookValidations.Deconfigure)
	mux.HandleFunc("POST /api/mcp-webhook-validations/{mcp_webhook_validation_id}/launch", mcpWebhookValidations.Launch)
	mux.HandleFunc("POST /api/mcp-webhook-validations/{mcp_webhook_validation_id}/reveal", mcpWebhookValidations.Reveal)
	mux.HandleFunc("POST /api/mcp-webhook-validations/{mcp_webhook_validation_id}/restart", mcpWebhookValidations.Restart)
	mux.HandleFunc("GET /api/mcp-webhook-validations/{mcp_webhook_validation_id}/details", mcpWebhookValidations.GetDetails)
	mux.HandleFunc("GET /api/mcp-webhook-validations/{mcp_webhook_validation_id}/logs", mcpWebhookValidations.Logs)

	// System MCP Servers (admin only)
	mux.HandleFunc("GET /api/system-mcp-servers", systemMCPServers.List)
	mux.HandleFunc("POST /api/system-mcp-servers/restart-nanobot-agent-deployments", systemMCPServers.RestartNanobotAgentDeployments)
	mux.HandleFunc("GET /api/system-mcp-servers/{id}", systemMCPServers.Get)
	mux.HandleFunc("POST /api/system-mcp-servers", systemMCPServers.Create)
	mux.HandleFunc("PUT /api/system-mcp-servers/{id}", systemMCPServers.Update)
	mux.HandleFunc("DELETE /api/system-mcp-servers/{id}", systemMCPServers.Delete)
	mux.HandleFunc("POST /api/system-mcp-servers/{id}/configure", systemMCPServers.Configure)
	mux.HandleFunc("POST /api/system-mcp-servers/{id}/deconfigure", systemMCPServers.Deconfigure)
	mux.HandleFunc("POST /api/system-mcp-servers/{id}/restart", systemMCPServers.Restart)
	mux.HandleFunc("POST /api/system-mcp-servers/{id}/reveal", systemMCPServers.Reveal)
	mux.HandleFunc("GET /api/system-mcp-servers/{id}/details", systemMCPServers.GetDetails)
	mux.HandleFunc("GET /api/system-mcp-servers/{id}/logs", systemMCPServers.Logs)
	mux.HandleFunc("GET /api/system-mcp-servers/{id}/tools", systemMCPServers.GetTools)

	// System MCP Catalogs (admin only)
	mux.HandleFunc("GET /api/system-mcp-catalogs", systemMCPCatalogs.List)
	mux.HandleFunc("POST /api/system-mcp-catalogs", systemMCPCatalogs.Create)
	mux.HandleFunc("GET /api/system-mcp-catalogs/{catalog_id}", systemMCPCatalogs.Get)
	mux.HandleFunc("PUT /api/system-mcp-catalogs/{catalog_id}", systemMCPCatalogs.Update)
	mux.HandleFunc("DELETE /api/system-mcp-catalogs/{catalog_id}", systemMCPCatalogs.Delete)
	mux.HandleFunc("POST /api/system-mcp-catalogs/{catalog_id}/refresh", systemMCPCatalogs.Refresh)
	mux.HandleFunc("GET /api/system-mcp-catalogs/{catalog_id}/entries", systemMCPCatalogs.ListEntries)
	mux.HandleFunc("POST /api/system-mcp-catalogs/{catalog_id}/entries", systemMCPCatalogs.CreateEntry)
	mux.HandleFunc("GET /api/system-mcp-catalogs/{catalog_id}/entries/{entry_id}", systemMCPCatalogs.GetEntry)
	mux.HandleFunc("PUT /api/system-mcp-catalogs/{catalog_id}/entries/{entry_id}", systemMCPCatalogs.UpdateEntry)
	mux.HandleFunc("DELETE /api/system-mcp-catalogs/{catalog_id}/entries/{entry_id}", systemMCPCatalogs.DeleteEntry)

	// MCP Gateway Endpoints
	// The first pattern handles the root path, the second handles all sub-paths
	mux.HandleFunc("/mcp-connect/{mcp_id}", mcpGateway.Proxy)
	mux.HandleFunc("/mcp-connect/{mcp_id}/{rest...}", mcpGateway.Proxy)

	// Registry API
	mux.HandleFunc("GET /v0.1/servers", registryHandler.ListServers)
	mux.HandleFunc("GET /v0.1/servers/{serverName}/versions", registryHandler.ListServerVersions)
	mux.HandleFunc("GET /v0.1/servers/{serverName}/versions/{version}", registryHandler.GetServerVersion)

	// MCP Audit Logs
	mux.HandleFunc("GET /api/mcp-audit-logs", mcpAuditLogs.ListAuditLogs)
	mux.HandleFunc("POST /api/mcp-audit-logs", mcpAuditLogs.SubmitAuditLogs)
	mux.HandleFunc("GET /api/mcp-audit-logs/filter-options/{filter}", mcpAuditLogs.ListAuditLogFilterOptions)
	mux.HandleFunc("GET /api/mcp-audit-logs/detail/{audit_log_id}", mcpAuditLogs.GetAuditLog)
	mux.HandleFunc("GET /api/mcp-audit-logs/{mcp_id}", mcpAuditLogs.ListAuditLogs)
	mux.HandleFunc("GET /api/mcp-stats", mcpAuditLogs.GetUsageStats)
	mux.HandleFunc("GET /api/mcp-stats/{mcp_id}", mcpAuditLogs.GetUsageStats)

	// Audit Log Exports
	mux.HandleFunc("POST /api/audit-log-exports", auditLogExports.CreateAuditLogExport)
	mux.HandleFunc("GET /api/audit-log-exports", auditLogExports.ListAuditLogExports)
	mux.HandleFunc("GET /api/audit-log-exports/{id}", auditLogExports.GetAuditLogExport)
	mux.HandleFunc("DELETE /api/audit-log-exports/{id}", auditLogExports.DeleteAuditLogExport)

	// Scheduled Audit Log Exports
	mux.HandleFunc("POST /api/scheduled-audit-log-exports", auditLogExports.CreateScheduledAuditLogExport)
	mux.HandleFunc("GET /api/scheduled-audit-log-exports", auditLogExports.ListScheduledAuditLogExports)
	mux.HandleFunc("GET /api/scheduled-audit-log-exports/{id}", auditLogExports.GetScheduledAuditLogExport)
	mux.HandleFunc("PATCH /api/scheduled-audit-log-exports/{id}", auditLogExports.UpdateScheduledAuditLogExport)
	mux.HandleFunc("DELETE /api/scheduled-audit-log-exports/{id}", auditLogExports.DeleteScheduledAuditLogExport)

	// Storage Credentials Management
	mux.HandleFunc("POST /api/storage-credentials", auditLogExports.ConfigureStorageCredentials)
	mux.HandleFunc("GET /api/storage-credentials", auditLogExports.GetStorageCredentials)
	mux.HandleFunc("DELETE /api/storage-credentials", auditLogExports.DeleteStorageCredentials)
	mux.HandleFunc("POST /api/storage-credentials/test", auditLogExports.TestStorageCredentials)

	// Published Artifacts
	mux.HandleFunc("POST /api/published-artifacts", publishedArtifacts.Create)
	mux.HandleFunc("GET /api/published-artifacts", publishedArtifacts.List)
	mux.HandleFunc("GET /api/published-artifacts/{id}", publishedArtifacts.Get)
	mux.HandleFunc("GET /api/published-artifacts/{id}/download", publishedArtifacts.Download)
	mux.HandleFunc("GET /api/published-artifacts/{id}/{version}/skill", publishedArtifacts.GetSkillMD)
	mux.HandleFunc("PUT /api/published-artifacts/{id}", publishedArtifacts.Update)
	mux.HandleFunc("DELETE /api/published-artifacts/{id}", publishedArtifacts.Delete)

	// Skills
	mux.HandleFunc("GET /api/skills", skills.List)
	mux.HandleFunc("GET /api/skills/{id}", skills.Get)
	mux.HandleFunc("GET /api/skills/{id}/download", skills.Download)

	// Skill repositories (admin only)
	mux.HandleFunc("GET /api/skill-repositories", skillRepositories.List)
	mux.HandleFunc("POST /api/skill-repositories", skillRepositories.Create)
	mux.HandleFunc("GET /api/skill-repositories/{skill_repository_id}", skillRepositories.Get)
	mux.HandleFunc("PUT /api/skill-repositories/{skill_repository_id}", skillRepositories.Update)
	mux.HandleFunc("DELETE /api/skill-repositories/{skill_repository_id}", skillRepositories.Delete)
	mux.HandleFunc("POST /api/skill-repositories/{skill_repository_id}/refresh", skillRepositories.Refresh)

	// Skill access rules (admin only)
	mux.HandleFunc("GET /api/skill-access-rules", skillAccessRules.List)
	mux.HandleFunc("POST /api/skill-access-rules", skillAccessRules.Create)
	mux.HandleFunc("GET /api/skill-access-rules/{skill_access_rule_id}", skillAccessRules.Get)
	mux.HandleFunc("PUT /api/skill-access-rules/{skill_access_rule_id}", skillAccessRules.Update)
	mux.HandleFunc("DELETE /api/skill-access-rules/{skill_access_rule_id}", skillAccessRules.Delete)

	// OAuthClients
	mux.HandleFunc("GET /api/oauth-clients", oauthClients.List)
	mux.HandleFunc("POST /api/oauth-clients", oauthClients.Create)
	mux.HandleFunc("GET /api/oauth-clients/{client_id}", oauthClients.Get)
	mux.HandleFunc("PUT /api/oauth-clients/{client_id}", oauthClients.Update)
	mux.HandleFunc("DELETE /api/oauth-clients/{client_id}", oauthClients.Delete)
	mux.HandleFunc("POST /api/oauth-clients/{client_id}/client-secret", oauthClients.RollClientSecret)

	// User Default Role Settings
	mux.HandleFunc("GET /api/user-default-role-settings", userDefaultRoleSettings.Get)
	mux.HandleFunc("POST /api/user-default-role-settings", userDefaultRoleSettings.Set)

	// K8s Settings
	k8sSettingsHandler := handlers.NewK8sSettingsHandler()
	mux.HandleFunc("GET /api/k8s-settings", k8sSettingsHandler.Get)
	mux.HandleFunc("PUT /api/k8s-settings", k8sSettingsHandler.Update)

	// Image Pull Secrets
	mux.HandleFunc("GET /api/image-pull-secrets/capability", imagePullSecretsHandler.Capability)
	mux.HandleFunc("GET /api/image-pull-secrets", imagePullSecretsHandler.List)
	mux.HandleFunc("POST /api/image-pull-secrets", imagePullSecretsHandler.Create)
	mux.HandleFunc("GET /api/image-pull-secrets/{id}", imagePullSecretsHandler.Get)
	mux.HandleFunc("PUT /api/image-pull-secrets/{id}", imagePullSecretsHandler.Update)
	mux.HandleFunc("DELETE /api/image-pull-secrets/{id}", imagePullSecretsHandler.Delete)
	mux.HandleFunc("POST /api/image-pull-secrets/{id}/test", imagePullSecretsHandler.Test)
	mux.HandleFunc("POST /api/image-pull-secrets/{id}/refresh", imagePullSecretsHandler.Refresh)

	// MCP Capacity (admin only)
	mcpCapacityHandler := handlers.NewMCPCapacityHandler(services.MCPLoader)
	mux.HandleFunc("GET /api/mcp-capacity", mcpCapacityHandler.GetCapacity)

	// EULA
	eulaHandler := handlers.NewEulaHandler()
	mux.HandleFunc("GET /api/eula", eulaHandler.Get)
	mux.HandleFunc("PUT /api/eula", eulaHandler.Update)

	// App Preferences
	appPrefsHandler := handlers.NewAppPreferencesHandler()
	mux.HandleFunc("GET /api/app-preferences", appPrefsHandler.Get)
	mux.HandleFunc("PUT /api/app-preferences", appPrefsHandler.Update)

	// Debug
	mux.HTTPHandle("GET /debug/pprof/", http.DefaultServeMux)
	mux.HTTPHandle("GET /debug/triggers", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		b, err := services.Router.DumpTriggers(true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		_, _ = w.Write(b)
	}))

	// Metrics
	mux.HTTPHandle("GET /debug/metrics", promhttp.HandlerFor(legacyregistry.DefaultGatherer, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	}))

	// Model providers
	mux.HandleFunc("GET /api/model-providers", modelProviders.List)
	mux.HandleFunc("GET /api/model-providers/{model_provider_id}", modelProviders.ByID)
	mux.HandleFunc("POST /api/model-providers/{model_provider_id}/configure", modelProviders.Configure)
	mux.HandleFunc("POST /api/model-providers/{model_provider_id}/deconfigure", modelProviders.Deconfigure)
	mux.HandleFunc("POST /api/model-providers/{model_provider_id}/refresh-models", modelProviders.RefreshModels)
	mux.HandleFunc("POST /api/model-providers/{model_provider_id}/reveal", modelProviders.Reveal)
	mux.HandleFunc("POST /api/model-providers/{model_provider_id}/validate", modelProviders.Validate)

	// Auth providers
	mux.HandleFunc("GET /api/auth-providers", authProviders.List)
	mux.HandleFunc("GET /api/auth-providers/{id}", authProviders.ByID)
	mux.HandleFunc("POST /api/auth-providers/{id}/configure", authProviders.Configure)
	mux.HandleFunc("POST /api/auth-providers/{id}/deconfigure", authProviders.Deconfigure)
	mux.HandleFunc("POST /api/auth-providers/{id}/reveal", authProviders.Reveal)

	// Bootstrap
	mux.HandleFunc("GET /api/bootstrap", services.Bootstrapper.IsEnabled)
	mux.HandleFunc("POST /api/bootstrap/login", services.Bootstrapper.Login)
	mux.HandleFunc("POST /api/bootstrap/logout", services.Bootstrapper.Logout)

	// Setup endpoints for bootstrap configuration flow
	mux.HandleFunc("GET /api/setup/explicit-role-emails", setupHandler.ListExplicitRoleEmails)
	mux.HandleFunc("POST /api/setup/initiate-temp-login", setupHandler.InitiateTempLogin)
	mux.HandleFunc("GET /api/setup/oauth-complete", setupHandler.OAuthComplete)
	mux.HandleFunc("GET /api/setup/temp-user", setupHandler.GetTempUser)
	mux.HandleFunc("POST /api/setup/confirm-owner", setupHandler.ConfirmOwner)
	mux.HandleFunc("POST /api/setup/cancel-temp-login", setupHandler.CancelTempLogin)

	// Models
	mux.HandleFunc("POST /api/models", models.Create)
	mux.HandleFunc("GET /api/models", models.List)
	mux.HandleFunc("GET /api/models/{id}", models.ByID)
	mux.HandleFunc("DELETE /api/models/{id}", models.Delete)
	mux.HandleFunc("PUT /api/models/{id}", models.Update)

	// Model Permission Rules
	mux.HandleFunc("GET /api/model-access-policies", modelAccessPolicies.List)
	mux.HandleFunc("GET /api/model-access-policies/{id}", modelAccessPolicies.Get)
	mux.HandleFunc("POST /api/model-access-policies", modelAccessPolicies.Create)
	mux.HandleFunc("PUT /api/model-access-policies/{id}", modelAccessPolicies.Update)
	mux.HandleFunc("DELETE /api/model-access-policies/{id}", modelAccessPolicies.Delete)

	// Message Policies
	if services.MessagePoliciesEnabled {
		mux.HandleFunc("GET /api/message-policies", messagePolicies.List)
		mux.HandleFunc("GET /api/message-policies/{id}", messagePolicies.Get)
		mux.HandleFunc("POST /api/message-policies", messagePolicies.Create)
		mux.HandleFunc("PUT /api/message-policies/{id}", messagePolicies.Update)
		mux.HandleFunc("DELETE /api/message-policies/{id}", messagePolicies.Delete)

		// Message Policy Violations
		mux.HandleFunc("GET /api/message-policy-violations", policyViolations.List)
		mux.HandleFunc("GET /api/message-policy-violations/filter-options/{filter}", policyViolations.ListFilterOptions)
		mux.HandleFunc("GET /api/message-policy-violations/{id}", policyViolations.Get)
		mux.HandleFunc("GET /api/message-policy-violation-stats", policyViolations.GetStats)
	}

	// Device Scans
	mux.HandleFunc("POST /api/devices/scans", deviceScans.Submit)
	mux.HandleFunc("GET /api/devices/scans", deviceScans.List)
	mux.HandleFunc("GET /api/devices/scans/{scan_id}", deviceScans.Get)
	mux.HandleFunc("DELETE /api/devices/scans/{scan_id}", deviceScans.Delete)
	mux.HandleFunc("GET /api/devices/scan-stats", deviceScans.GetScanStats)
	mux.HandleFunc("GET /api/devices/mcp-servers/{config_hash}", deviceScans.GetMCPServerDetail)
	mux.HandleFunc("GET /api/devices/mcp-servers/{config_hash}/occurrences", deviceScans.ListMCPServerOccurrences)
	mux.HandleFunc("GET /api/devices/skills", deviceScans.ListSkills)
	mux.HandleFunc("GET /api/devices/skills/{name}", deviceScans.GetSkill)
	mux.HandleFunc("GET /api/devices/skills/{name}/occurrences", deviceScans.ListSkillOccurrences)
	mux.HandleFunc("GET /api/devices/clients", deviceScans.ListClients)
	mux.HandleFunc("GET /api/devices/clients/{name}", deviceScans.GetClient)

	// Available Models
	mux.HandleFunc("GET /api/available-models", availableModels.List)
	mux.HandleFunc("GET /api/available-models/{model_provider_id}", availableModels.ListForModelProvider)

	// Default Model Aliases
	mux.HandleFunc("POST /api/default-model-aliases", defaultModelAliases.Create)
	mux.HandleFunc("GET /api/default-model-aliases", defaultModelAliases.List)
	mux.HandleFunc("DELETE /api/default-model-aliases/{id}", defaultModelAliases.Delete)
	mux.HandleFunc("GET /api/default-model-aliases/{id}", defaultModelAliases.GetByID)
	mux.HandleFunc("PUT /api/default-model-aliases/{id}", defaultModelAliases.Update)

	// Uploaded images
	mux.HandleFunc("POST /api/image/upload", images.UploadImage)
	mux.HandleFunc("GET /api/image/{id}", images.GetImage)

	// Projects
	projects := handlers.NewProjectHandler()
	mux.HandleFunc("POST /api/projects", projects.Create)
	mux.HandleFunc("GET /api/projects", projects.List)
	mux.HandleFunc("GET /api/projects/{project_id}", projects.ByID)
	mux.HandleFunc("PUT /api/projects/{project_id}", projects.Update)
	mux.HandleFunc("DELETE /api/projects/{project_id}", projects.Delete)

	// NanobotAgents
	nanobotAgents := handlers.NewNanobotAgentHandler(services.MCPLoader, services.ServerURL)
	mux.HandleFunc("GET /api/nanobot-agents", nanobotAgents.ListAll)
	mux.HandleFunc("POST /api/projects/{project_id}/agents", nanobotAgents.Create)
	mux.HandleFunc("GET /api/projects/{project_id}/agents", nanobotAgents.List)
	mux.HandleFunc("GET /api/projects/{project_id}/agents/{nanobot_agent_id}", nanobotAgents.ByID)
	mux.HandleFunc("PUT /api/projects/{project_id}/agents/{nanobot_agent_id}", nanobotAgents.Update)
	mux.HandleFunc("DELETE /api/projects/{project_id}/agents/{nanobot_agent_id}", nanobotAgents.Delete)
	mux.HandleFunc("POST /api/projects/{project_id}/agents/{nanobot_agent_id}/launch", nanobotAgents.Launch)

	// Catch all 404 for API
	mux.HTTPHandle("/api/", http.NotFoundHandler())

	// Auth Provider tools
	mux.HandleFunc("/oauth2/", services.ProxyManager.HandlerFunc)

	// Well-known
	wellknown.SetupHandlers(services.ServerURL, services.OAuthServerConfig, services.RegistryNoAuth, mux)

	// Obot OAuth
	oauth.SetupHandlers(oauthChecker, services.MCPOAuthTokenStorage, services.PersistentTokenServer, services.OAuthServerConfig, services.ServerURL, services.MCPOAuthClientSecretExpiration, mux)

	// Gateway APIs
	services.GatewayServer.AddRoutes(services.APIServer)

	services.APIServer.HTTPHandle("/", ui.Handler(services.DevUIPort, services.UserUIPort))

	return services.APIServer, nil
}
