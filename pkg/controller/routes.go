package controller

import (
	"github.com/obot-platform/nah/pkg/handlers"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/controller/generationed"
	"github.com/obot-platform/obot/pkg/controller/handlers/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/controller/handlers/adminworkspace"
	"github.com/obot-platform/obot/pkg/controller/handlers/agents"
	"github.com/obot-platform/obot/pkg/controller/handlers/alias"
	"github.com/obot-platform/obot/pkg/controller/handlers/auditlogexport"
	"github.com/obot-platform/obot/pkg/controller/handlers/cleanup"
	"github.com/obot-platform/obot/pkg/controller/handlers/cronjob"
	"github.com/obot-platform/obot/pkg/controller/handlers/knowledgefile"
	"github.com/obot-platform/obot/pkg/controller/handlers/knowledgeset"
	"github.com/obot-platform/obot/pkg/controller/handlers/knowledgesource"
	"github.com/obot-platform/obot/pkg/controller/handlers/knowledgesummary"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpcatalog"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpserver"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpservercatalogentry"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpserverinstance"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpsession"
	"github.com/obot-platform/obot/pkg/controller/handlers/modelaccesspolicy"
	"github.com/obot-platform/obot/pkg/controller/handlers/nanobotagent"
	"github.com/obot-platform/obot/pkg/controller/handlers/oauthapp"
	"github.com/obot-platform/obot/pkg/controller/handlers/oauthclients"
	"github.com/obot-platform/obot/pkg/controller/handlers/poweruserworkspace"
	"github.com/obot-platform/obot/pkg/controller/handlers/projectinvitation"
	"github.com/obot-platform/obot/pkg/controller/handlers/projectmcpserver"
	"github.com/obot-platform/obot/pkg/controller/handlers/projects"
	"github.com/obot-platform/obot/pkg/controller/handlers/retention"
	"github.com/obot-platform/obot/pkg/controller/handlers/runs"
	"github.com/obot-platform/obot/pkg/controller/handlers/runstates"
	"github.com/obot-platform/obot/pkg/controller/handlers/scheduledauditlogexport"
	"github.com/obot-platform/obot/pkg/controller/handlers/systemmcpserver"
	"github.com/obot-platform/obot/pkg/controller/handlers/threads"
	"github.com/obot-platform/obot/pkg/controller/handlers/threadshare"
	"github.com/obot-platform/obot/pkg/controller/handlers/toolinfo"
	"github.com/obot-platform/obot/pkg/controller/handlers/toolreference"
	"github.com/obot-platform/obot/pkg/controller/handlers/workflow"
	"github.com/obot-platform/obot/pkg/controller/handlers/workflowexecution"
	"github.com/obot-platform/obot/pkg/controller/handlers/workflowstep"
	"github.com/obot-platform/obot/pkg/controller/handlers/workspace"
	"github.com/obot-platform/obot/pkg/controller/mcpwebhookvalidation"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (c *Controller) setupRoutes() {
	root := c.router

	workflowExecution := workflowexecution.New(c.services.Invoker)
	workflowStep := workflowstep.New(c.services.Invoker, c.services.GPTClient, c.services.MCPLoader)
	toolRef := toolreference.New(
		c.services.GPTClient,
		c.services.ProviderDispatcher,
		c.services.ToolRegistryURLs,
		c.services.SupportDocker,
	)
	workspace := workspace.New(c.services.GPTClient, c.services.WorkspaceProviderType)
	knowledgeset := knowledgeset.New(c.services.Invoker, c.services.GPTClient)
	knowledgesource := knowledgesource.NewHandler(c.services.Invoker, c.services.GPTClient)
	knowledgefile := knowledgefile.New(c.services.Invoker, c.services.GPTClient, c.services.KnowledgeSetIngestionLimit)
	runs := runs.New(c.services.Invoker, c.services.Router.Backend(), c.services.GatewayClient, c.services.GPTClient)
	cronJobs := cronjob.New()
	oauthLogins := oauthapp.NewLogin(c.services.Invoker, c.services.GPTClient, c.services.ServerURL)
	knowledgesummary := knowledgesummary.NewHandler(c.services.GPTClient)
	toolInfo := toolinfo.New(c.services.GPTClient)
	threads := threads.NewHandler(c.services.GPTClient, c.services.Invoker, c.services.MCPLoader)
	credentialCleanup := cleanup.NewCredentials(c.services.GPTClient, c.services.MCPLoader, c.services.GatewayClient, c.services.ServerURL, c.services.InternalServerURL)
	projects := projects.NewHandler()
	runstates := runstates.NewHandler(c.services.GatewayClient)
	userCleanup := cleanup.NewUserCleanup(c.services.GatewayClient, c.services.AccessControlRuleHelper)
	mcpCatalog := mcpcatalog.New(c.services.DefaultMCPCatalogPath, c.services.GatewayClient, c.services.AccessControlRuleHelper)
	mcpSession := mcpsession.New(c.services.GPTClient)
	mcpserver := mcpserver.New(c.services.GPTClient, c.services.MCPLoader, c.services.ServerURL)
	mcpserverinstance := mcpserverinstance.New(c.services.GatewayClient)
	accesscontrolrule := accesscontrolrule.New(c.services.AccessControlRuleHelper)
	mcpWebhookValidations := mcpwebhookvalidation.New()
	powerUserWorkspaceHandler := poweruserworkspace.NewHandler(c.services.GatewayClient)
	adminWorkspaceHandler := adminworkspace.New(c.services.GatewayClient)
	mcpServerCatalogEntryHandler := mcpservercatalogentry.NewHandler(c.services.GPTClient)
	auditLogExportHandler := auditlogexport.NewHandler(c.services.GPTClient, c.services.GatewayClient, c.services.EncryptionConfig)
	scheduledAuditLogExportHandler := scheduledauditlogexport.NewHandler()
	oauthclients := oauthclients.NewHandler(c.services.GPTClient)
	projectMCPServerHandler := projectmcpserver.NewHandler()
	systemMCPServerHandler := systemmcpserver.New(c.services.GPTClient, c.services.MCPLoader, c.services.ServerURL)
	nanobotAgentHandler := nanobotagent.New(c.services.GPTClient, c.services.PersistentTokenServer, c.services.GatewayClient, c.services.MCPRemoteShimBaseImage, c.services.ServerURL, c.services.MCPLoader)

	// Runs
	root.Type(&v1.Run{}).FinalizeFunc(v1.RunFinalizer, runs.DeleteRunState)
	root.Type(&v1.Run{}).HandlerFunc(runs.DeleteFinished)
	root.Type(&v1.Run{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Run{}).HandlerFunc(runs.Resume)

	// Migrate RunStates
	root.Type(&v1.RunState{}).HandlerFunc(runstates.Migrate)

	// Threads
	root.Type(&v1.Thread{}).HandlerFunc(retention.Migrate)
	root.Type(&v1.Thread{}).HandlerFunc(retention.RunRetention(c.services.RetentionPolicy))
	root.Type(&v1.Thread{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CreateWorkspaces)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CreateSharedWorkspace)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CreateKnowledgeSet)
	root.Type(&v1.Thread{}).HandlerFunc(threads.WorkflowState)
	root.Type(&v1.Thread{}).HandlerFunc(knowledgesummary.Summarize)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CleanupEphemeralThreads)
	root.Type(&v1.Thread{}).HandlerFunc(threads.GenerateName)
	root.Type(&v1.Thread{}).HandlerFunc(projects.CopyProjectInfo)
	root.Type(&v1.Thread{}).HandlerFunc(projects.CleanupChatbots)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CopyTasksFromSource)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CopyToolsFromSource)
	root.Type(&v1.Thread{}).HandlerFunc(threads.UpgradeThread)
	root.Type(&v1.Thread{}).HandlerFunc(threads.EnsurePublicID)
	root.Type(&v1.Thread{}).HandlerFunc(threads.EnsureUpgradeAvailable)
	root.Type(&v1.Thread{}).HandlerFunc(threads.EnsureLatestConfigRevision)
	root.Type(&v1.Thread{}).HandlerFunc(threads.SetCreated)
	root.Type(&v1.Thread{}).HandlerFunc(threads.EnsureTemplateThreadShare)
	root.Type(&v1.Thread{}).HandlerFunc(threads.RemoveOldFinalizers)
	root.Type(&v1.Thread{}).FinalizeFunc(v1.ThreadFinalizer, credentialCleanup.Remove)

	// KnowledgeSummary
	root.Type(&v1.KnowledgeSummary{}).HandlerFunc(cleanup.Cleanup)

	// Workflows
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.EnsureIDs)
	root.Type(&v1.Workflow{}).HandlerFunc(threads.EnsureShared)
	root.Type(&v1.Workflow{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Workflow{}).FinalizeFunc(v1.WorkflowFinalizer, credentialCleanup.Remove)

	// WorkflowExecutions
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(workflowExecution.Run)
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(workflowExecution.UpdateRun)
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(workflowExecution.ReassignThread)

	// Agents
	root.Type(&v1.Agent{}).HandlerFunc(modelaccesspolicy.MigrateAgentAllowedModels)
	root.Type(&v1.Agent{}).HandlerFunc(modelaccesspolicy.MigrateAgentDefaultModel)
	root.Type(&v1.Agent{}).HandlerFunc(agents.CreateWorkspaceAndKnowledgeSet)
	root.Type(&v1.Agent{}).HandlerFunc(agents.BackPopulateAuthStatus)
	root.Type(&v1.Agent{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.Agent{}).HandlerFunc(toolInfo.SetToolInfoStatus)
	root.Type(&v1.Agent{}).HandlerFunc(toolInfo.RemoveUnneededCredentials)
	root.Type(&v1.Agent{}).HandlerFunc(generationed.UpdateObservedGeneration)
	root.Type(&v1.Agent{}).FinalizeFunc(v1.AgentFinalizer, credentialCleanup.Remove)

	// Uploads
	root.Type(&v1.KnowledgeSource{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeSource{}).FinalizeFunc(v1.KnowledgeSourceFinalizer, knowledgesource.Cleanup)
	root.Type(&v1.KnowledgeSource{}).HandlerFunc(knowledgesource.Reschedule)
	root.Type(&v1.KnowledgeSource{}).HandlerFunc(knowledgesource.Sync)

	// ToolReferences
	root.Type(&v1.ToolReference{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.ToolReference{}).HandlerFunc(toolRef.Populate)
	root.Type(&v1.ToolReference{}).HandlerFunc(toolRef.BackPopulateModels)
	root.Type(&v1.ToolReference{}).FinalizeFunc(v1.ToolReferenceFinalizer, toolRef.CleanupModelProvider)

	// Models
	root.Type(&v1.Model{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.Model{}).HandlerFunc(generationed.UpdateObservedGeneration)

	// DefaultModelAliases
	root.Type(&v1.DefaultModelAlias{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.DefaultModelAlias{}).HandlerFunc(generationed.UpdateObservedGeneration)

	// Knowledge files
	root.Type(&v1.KnowledgeFile{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeFile{}).FinalizeFunc(v1.KnowledgeFileFinalizer, knowledgefile.Cleanup)
	root.Type(&v1.KnowledgeFile{}).HandlerFunc(knowledgefile.IngestFile)
	root.Type(&v1.KnowledgeFile{}).HandlerFunc(knowledgefile.Unapproved)

	// Workspaces
	root.Type(&v1.Workspace{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Workspace{}).FinalizeFunc(v1.WorkspaceFinalizer, workspace.RemoveWorkspace)
	root.Type(&v1.Workspace{}).HandlerFunc(workspace.CreateWorkspace)

	// KnowledgeSets
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeSet{}).FinalizeFunc(v1.KnowledgeSetFinalizer, knowledgeset.Cleanup)
	// Also cleanup the dataset when there is no content.
	// This will allow the user to switch the embedding model implicitly.
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.Cleanup)
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.CreateWorkspace)
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.CheckHasContent)
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.SetEmbeddingModel)

	// Cronjobs
	root.Type(&v1.CronJob{}).HandlerFunc(cronJobs.SetSuccessRunTime)
	root.Type(&v1.CronJob{}).HandlerFunc(cronJobs.Run)
	root.Type(&v1.CronJob{}).HandlerFunc(cleanup.Cleanup)

	// OAuthApps
	root.Type(&v1.OAuthApp{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.OAuthApp{}).HandlerFunc(alias.AssignAlias)

	// OAuthAppLogins
	root.Type(&v1.OAuthAppLogin{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.OAuthAppLogin{}).HandlerFunc(oauthLogins.RunTool)

	// Alias
	root.Type(&v1.Alias{}).HandlerFunc(alias.UnassignAlias)

	// Thread Authorizations
	root.Type(&v1.ThreadAuthorization{}).HandlerFunc(cleanup.Cleanup)

	// ThreadShare
	root.Type(&v1.ThreadShare{}).HandlerFunc(threadshare.CopyProjectInfo)
	root.Type(&v1.ThreadShare{}).HandlerFunc(cleanup.Cleanup)

	// WorkflowSteps
	root.Type(&v1.WorkflowStep{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.WorkflowStep{}).HandlerFunc(handlers.GCOrphans)
	root.Type(&v1.WorkflowStep{}).Middleware(workflowStep.Preconditions).HandlerFunc(workflowStep.RunInvoke)
	root.Type(&v1.WorkflowStep{}).Middleware(workflowStep.Preconditions).HandlerFunc(workflowStep.RunLoop)

	// Tools
	root.Type(&v1.Tool{}).HandlerFunc(cleanup.Cleanup)

	// User Cleanup
	root.Type(&v1.UserDelete{}).HandlerFunc(userCleanup.Cleanup)

	// MCPCatalog
	root.Type(&v1.MCPCatalog{}).HandlerFunc(mcpCatalog.Sync)
	root.Type(&v1.MCPCatalog{}).HandlerFunc(mcpCatalog.DeleteUnauthorizedMCPServersForCatalog)
	root.Type(&v1.MCPCatalog{}).HandlerFunc(mcpCatalog.DeleteUnauthorizedMCPServerInstancesForCatalog)

	// MCPServerCatalogEntry
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.MCPServerCatalogEntry{}).FinalizeFunc(v1.MCPServerCatalogEntryFinalizer, mcpServerCatalogEntryHandler.RemoveOAuthCredentials)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.DeleteEntriesWithoutRuntime)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.UpdateManifestHashAndLastUpdated)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.CleanupNestedCompositeEntries)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.DetectCompositeDrift)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.EnsureUserCount)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.CleanupUnusedOAuthCredentials)
	root.Type(&v1.MCPServerCatalogEntry{}).HandlerFunc(mcpServerCatalogEntryHandler.EnsureOAuthCredentialStatus)

	// MCPServer
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.EnsureMCPCatalogID)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.MigrateSharedWithinMCPCatalogName)
	root.Type(&v1.MCPServer{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.DeleteServersWithoutRuntime)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.DeleteServersForAnonymousUser)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.CleanupNestedCompositeServers)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.DetectDrift)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.DetectK8sSettingsDrift)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.EnsureMCPServerInstanceUserCount)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.SyncOAuthCredentialStatus)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.EnsureMCPServerSecretInfo)
	root.Type(&v1.MCPServer{}).HandlerFunc(mcpserver.EnsureCompositeComponents)
	root.Type(&v1.MCPServer{}).FinalizeFunc(v1.MCPServerFinalizer, credentialCleanup.RemoveMCPCredentials)

	// MCPServerInstance
	root.Type(&v1.MCPServerInstance{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.MCPServerInstance{}).HandlerFunc(mcpserverinstance.MigrationDeleteSingleUserInstances)
	root.Type(&v1.MCPServerInstance{}).FinalizeFunc(v1.MCPServerInstanceFinalizer, mcpserverinstance.RemoveOAuthToken)

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

	// ProjectInvitations
	root.Type(&v1.ProjectInvitation{}).HandlerFunc(projectinvitation.SetRespondedTime)
	root.Type(&v1.ProjectInvitation{}).HandlerFunc(projectinvitation.Expiration)
	root.Type(&v1.ProjectInvitation{}).HandlerFunc(projectinvitation.Cleanup)
	root.Type(&v1.ProjectInvitation{}).HandlerFunc(cleanup.Cleanup)

	// OAuthClients
	root.Type(&v1.OAuthClient{}).HandlerFunc(cleanup.OAuthClients)
	root.Type(&v1.OAuthClient{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.OAuthClient{}).FinalizeFunc(v1.OAuthClientFinalizer, oauthclients.CleanupOAuthClientCred)

	// OAuthAuthRequests
	root.Type(&v1.OAuthAuthRequest{}).HandlerFunc(cleanup.OAuthAuth)
	root.Type(&v1.OAuthAuthRequest{}).HandlerFunc(cleanup.Cleanup)

	// OAuthTokens
	root.Type(&v1.OAuthToken{}).HandlerFunc(cleanup.Cleanup)

	// MCP Sessions
	root.Type(&v1.MCPSession{}).HandlerFunc(mcpSession.RemoveUnused)
	root.Type(&v1.MCPSession{}).FinalizeFunc(v1.MCPSessionFinalizer, mcpSession.CleanupCredentials)

	// MCP Webhook Validations
	root.Type(&v1.MCPWebhookValidation{}).HandlerFunc(mcpWebhookValidations.CleanupResources)

	// UserRoleChange
	root.Type(&v1.UserRoleChange{}).HandlerFunc(powerUserWorkspaceHandler.HandleRoleChange)

	// UserGroupChange
	root.Type(&v1.UserGroupChange{}).HandlerFunc(mcpCatalog.HandleUserGroupChange)

	// GroupRoleChange
	root.Type(&v1.GroupRoleChange{}).HandlerFunc(powerUserWorkspaceHandler.HandleGroupRoleChange)

	// PowerUserWorkspace
	root.Type(&v1.PowerUserWorkspace{}).HandlerFunc(powerUserWorkspaceHandler.CreateACR)
	root.Type(&v1.PowerUserWorkspace{}).HandlerFunc(mcpCatalog.DeleteUnauthorizedMCPServersForWorkspace)
	root.Type(&v1.PowerUserWorkspace{}).HandlerFunc(mcpCatalog.DeleteUnauthorizedMCPServerInstancesForWorkspace)

	// Project-based MCP Servers
	root.Type(&v1.ProjectMCPServer{}).HandlerFunc(projectMCPServerHandler.EnsureMCPServerName)
	root.Type(&v1.ProjectMCPServer{}).FinalizeFunc(v1.ProjectMCPServerFinalizer, credentialCleanup.ShutdownProjectMCP)
	root.Type(&v1.ProjectMCPServer{}).HandlerFunc(cleanup.Cleanup)

	// System MCP Servers
	root.Type(&v1.SystemMCPServer{}).HandlerFunc(systemMCPServerHandler.EnsureSecretInfo)
	root.Type(&v1.SystemMCPServer{}).HandlerFunc(systemMCPServerHandler.EnsureDeployment)
	root.Type(&v1.SystemMCPServer{}).FinalizeFunc(v1.SystemMCPServerFinalizer, systemMCPServerHandler.CleanupDeployment)

	// AuditLogExport
	root.Type(&v1.AuditLogExport{}).HandlerFunc(auditLogExportHandler.ExportAuditLogs)

	// ScheduledAuditLogExport
	root.Type(&v1.ScheduledAuditLogExport{}).HandlerFunc(scheduledAuditLogExportHandler.ScheduleExports)

	// NanobotAgent
	if c.services.NanobotIntegration {
		root.Type(&v1.NanobotAgent{}).HandlerFunc(nanobotAgentHandler.CreateMCPServer)
		root.Type(&v1.NanobotAgent{}).HandlerFunc(cleanup.Cleanup)
		root.Type(&v1.NanobotAgent{}).FinalizeFunc(v1.NanobotAgentFinalizer, nanobotAgentHandler.Cleanup)
	} else {
		root.Type(&v1.NanobotAgent{}).HandlerFunc(nanobotAgentHandler.DeleteMCPServer)
	}

	c.toolRefHandler = toolRef
	c.mcpCatalogHandler = mcpCatalog
	c.adminWorkspaceHandler = adminWorkspaceHandler
}
