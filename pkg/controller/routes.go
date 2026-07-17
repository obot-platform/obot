package controller

import (
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/controller/generationed"
	"github.com/obot-platform/obot/pkg/controller/handlers/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/controller/handlers/adminworkspace"
	"github.com/obot-platform/obot/pkg/controller/handlers/agentsource"
	"github.com/obot-platform/obot/pkg/controller/handlers/alias"
	"github.com/obot-platform/obot/pkg/controller/handlers/auditlogexport"
	"github.com/obot-platform/obot/pkg/controller/handlers/cleanup"
	"github.com/obot-platform/obot/pkg/controller/handlers/hostedagent"
	"github.com/obot-platform/obot/pkg/controller/handlers/imagepullsecret"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpcatalog"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpserver"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpservercatalogentry"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpserverinstance"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpwebhookvalidation"
	"github.com/obot-platform/obot/pkg/controller/handlers/mdmassetsource"
	"github.com/obot-platform/obot/pkg/controller/handlers/model"
	"github.com/obot-platform/obot/pkg/controller/handlers/modelaccesspolicy"
	"github.com/obot-platform/obot/pkg/controller/handlers/modelinfosource"
	"github.com/obot-platform/obot/pkg/controller/handlers/nanobotagent"
	"github.com/obot-platform/obot/pkg/controller/handlers/oauthclients"
	"github.com/obot-platform/obot/pkg/controller/handlers/oktagroupmigration"
	"github.com/obot-platform/obot/pkg/controller/handlers/poweruserworkspace"
	"github.com/obot-platform/obot/pkg/controller/handlers/project"
	"github.com/obot-platform/obot/pkg/controller/handlers/provider"
	"github.com/obot-platform/obot/pkg/controller/handlers/scheduledauditlogexport"
	"github.com/obot-platform/obot/pkg/controller/handlers/skillrepository"
	"github.com/obot-platform/obot/pkg/controller/handlers/systemmcpserver"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (c *Controller) setupRoutes() {
	root := c.services.Router

	providers := provider.New(c.services.GatewayClient, c.services.ProviderDispatcher, c.services.LicenseProvider, c.services.ProviderRegistryPaths)
	credentialCleanup := cleanup.NewCredentials(c.services.MCPSessionManager, c.services.GatewayClient, c.services.ServerURL)
	userCleanup := cleanup.NewUserCleanup(c.services.GatewayClient, c.services.AccessControlRuleHelper)
	mcpCatalog := mcpcatalog.New(c.services.DefaultMCPCatalogPath, c.services.DefaultSystemMCPCatalogPath, c.services.GatewayClient, c.services.AccessControlRuleHelper, c.services.MCPSessionManager)
	modelInfoSource := modelinfosource.New(c.services.ModelInfoSourceURL, c.services.MCPSessionManager.RemoteMCPURLValidationConfig())
	mdmAssetSource := mdmassetsource.New(c.services.MDMAssetSource, c.services.GatewayClient)
	skillRepository := skillrepository.New(c.services.GatewayClient)
	mcpserver := mcpserver.New(c.services.GatewayClient, c.services.MCPSessionManager, c.services.MCPOAuthTokenStorage, c.services.MCPNetworkPolicyEnabled, c.services.MCPDefaultDenyAllEgress, c.services.SingleUserIdleServerShutdownInterval, c.services.MultiUserIdleServerShutdownInterval, c.services.AgentIdleServerShutdownInterval, c.services.ServerURL, c.services.MCPRuntimeBackend, c.services.MCPImagePullSecrets)
	mcpserverinstance := mcpserverinstance.New(c.services.GatewayClient)
	accesscontrolrule := accesscontrolrule.New(c.services.AccessControlRuleHelper)
	mcpWebhookValidations := mcpwebhookvalidation.New(c.services.GatewayClient, c.services.MCPHTTPWebhookBaseImage)
	powerUserWorkspaceHandler := poweruserworkspace.NewHandler(c.services.GatewayClient)
	adminWorkspaceHandler := adminworkspace.New(c.services.GatewayClient)
	mcpServerCatalogEntryHandler := mcpservercatalogentry.NewHandler(c.services.GatewayClient)
	auditLogExportHandler := auditlogexport.NewHandler(c.services.GatewayClient)
	scheduledAuditLogExportHandler := scheduledauditlogexport.NewHandler()
	oauthclients := oauthclients.NewHandler(c.services.GatewayClient)
	systemMCPServerHandler := systemmcpserver.New(c.services.GatewayClient, c.services.MCPSessionManager, c.services.ServerURL)
	nanobotAgentHandler := nanobotagent.New(c.services.GatewayClient, c.services.LocalRouter, c.services.NanobotAgentImage, c.services.ServerURL, c.services.MCPServerNamespace, c.services.MCPSessionManager)
	agentSourceHandler := agentsource.New()
	hostedAgentHandler := hostedagent.New(c.services.ServerURL)
	oktaGroupMigrationHandler := oktagroupmigration.New()
	projectHandler := project.New(c.services.GatewayClient)
	imagePullSecretHandler := imagepullsecret.New(c.services.GatewayClient, c.services.LocalK8sClient, c.services.MCPRuntimeBackend, c.services.MCPServerNamespace, c.services.ServiceNamespace, c.services.ServiceAccountName, c.services.MCPImagePullSecrets, c.services.ServiceAccountIssuerURL)
	modelHandler := model.NewHandler(c.services.GatewayClient)

	// AuthProviders
	root.Type(&v1.AuthProvider{}).HandlerFunc(providers.SetAuthProviderConfiguredStatus)

	// ModelProviders
	root.Type(&v1.ModelProvider{}).HandlerFunc(providers.SetModelProviderConfiguredStatus)
	root.Type(&v1.ModelProvider{}).HandlerFunc(providers.BackPopulateModels)
	root.Type(&v1.ModelProvider{}).FinalizeFunc(v1.ModelProviderFinalizer, providers.CleanupModelProvider)

	// Models
	root.Type(&v1.Model{}).HandlerFunc(modelHandler.RemoveApplyUpdateAnnotation)
	root.Type(&v1.Model{}).HandlerFunc(modelHandler.Cleanup)
	root.Type(&v1.Model{}).HandlerFunc(modelHandler.EnsureModelInfo)
	root.Type(&v1.Model{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.Model{}).HandlerFunc(generationed.UpdateObservedGeneration)

	// DefaultModelAliases
	root.Type(&v1.DefaultModelAlias{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.DefaultModelAlias{}).HandlerFunc(generationed.UpdateObservedGeneration)

	// Alias
	root.Type(&v1.Alias{}).HandlerFunc(alias.UnassignAlias)

	// User Cleanup
	root.Type(&v1.UserDelete{}).HandlerFunc(userCleanup.Cleanup)

	// ModelInfoSource
	root.Type(&v1.ModelInfoSource{}).HandlerFunc(modelInfoSource.Sync)
	root.Type(&v1.ModelInfo{}).HandlerFunc(cleanup.Cleanup)

	// MDMAssetSource
	root.Type(&v1.MDMAssetSource{}).Namespace(system.DefaultNamespace).Name(system.DefaultMDMAssetSource).HandlerFunc(mdmAssetSource.Sync)

	// MCPCatalog
	root.Type(&v1.MCPCatalog{}).HandlerFunc(mcpCatalog.Sync)
	root.Type(&v1.MCPCatalog{}).HandlerFunc(mcpCatalog.DeleteUnauthorizedMCPServersForCatalog)
	root.Type(&v1.MCPCatalog{}).HandlerFunc(mcpCatalog.DeleteUnauthorizedMCPServerInstancesForCatalog)

	// SystemMCPCatalog
	root.Type(&v1.SystemMCPCatalog{}).HandlerFunc(mcpCatalog.SyncSystem)

	// SkillRepository
	root.Type(&v1.SkillRepository{}).HandlerFunc(skillRepository.Sync)

	// Skill
	root.Type(&v1.Skill{}).HandlerFunc(cleanup.Cleanup)

	// AgentSource
	root.Type(&v1.AgentSource{}).HandlerFunc(agentSourceHandler.Sync)

	// HostedAgent
	// cleanup.Cleanup honors DeleteRefs, which removes agents whose AgentSource
	// is gone. Hand-registered agents have no SourceID and are left alone.
	root.Type(&v1.HostedAgent{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.HostedAgent{}).HandlerFunc(hostedAgentHandler.Orchestrate)

	// HostedAgentInstance
	root.Type(&v1.HostedAgentInstance{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.HostedAgentInstance{}).HandlerFunc(hostedAgentHandler.OrchestrateInstance)

	// ImagePullSecret
	root.Type(&v1.ImagePullSecret{}).FinalizeFunc(v1.ImagePullSecretFinalizer, imagePullSecretHandler.Cleanup)
	root.Type(&v1.ImagePullSecret{}).HandlerFunc(imagePullSecretHandler.Reconcile)

	// MCPServerCatalogEntry
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.MCPServerCatalogEntry{}).FinalizeFunc(v1.MCPServerCatalogEntryFinalizer, mcpServerCatalogEntryHandler.RemoveOAuthCredentials)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.EnsureServerUserType)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.DeleteEntriesWithoutRuntime)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.UpdateManifestHashAndLastUpdated)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.CleanupNestedCompositeEntries)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.DetectCompositeDrift)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.EnsureUserCount)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.CleanupUnusedOAuthCredentials)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.EnsureOAuthCredentialStatus)

	// SystemMCPServerCatalogEntry
	root.Type(&v1.SystemMCPServerCatalogEntry{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.SystemMCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.UpdateSystemManifestHashAndLastUpdated)

	// MCPServer
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.EnsureMCPCatalogID)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.MigrateSharedWithinMCPCatalogName)
	root.Type(&v1.MCPServer{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.DeleteServersWithoutRuntime)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.DeleteServersForAnonymousUser)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.CleanupNestedCompositeServers)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.DetectDrift)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.DetectK8sSettingsDrift)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.EnsureMCPNetworkPolicy)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.EnsureMCPServerInstanceUserCount)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.SyncOAuthCredentialStatus)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.SyncOAuthMetadata)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.SyncThirdPartyAuthStatus)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.EnsureMCPServerSecretInfo)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.EnsureCompositeComponents)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.ShutdownIdleServers)
	root.Type(&v1.MCPServer{}).FinalizeFunc(v1.MCPServerFinalizer, credentialCleanup.RemoveMCPCredentials)

	// MCPNetworkPolicy
	root.Type(&v1.MCPNetworkPolicy{}).HandlerFunc(cleanup.Cleanup)

	// MCPServerInstance
	root.Type(&v1.MCPServerInstance{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.MCPServerInstance{}).HandlerFunc(mcpserverinstance.MigrationDeleteSingleUserInstances)
	root.Type(&v1.MCPServerInstance{}).HandlerFunc(mcpserverinstance.UpdateMultiUserConfig)
	root.Type(&v1.MCPServerInstance{}).FinalizeFunc(v1.MCPServerInstanceFinalizer, credentialCleanup.RemoveMCPInstanceCredentials)

	// AccessControlRule
	root.Type(&v1.AccessControlRule{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.AccessControlRule{}).HandlerFunc(accesscontrolrule.PruneDeletedResources)
	// This is a hack. We use field selectors to trigger other resources. However, when an access control rule is deleted,
	// we don't trigger because we don't have the object to match the field selectors against.
	// Having a finalizer that does nothing will ensure that the other resources are triggered.
	root.Type(&v1.AccessControlRule{}).FinalizeFunc(v1.AccessControlRuleFinalizer, func(router.Request, router.Response) error {
		return nil
	})

	// ModelAccessPolicys
	root.Type(&v1.ModelAccessPolicy{}).HandlerFunc(modelaccesspolicy.PruneModels)

	// OAuthClients
	root.Type(&v1.OAuthClient{}).HandlerFunc(cleanup.OAuthClients)
	root.Type(&v1.OAuthClient{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.OAuthClient{}).FinalizeFunc(v1.OAuthClientFinalizer, oauthclients.CleanupOAuthClientCred)

	// OAuthAuthRequests
	root.Type(&v1.OAuthAuthRequest{}).HandlerFunc(cleanup.OAuthAuth)
	root.Type(&v1.OAuthAuthRequest{}).HandlerFunc(cleanup.Cleanup)

	// OAuthTokens
	root.Type(&v1.OAuthToken{}).HandlerFunc(cleanup.Cleanup)

	// MCP Webhook Validations
	root.Type(&v1.MCPWebhookValidation{}).HandlerFunc(mcpWebhookValidations.CleanupResources)
	root.Type(&v1.MCPWebhookValidation{}).HandlerFunc(mcpWebhookValidations.EnsureSystemServer)

	// UserRoleChange
	root.Type(&v1.UserRoleChange{}).HandlerFunc(powerUserWorkspaceHandler.HandleRoleChange)

	// UserGroupChange
	root.Type(&v1.UserGroupChange{}).HandlerFunc(mcpCatalog.HandleUserGroupChange)

	// GroupRoleChange
	root.Type(&v1.GroupRoleChange{}).HandlerFunc(powerUserWorkspaceHandler.HandleGroupRoleChange)

	// OktaGroupMigration
	root.Type(&v1.OktaGroupMigration{}).HandlerFunc(oktaGroupMigrationHandler.Migrate)

	// PowerUserWorkspace
	root.Type(&v1.PowerUserWorkspace{}).HandlerFunc(powerUserWorkspaceHandler.CreateACR)
	root.Type(&v1.PowerUserWorkspace{}).HandlerFunc(mcpCatalog.DeleteUnauthorizedMCPServersForWorkspace)
	root.Type(&v1.PowerUserWorkspace{}).HandlerFunc(mcpCatalog.DeleteUnauthorizedMCPServerInstancesForWorkspace)

	// System MCP Servers
	root.Type(&v1.SystemMCPServer{}).HandlerFunc(systemMCPServerHandler.EnsureSecretInfo)
	root.Type(&v1.SystemMCPServer{}).HandlerFunc(systemMCPServerHandler.EnsureDeployment)
	root.Type(&v1.SystemMCPServer{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.SystemMCPServer{}).FinalizeFunc(v1.SystemMCPServerFinalizer, systemMCPServerHandler.CleanupDeployment)

	// AuditLogExport
	root.Type(&v1.AuditLogExport{}).HandlerFunc(auditLogExportHandler.ExportAuditLogs)

	// ScheduledAuditLogExport
	root.Type(&v1.ScheduledAuditLogExport{}).HandlerFunc(scheduledAuditLogExportHandler.ScheduleExports)

	// ProjectV2 Migration
	//nolint:staticcheck
	root.Type(&v1.ProjectV2{}).HandlerFunc(projectHandler.MigrateProjectV2)

	// NanobotAgent
	root.Type(&v1.NanobotAgent{}).HandlerFunc(nanobotAgentHandler.EnsureMCPServer)
	root.Type(&v1.NanobotAgent{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.NanobotAgent{}).FinalizeFunc(v1.NanobotAgentFinalizer, nanobotAgentHandler.Cleanup)

	c.providerHandler = providers
	c.mcpCatalogHandler = mcpCatalog
	c.mdmAssetSourceHandler = mdmAssetSource
	c.modelInfoSourceHandler = modelInfoSource
	c.adminWorkspaceHandler = adminWorkspaceHandler
}
