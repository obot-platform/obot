package router

import (
	"net/http"

	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/api/handlers/mcpgateway"
	"github.com/obot-platform/obot/pkg/api/handlers/mcpgateway/oauth"
	"github.com/obot-platform/obot/pkg/api/handlers/sendgrid"
	"github.com/obot-platform/obot/pkg/api/handlers/wellknown"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/obot-platform/obot/ui"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/component-base/metrics/legacyregistry"
)

func Router(services *services.Services) (http.Handler, error) {
	mux := services.APIServer

	oauthChecker := oauth.NewMCPOAuthHandlerFactory(services.ServerURL, services.MCPLoader, services.StorageClient, services.GPTClient, services.GatewayClient, services.MCPOAuthTokenStorage)

	agents := handlers.NewAgentHandler(services.EphemeralTokenServer, services.ProviderDispatcher, services.MCPLoader, services.Invoker, services.ServerURL)
	assistants := handlers.NewAssistantHandler(services.ProviderDispatcher, services.Invoker, services.Events, services.Router.Backend())
	tools := handlers.NewToolHandler(services.Invoker)
	tasks := handlers.NewTaskHandler(services.Invoker, services.Events)
	invoker := handlers.NewInvokeHandler(services.Invoker)
	threads := handlers.NewThreadHandler(services.ProviderDispatcher, services.Events)
	runs := handlers.NewRunHandler(services.Events)
	toolRefs := handlers.NewToolReferenceHandler()
	webhooks := handlers.NewWebhookHandler()
	cronJobs := handlers.NewCronJobHandler()
	models := handlers.NewModelHandler()
	mcpCatalogs := handlers.NewMCPCatalogHandler(services.DefaultMCPCatalogPath, services.ServerURL, services.MCPLoader, oauthChecker, services.GatewayClient, services.AccessControlRuleHelper)
	accessControlRules := handlers.NewAccessControlRuleHandler()
	powerUserWorkspaces := handlers.NewPowerUserWorkspaceHandler(services.ServerURL, services.AccessControlRuleHelper)
	mcpWebhookValidations := handlers.NewMCPWebhookValidationHandler()
	availableModels := handlers.NewAvailableModelsHandler(services.ProviderDispatcher)
	modelProviders := handlers.NewModelProviderHandler(services.ProviderDispatcher, services.Invoker)
	authProviders := handlers.NewAuthProviderHandler(services.ProviderDispatcher, services.PostgresDSN)
	fileScannerProviders := handlers.NewFileScannerProviderHandler(services.ProviderDispatcher, services.Invoker)
	prompt := handlers.NewPromptHandler()
	emailReceiver := handlers.NewEmailReceiverHandler(services.EmailServerName)
	defaultModelAliases := handlers.NewDefaultModelAliasHandler()
	version := handlers.NewVersionHandler(services.EmailServerName, services.PostgresDSN, services.SupportDocker, services.AuthEnabled)
	projects := handlers.NewProjectsHandler(services.Router.Backend(), services.Invoker)
	projectShare := handlers.NewProjectShareHandler()
	templates := handlers.NewTemplateHandler()
	files := handlers.NewFilesHandler(services.ProviderDispatcher)
	memories := handlers.NewMemoryHandler()
	workflows := handlers.NewWorkflowHandler()
	slackEventHandler := handlers.NewSlackEventHandler()
	sendgridWebhookHandler := sendgrid.NewInboundWebhookHandler(services.StorageClient, services.EmailServerName, services.SendgridWebhookUsername, services.SendgridWebhookPassword)
	images := handlers.NewImageHandler(services.GeminiClient)
	slackHandler := handlers.NewSlackHandler()
	mcp := handlers.NewMCPHandler(services.MCPLoader, services.AccessControlRuleHelper, oauthChecker, services.ServerURL)
	projectMCP := handlers.NewProjectMCPHandler(services.MCPLoader, services.AccessControlRuleHelper, services.EphemeralTokenServer, oauthChecker, services.ServerURL)
	projectInvitations := handlers.NewProjectInvitationHandler()
	mcpGateway := mcpgateway.NewHandler(services.StorageClient, services.MCPLoader, services.WebhookHelper, services.MCPOAuthTokenStorage, services.GatewayClient, services.GPTClient, services.ServerURL)
	mcpAuditLogs := mcpgateway.NewAuditLogHandler()
	serverInstances := handlers.NewServerInstancesHandler(services.AccessControlRuleHelper, services.ServerURL)
	userDefaultRoleSettings := handlers.NewUserDefaultRoleSettingHandler()

	// Version
	mux.HandleFunc("GET /api/version", version.GetVersion)

	// Agents
	mux.HandleFunc("POST /api/agents", agents.Create)
	mux.HandleFunc("GET /api/agents", agents.List)
	mux.HandleFunc("GET /api/agents/{id}", agents.ByID)
	mux.HandleFunc("DELETE /api/agents/{id}", agents.Delete)
	mux.HandleFunc("PUT /api/agents/{id}", agents.Update)
	mux.HandleFunc("POST /api/agents/{id}/authenticate", agents.Authenticate)
	mux.HandleFunc("POST /api/agents/{id}/deauthenticate", agents.DeAuthenticate)
	mux.HandleFunc("POST /api/agents/{id}/oauth-credentials/{ref}/login", agents.EnsureCredentialForKnowledgeSource)
	mux.HandleFunc("GET /api/agents/{id}/script", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/script.gpt", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/script/tool.gpt", agents.Script)
	mux.HandleFunc("PUT /api/agents/{id}/setdefault", agents.SetDefault)
	mux.HandleFunc("GET /api/agents/{id}/threads/{thread_id}/script", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/threads/{thread_id}/script.gpt", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/threads/{thread_id}/script/tool.gpt", agents.Script)

	// Top Level Projects
	mux.HandleFunc("GET /api/projects", projects.ListProjects)
	mux.HandleFunc("GET /api/projects/{project_id}", projects.GetProject)

	// ThreadShare
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/share", projectShare.CreateShare)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/share", projectShare.DeleteShare)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/share", projectShare.GetShare)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/share", projectShare.UpdateShare)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/featured", projectShare.SetFeatured)
	mux.HandleFunc("GET /api/shares", projectShare.ListShares)
	mux.HandleFunc("POST /api/shares/{share_public_id}", projectShare.CreateProjectFromShare)
	mux.HandleFunc("GET /api/shares/{share_public_id}", projectShare.GetShareFromShareID)

	// Assistants
	mux.HandleFunc("GET /api/assistants", assistants.List)
	mux.HandleFunc("GET /api/assistants/{id}", assistants.Get)

	// Project Creds
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/credentials", projects.ListCredentials)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tools/{tools}/authenticate", projects.Authenticate)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tools}/deauthenticate", projects.DeAuthenticate)

	// Project Local Creds
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/local-credentials", projects.ListLocalCredentials)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tools/{tools}/local-authenticate", projects.LocalAuthenticate)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tools}/local-deauthenticate", projects.LocalDeAuthenticate)

	// Project thread control
	mux.HandleFunc("POST /api/assistants/{id}/projects/{project_id}/threads/{thread_id}/abort", assistants.Abort)
	mux.HandleFunc("GET /api/assistants/{id}/projects/{project_id}/threads/{thread_id}/events", assistants.Events)
	mux.HandleFunc("POST /api/assistants/{id}/projects/{project_id}/threads/{thread_id}/invoke", assistants.Invoke)

	// Project tools
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/tools", assistants.SetTools)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tools", assistants.Tools)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}", tools.Get)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool}", assistants.RemoveTool)

	// Project files
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}", files.GetFile)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/files/{file...}", files.GetFile)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}", files.UploadFile)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/files/{file...}", files.UploadFile)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/files", files.Files)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}", files.DeleteFile)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/files/{file...}", files.DeleteFile)

	// Project Knowledge files
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/knowledge", assistants.Knowledge)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file...}", assistants.GetKnowledgeFile)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file...}", assistants.DeleteKnowledge)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file}", assistants.UploadKnowledge)

	// Project Env
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/env", assistants.GetEnv)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/env", assistants.SetEnv)

	// Project Slack integration
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/slack", slackHandler.Get)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/slack", slackHandler.Create)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/slack", slackHandler.Update)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/slack", slackHandler.Delete)

	// Top level Tasks
	mux.HandleFunc("GET /api/tasks", tasks.List)
	mux.HandleFunc("DELETE /api/tasks/{id}", tasks.Delete)
	mux.HandleFunc("GET /api/tasks/{id}", tasks.Get)
	mux.HandleFunc("PUT /api/tasks/{id}", tasks.Update)
	mux.HandleFunc("GET /api/tasks/{id}/files", agents.ListFiles)

	// These can be removed when we get rid of the legacy admin side of things.
	mux.HandleFunc("GET /api/tasks/{id}/file/{file...}", agents.GetFile)
	mux.HandleFunc("GET /api/tasks/{id}/files/{file...}", agents.GetFile)
	mux.HandleFunc("DELETE /api/tasks/{id}/file/{file...}", agents.DeleteFile)
	mux.HandleFunc("DELETE /api/tasks/{id}/files/{file...}", agents.DeleteFile)
	mux.HandleFunc("POST /api/tasks/{id}/file/{file...}", agents.UploadFile)
	mux.HandleFunc("POST /api/tasks/{id}/files/{file...}", agents.UploadFile)
	mux.HandleFunc("POST /api/tasks/{id}/run", tasks.Run)
	mux.HandleFunc("DELETE /api/tasks/{id}/runs/{run_id}", tasks.DeleteRun)
	mux.HandleFunc("POST /api/tasks/{id}/runs/{run_id}/abort", tasks.AbortRun)
	mux.HandleFunc("POST /api/tasks/{id}/runs/{run_id}/events", tasks.Abort)
	mux.HandleFunc("GET /api/tasks/{id}/runs/{run_id}/events", tasks.Events)
	//

	// Project Tasks
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks", tasks.CreateFromScope)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks", tasks.ListFromScope)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}", tasks.DeleteFromScope)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}", tasks.GetFromScope)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}", tasks.UpdateFromScope)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/run", tasks.RunFromScope)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}/steps/{step_id}/run", tasks.RunFromScope)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs", tasks.ListRunsFromScope)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}", tasks.DeleteRunFromScope)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}", tasks.GetRunFromScope)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}/abort", tasks.AbortRunFromScope)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}/events", tasks.AbortFromScope)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}/events", tasks.EventsFromScope)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/file/{file...}", files.GetFile)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files/{file...}", files.GetFile)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/file/{file...}", files.UploadFile)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files/{file...}", files.UploadFile)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files", files.Files)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/file/{file...}", files.DeleteFile)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files/{file...}", files.DeleteFile)

	// Top level Thread Tasks
	mux.HandleFunc("POST /api/threads/{thread_id}/tasks", tasks.CreateFromScope)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks", tasks.ListFromScope)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks/{id}", tasks.GetFromScope)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks/{id}/runs", tasks.ListRunsFromScope)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks/{id}/runs/{run_id}", tasks.GetRunFromScope)

	// These can be removed when we get rid of the legacy admin side of things
	mux.HandleFunc("PUT /api/threads/{thread_id}/tasks/{id}", tasks.UpdateFromScope)
	mux.HandleFunc("POST /api/threads/{thread_id}/tasks/{id}/run", tasks.RunFromScope)
	//

	// Projects in Project
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects", projects.CreateProject)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects", projects.ListProjects)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}", projects.DeleteProject)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}", projects.GetProject)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}", projects.UpdateProject)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/copy", projects.CopyProject)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/default-model", projects.GetDefaultModelForProject)

	// Project Threads
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/threads", projects.CreateProjectThread)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/threads", projects.ListProjectThreads)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/threads/{id}", projects.GetProjectThread)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/threads/{id}", threads.Update)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/default-model", threads.GetDefaultModelForThread)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}", projects.DeleteProjectThread)

	// Project Members
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/members", projects.ListMembers)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/members/{member_id}", projects.DeleteMember)

	// Project Invitations
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/invitations", projectInvitations.CreateInvitationForProject)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/invitations", projectInvitations.ListInvitationsForProject)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/invitations/{code}", projectInvitations.DeleteInvitationForProject)
	mux.HandleFunc("GET /api/projectinvitations/{code}", projectInvitations.GetInvitation)
	mux.HandleFunc("POST /api/projectinvitations/{code}", projectInvitations.AcceptInvitation)
	mux.HandleFunc("DELETE /api/projectinvitations/{code}", projectInvitations.RejectInvitation)

	// Project Memories
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/memories", memories.CreateMemory)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/memories/{memory_id}", memories.UpdateMemory)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/memories", memories.ListMemories)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/memories", memories.DeleteMemories)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/memories/{memory_id}", memories.DeleteMemories)

	// Project Templates
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/template", templates.CreateProjectTemplate)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/template", templates.GetProjectTemplate)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/template", templates.DeleteProjectTemplate)
	mux.HandleFunc("GET /api/templates/{template_public_id}", templates.GetTemplate)
	mux.HandleFunc("POST /api/templates/{template_public_id}", templates.CopyTemplate)

	// Project upgrade from template
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/upgrade-from-template", projects.UpgradeFromTemplate)

	// Project model providers
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/model-providers", modelProviders.List)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/model-providers/{model_provider_id}/configure", modelProviders.Configure)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/model-providers/{model_provider_id}/deconfigure", modelProviders.Deconfigure)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/model-providers/{model_provider_id}/reveal", modelProviders.Reveal)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/model-providers/{model_provider_id}/validate", modelProviders.Validate)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/model-providers/{model_provider_id}/available-models", availableModels.ListForModelProvider)

	// Agent files
	mux.HandleFunc("GET /api/agents/{id}/file/{file...}", agents.GetFile)
	mux.HandleFunc("GET /api/agents/{id}/files/{file...}", agents.GetFile)
	mux.HandleFunc("GET /api/agents/{id}/files", agents.ListFiles)
	mux.HandleFunc("DELETE /api/agents/{id}/file/{file...}", agents.DeleteFile)
	mux.HandleFunc("DELETE /api/agents/{id}/files/{file...}", agents.DeleteFile)
	mux.HandleFunc("POST /api/agents/{id}/file/{file...}", agents.UploadFile)
	mux.HandleFunc("POST /api/agents/{id}/files/{file...}", agents.UploadFile)

	// Agent knowledge files
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-files", agents.ListKnowledgeFiles)
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-files/{file}", agents.GetKnowledgeFile)
	mux.HandleFunc("POST /api/agents/{agent_id}/knowledge-files/{file_id}/ingest", agents.ReIngestKnowledgeFile)
	mux.HandleFunc("DELETE /api/agents/{id}/knowledge-files/{file...}", agents.DeleteKnowledgeFile)
	mux.HandleFunc("POST /api/agents/{id}/knowledge-files/{file...}", agents.UploadKnowledgeFile)

	// Agent approve file
	mux.HandleFunc("POST /api/agents/{agent_id}/approve-file/{file_id}", agents.ApproveKnowledgeFile)

	// Remote Knowledge Sources
	mux.HandleFunc("POST /api/agents/{agent_id}/knowledge-sources", agents.CreateKnowledgeSource)
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-sources", agents.ListKnowledgeSources)
	mux.HandleFunc("DELETE /api/agents/{agent_id}/knowledge-sources/{id}", agents.DeleteKnowledgeSource)
	mux.HandleFunc("PUT /api/agents/{agent_id}/knowledge-sources/{id}", agents.UpdateKnowledgeSource)
	mux.HandleFunc("POST /api/agents/{agent_id}/knowledge-sources/{id}/sync", agents.ReSyncKnowledgeSource)
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-sources/{knowledge_source_id}/knowledge-files", agents.ListKnowledgeFiles)
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-sources/{knowledge_source_id}/knowledge-files/watch", agents.WatchKnowledgeFile)
	mux.HandleFunc("POST /api/agents/{agent_id}/knowledge-sources/{knowledge_source_id}/knowledge-files/{file_id}/ingest", agents.ReIngestKnowledgeFile)

	// Invoker
	// We can remove these endpoints when we get rid of the legacy admin side of things
	mux.HandleFunc("POST /api/invoke/{id}", invoker.Invoke)
	mux.HandleFunc("POST /api/invoke/{id}/thread/{thread}", invoker.Invoke)
	mux.HandleFunc("POST /api/invoke/{id}/threads/{thread}", invoker.Invoke)
	//

	// Threads
	mux.HandleFunc("GET /api/agents/{agent}/threads", threads.List)
	mux.HandleFunc("GET /api/threads", threads.List)
	mux.HandleFunc("GET /api/threads/{id}", threads.ByID)
	mux.HandleFunc("DELETE /api/threads/{id}", threads.Delete)
	mux.HandleFunc("PUT /api/threads/{id}", threads.Update)
	mux.HandleFunc("POST /api/threads/{id}/abort", threads.Abort)
	mux.HandleFunc("GET /api/threads/{id}/events", threads.Events)

	// Project Thread Tools
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/tools", assistants.Tools)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/tools", assistants.SetTools)

	// Project Thread Files
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/files", files.Files)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/file/{file...}", files.DeleteFile)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/files/{file...}", files.DeleteFile)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/file/{file...}", files.GetFile)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/files/{file...}", files.GetFile)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/file/{file...}", files.UploadFile)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/files/{file...}", files.UploadFile)

	// Project Thread knowledge files
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/threads/{id}/knowledge-files", threads.Knowledge)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/threads/{id}/knowledge-files/{file...}", threads.GetKnowledgeFile)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{id}/knowledge-files/{file...}", threads.DeleteKnowledge)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/threads/{id}/knowledge-files/{file}", threads.UploadKnowledge)

	// Thread files
	mux.HandleFunc("GET /api/threads/{thread_id}/files", files.Files)
	mux.HandleFunc("DELETE /api/threads/{thread_id}/file/{file...}", files.DeleteFile)
	mux.HandleFunc("DELETE /api/threads/{thread_id}/files/{file...}", files.DeleteFile)
	mux.HandleFunc("GET /api/threads/{thread_id}/file/{file...}", files.GetFile)
	mux.HandleFunc("GET /api/threads/{thread_id}/files/{file...}", files.GetFile)
	mux.HandleFunc("POST /api/threads/{thread_id}/file/{file...}", files.UploadFile)
	mux.HandleFunc("POST /api/threads/{thread_id}/files/{file...}", files.UploadFile)

	// Thread knowledge files
	mux.HandleFunc("GET /api/threads/{id}/knowledge-files", threads.Knowledge)
	mux.HandleFunc("GET /api/threads/{id}/knowledge-files/{file...}", threads.GetKnowledgeFile)
	mux.HandleFunc("DELETE /api/threads/{id}/knowledge-files/{file...}", threads.DeleteKnowledge)
	mux.HandleFunc("POST /api/threads/{id}/knowledge-files/{file}", threads.UploadKnowledge)

	// ToolRefs
	mux.HandleFunc("POST /api/tool-references", toolRefs.Create)
	mux.HandleFunc("GET /api/tool-references", toolRefs.List)
	mux.HandleFunc("GET /api/tool-references/{id}", toolRefs.ByID)
	mux.HandleFunc("DELETE /api/tool-references/{id}", toolRefs.Delete)
	mux.HandleFunc("PUT /api/tool-references/{id}", toolRefs.Update)
	mux.HandleFunc("POST /api/tool-references/{id}/force-refresh", toolRefs.ForceRefresh)

	// Runs
	mux.HandleFunc("GET /api/agents/{agent}/runs", runs.List)
	mux.HandleFunc("GET /api/agents/{agent}/threads/{thread}/runs", runs.List)
	mux.HandleFunc("GET /api/runs", runs.List)
	mux.HandleFunc("GET /api/runs/{id}", runs.ByID)
	mux.HandleFunc("GET /api/threads/{thread}/runs", runs.List)

	// We can remove these endpoints when we get rid of the legacy admin side of things
	mux.HandleFunc("DELETE /api/runs/{id}", runs.Delete)
	mux.HandleFunc("GET /api/runs/{id}/debug", runs.Debug)
	mux.HandleFunc("GET /api/runs/{id}/events", runs.Events)
	//

	// Credentials
	mux.HandleFunc("GET /api/agents/{context}/credentials", handlers.ListCredentials)
	mux.HandleFunc("DELETE /api/agents/{context}/credentials/{id}", handlers.DeleteCredential)
	mux.HandleFunc("GET /api/credentials", handlers.ListCredentials)
	mux.HandleFunc("DELETE /api/credentials/{id}", handlers.DeleteCredential)
	mux.HandleFunc("POST /api/credentials/recreate-all", handlers.RecreateAllCredentials)
	mux.HandleFunc("GET /api/threads/{context}/credentials", handlers.ListCredentials)
	mux.HandleFunc("DELETE /api/threads/{context}/credentials/{id}", handlers.DeleteCredential)

	// Environment variable credentials
	mux.HandleFunc("GET /api/agents/{id}/env", handlers.RevealEnv)
	mux.HandleFunc("POST /api/agents/{id}/env", handlers.SetEnv)

	// Webhooks
	mux.HandleFunc("POST /api/webhooks", webhooks.Create)
	mux.HandleFunc("GET /api/webhooks", webhooks.List)
	mux.HandleFunc("GET /api/webhooks/{id}", webhooks.ByID)
	mux.HandleFunc("DELETE /api/webhooks/{id}", webhooks.Delete)
	mux.HandleFunc("PUT /api/webhooks/{id}", webhooks.Update)
	mux.HandleFunc("POST /api/webhooks/{id}/remove-token", webhooks.RemoveToken)
	mux.HandleFunc("POST /api/webhooks/{namespace}/{id}", webhooks.Execute)

	// Webhook for third party integration to trigger workflow
	mux.HandleFunc("POST /api/sendgrid", sendgridWebhookHandler.InboundWebhookHandler)

	// Email Receivers
	mux.HandleFunc("POST /api/email-receivers", emailReceiver.Create)
	mux.HandleFunc("GET /api/email-receivers", emailReceiver.List)
	mux.HandleFunc("GET /api/email-receivers/{id}", emailReceiver.ByID)
	mux.HandleFunc("DELETE /api/email-receivers/{id}", emailReceiver.Delete)
	mux.HandleFunc("PUT /api/email-receivers/{id}", emailReceiver.Update)

	// CronJobs
	mux.HandleFunc("POST /api/cronjobs", cronJobs.Create)
	mux.HandleFunc("GET /api/cronjobs", cronJobs.List)
	mux.HandleFunc("GET /api/cronjobs/{id}", cronJobs.ByID)
	mux.HandleFunc("DELETE /api/cronjobs/{id}", cronJobs.Delete)
	mux.HandleFunc("POST /api/cronjobs/{id}", cronJobs.Execute)
	mux.HandleFunc("PUT /api/cronjobs/{id}", cronJobs.Update)

	// Slack event receiver
	mux.HandleFunc("POST /api/slack/events", slackEventHandler.HandleEvent)

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

	// User-Deployed MCP Servers (single-user and remote)
	mux.HandleFunc("GET /api/mcp-servers", mcp.ListServer)
	mux.HandleFunc("GET /api/mcp-servers/{mcp_server_id}", mcp.GetServer)
	mux.HandleFunc("POST /api/mcp-servers", mcp.CreateServer)
	mux.HandleFunc("PUT /api/mcp-servers/{mcp_server_id}", mcp.UpdateServer)
	mux.HandleFunc("PUT /api/mcp-servers/{mcp_server_id}/alias", mcp.UpdateServerAlias)
	mux.HandleFunc("DELETE /api/mcp-servers/{mcp_server_id}", mcp.DeleteServer)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/launch", mcp.LaunchServer)
	mux.HandleFunc("POST /api/mcp-servers/{mcp_server_id}/check-oauth", mcp.CheckOAuth)
	mux.HandleFunc("GET /api/mcp-servers/{mcp_server_id}/oauth-url", mcp.GetOAuthURL)
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
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/entries/all-servers", mcpCatalogs.AdminListServersForAllEntriesInCatalog)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/generate-tool-previews", mcpCatalogs.GenerateToolPreviews)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/generate-tool-previews/oauth-url", mcpCatalogs.GenerateToolPreviewsOAuthURL)

	// MCPServers within the catalog (admin only, for multi-user MCP servers)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers", mcp.ListServer)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}", mcp.GetServer)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers", mcp.CreateServer)
	mux.HandleFunc("PUT /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}", mcp.UpdateServer)
	mux.HandleFunc("DELETE /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}", mcp.DeleteServer)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/launch", mcp.LaunchServer)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/check-oauth", mcp.CheckOAuth)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/oauth-url", mcp.GetOAuthURL)
	mux.HandleFunc("DELETE /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/oauth", mcp.ClearOAuthCredentials)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/configure", mcp.ConfigureServer)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/deconfigure", mcp.DeconfigureServer)
	mux.HandleFunc("POST /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/reveal", mcp.Reveal)
	mux.HandleFunc("GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/instances", serverInstances.ListServerInstancesForServer)
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
	mux.HandleFunc("GET /api/workspaces/all-servers/all-instances", powerUserWorkspaces.ListAllServersInstances)

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
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/entries/{entry_id}/generate-tool-previews", mcpCatalogs.GenerateToolPreviews)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/entries/{entry_id}/generate-tool-previews/oauth-url", mcpCatalogs.GenerateToolPreviewsOAuthURL)

	// Workspace-scoped MCP Servers (PowerUserPlus and higher only)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers", mcp.ListServer)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}", mcp.GetServer)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers", mcp.CreateServer)
	mux.HandleFunc("PUT /api/workspaces/{workspace_id}/servers/{mcp_server_id}", mcp.UpdateServer)
	mux.HandleFunc("DELETE /api/workspaces/{workspace_id}/servers/{mcp_server_id}", mcp.DeleteServer)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/launch", mcp.LaunchServer)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/check-oauth", mcp.CheckOAuth)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth-url", mcp.GetOAuthURL)
	mux.HandleFunc("DELETE /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth", mcp.ClearOAuthCredentials)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/configure", mcp.ConfigureServer)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/deconfigure", mcp.DeconfigureServer)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/reveal", mcp.Reveal)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/details", mcp.GetServerDetails)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/logs", mcp.StreamServerLogs)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/restart", mcp.RestartServerDeployment)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/instances", serverInstances.ListServerInstancesForServer)

	// MCP Webhook Validations (admin only)
	mux.HandleFunc("GET /api/mcp-webhook-validations", mcpWebhookValidations.List)
	mux.HandleFunc("GET /api/mcp-webhook-validations/{mcp_webhook_validation_id}", mcpWebhookValidations.Get)
	mux.HandleFunc("POST /api/mcp-webhook-validations", mcpWebhookValidations.Create)
	mux.HandleFunc("PUT /api/mcp-webhook-validations/{mcp_webhook_validation_id}", mcpWebhookValidations.Update)
	mux.HandleFunc("DELETE /api/mcp-webhook-validations/{mcp_webhook_validation_id}", mcpWebhookValidations.Delete)
	mux.HandleFunc("DELETE /api/mcp-webhook-validations/{mcp_webhook_validation_id}/secret", mcpWebhookValidations.RemoveSecret)

	// MCP Gateway Endpoints
	mux.HandleFunc("/mcp-connect/{mcp_id}", mcpGateway.StreamableHTTP)

	// MCP Audit Logs
	mux.HandleFunc("GET /api/mcp-audit-logs", mcpAuditLogs.ListAuditLogs)
	mux.HandleFunc("GET /api/mcp-audit-logs/filter-options/{filter}", mcpAuditLogs.ListAuditLogFilterOptions)
	mux.HandleFunc("GET /api/mcp-audit-logs/{mcp_id}", mcpAuditLogs.ListAuditLogs)
	mux.HandleFunc("GET /api/mcp-stats", mcpAuditLogs.GetUsageStats)
	mux.HandleFunc("GET /api/mcp-stats/{mcp_id}", mcpAuditLogs.GetUsageStats)

	// MCP Servers in projects
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/mcpservers", projectMCP.ListServer)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}", projectMCP.GetServer)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/mcpservers", projectMCP.CreateServer)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}", projectMCP.DeleteServer)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/launch", projectMCP.LaunchServer)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/check-oauth", projectMCP.CheckOAuth)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/oauth-url", projectMCP.GetOAuthURL)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/tools", projectMCP.GetTools)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/tools", projectMCP.SetTools)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/tools/{thread_id}", projectMCP.GetTools)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/tools/{thread_id}", projectMCP.SetTools)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/resources", projectMCP.GetResources)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/resources/{resource_uri}", projectMCP.ReadResource)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/prompts", projectMCP.GetPrompts)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/prompts/{prompt_name}", projectMCP.GetPrompt)

	// User Default Role Settings
	mux.HandleFunc("GET /api/user-default-role-settings", userDefaultRoleSettings.Get)
	mux.HandleFunc("POST /api/user-default-role-settings", userDefaultRoleSettings.Set)

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

	// File scanner providers
	mux.HandleFunc("GET /api/file-scanner-providers", fileScannerProviders.List)
	mux.HandleFunc("GET /api/file-scanner-providers/{id}", fileScannerProviders.ByID)
	mux.HandleFunc("POST /api/file-scanner-providers/{id}/configure", fileScannerProviders.Configure)
	mux.HandleFunc("POST /api/file-scanner-providers/{id}/deconfigure", fileScannerProviders.Deconfigure)
	mux.HandleFunc("POST /api/file-scanner-providers/{id}/reveal", fileScannerProviders.Reveal)
	mux.HandleFunc("POST /api/file-scanner-providers/{id}/validate", fileScannerProviders.Validate)

	// Bootstrap
	mux.HandleFunc("GET /api/bootstrap", services.Bootstrapper.IsEnabled)
	mux.HandleFunc("POST /api/bootstrap/login", services.Bootstrapper.Login)
	mux.HandleFunc("POST /api/bootstrap/logout", services.Bootstrapper.Logout)

	// Models
	mux.HandleFunc("POST /api/models", models.Create)
	mux.HandleFunc("GET /api/models", models.List)
	mux.HandleFunc("GET /api/models/{id}", models.ByID)
	mux.HandleFunc("DELETE /api/models/{id}", models.Delete)
	mux.HandleFunc("PUT /api/models/{id}", models.Update)

	// Available Models
	mux.HandleFunc("GET /api/available-models", availableModels.List)
	mux.HandleFunc("GET /api/available-models/{model_provider_id}", availableModels.ListForModelProvider)

	// Default Model Aliases
	mux.HandleFunc("POST /api/default-model-aliases", defaultModelAliases.Create)
	mux.HandleFunc("GET /api/default-model-aliases", defaultModelAliases.List)
	mux.HandleFunc("DELETE /api/default-model-aliases/{id}", defaultModelAliases.Delete)
	mux.HandleFunc("GET /api/default-model-aliases/{id}", defaultModelAliases.GetByID)
	mux.HandleFunc("PUT /api/default-model-aliases/{id}", defaultModelAliases.Update)

	// Workflows
	mux.HandleFunc("GET /api/workflows", workflows.List)
	mux.HandleFunc("GET /api/workflows/{id}", workflows.ByID)

	// We can remove these endpoints when we get rid of the legacy admin side of things
	mux.HandleFunc("PUT /api/workflows/{id}", workflows.Update)
	mux.HandleFunc("DELETE /api/workflows/{id}", workflows.Delete)
	//

	// Generated and uploaded images
	mux.HandleFunc("POST /api/image/generate", images.GenerateImage)
	mux.HandleFunc("POST /api/image/upload", images.UploadImage)
	mux.HandleFunc("GET /api/image/{id}", images.GetImage)

	// Prompt
	mux.HandleFunc("POST /api/prompt", prompt.Prompt)

	// Catch all 404 for API
	mux.HTTPHandle("/api/", http.NotFoundHandler())

	// Auth Provider tools
	mux.HandleFunc("/oauth2/", services.ProxyManager.HandlerFunc)

	// Well-known
	wellknown.SetupHandlers(services.ServerURL, services.OAuthServerConfig, mux)

	// Obot OAuth
	oauth.SetupHandlers(services.GatewayClient, oauthChecker, services.PersistentTokenServer, services.OAuthServerConfig, services.ServerURL, mux)

	// Gateway APIs
	services.GatewayServer.AddRoutes(services.APIServer)

	services.APIServer.HTTPHandle("/", ui.Handler(services.DevUIPort, services.UserUIPort))

	return services.APIServer, nil
}
