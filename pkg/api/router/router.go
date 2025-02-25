package router

import (
	"net/http"

	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/api/handlers/sendgrid"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/obot-platform/obot/ui"
)

func Router(services *services.Services) (http.Handler, error) {
	mux := services.APIServer

	agents := handlers.NewAgentHandler(services.GPTClient, services.Invoker, services.ServerURL)
	auths := handlers.NewAuthorizationHandler(services.GatewayClient)
	assistants := handlers.NewAssistantHandler(services.Invoker, services.Events, services.GPTClient, services.Router.Backend())
	tools := handlers.NewToolHandler(services.GPTClient, services.Invoker)
	tasks := handlers.NewTaskHandler(services.Invoker, services.Events, services.GPTClient, services.ServerURL)
	invoker := handlers.NewInvokeHandler(services.Invoker)
	threads := handlers.NewThreadHandler(services.GPTClient, services.Events)
	runs := handlers.NewRunHandler(services.Events)
	toolRefs := handlers.NewToolReferenceHandler(services.GPTClient)
	webhooks := handlers.NewWebhookHandler()
	cronJobs := handlers.NewCronJobHandler()
	models := handlers.NewModelHandler()
	availableModels := handlers.NewAvailableModelsHandler(services.GPTClient, services.ProviderDispatcher)
	modelProviders := handlers.NewModelProviderHandler(services.GPTClient, services.ProviderDispatcher, services.Invoker)
	authProviders := handlers.NewAuthProviderHandler(services.GPTClient, services.ProviderDispatcher)
	prompt := handlers.NewPromptHandler(services.GPTClient)
	emailReceiver := handlers.NewEmailReceiverHandler(services.EmailServerName)
	defaultModelAliases := handlers.NewDefaultModelAliasHandler()
	version := handlers.NewVersionHandler(services.EmailServerName, services.SupportDocker, services.AuthEnabled)
	tables := handlers.NewTableHandler(services.GPTClient)
	projects := handlers.NewProjectsHandler(services.Router.Backend(), services.Invoker, services.GPTClient)
	templates := handlers.NewTemplateHandler(services.Router.Backend())

	sendgridWebhookHandler := sendgrid.NewInboundWebhookHandler(services.StorageClient, services.EmailServerName, services.SendgridWebhookUsername, services.SendgridWebhookPassword)

	// Version
	mux.HandleFunc("GET /api/version", version.GetVersion)

	// Agents
	mux.HandleFunc("GET /api/agents", agents.List)
	mux.HandleFunc("GET /api/agents/{id}", agents.ByID)
	mux.HandleFunc("PUT /api/agents/{id}/setdefault", agents.SetDefault)
	mux.HandleFunc("GET /api/agents/{id}/script", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/script.gpt", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/script/tool.gpt", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/threads/{thread_id}/script", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/threads/{thread_id}/script.gpt", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/threads/{thread_id}/script/tool.gpt", agents.Script)
	mux.HandleFunc("POST /api/agents", agents.Create)
	mux.HandleFunc("POST /api/agents/{id}/authenticate", agents.Authenticate)
	mux.HandleFunc("POST /api/agents/{id}/deauthenticate", agents.DeAuthenticate)
	mux.HandleFunc("PUT /api/agents/{id}", agents.Update)
	mux.HandleFunc("DELETE /api/agents/{id}", agents.Delete)
	mux.HandleFunc("POST /api/agents/{id}/oauth-credentials/{ref}/login", agents.EnsureCredentialForKnowledgeSource)

	// Agent Authorizations
	mux.HandleFunc("GET /api/agents/{id}/authorizations", auths.ListAgentAuthorizations)
	mux.HandleFunc("POST /api/agents/{id}/authorizations/add", auths.AddAgentAuthorization)
	mux.HandleFunc("POST /api/agents/{id}/authorizations/remove", auths.RemoveAgentAuthorization)

	// Templates
	mux.HandleFunc("GET /api/templates", templates.ListTemplates)
	mux.HandleFunc("GET /api/templates/{id}", templates.GetTemplate)

	// Projects
	mux.HandleFunc("GET /api/projects", projects.ListProjects)

	// Assistants
	mux.HandleFunc("GET /api/assistants", assistants.List)
	mux.HandleFunc("GET /api/assistants/{id}", assistants.Get)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/credentials", projects.ListCredentials)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/credentials/{cred_id}", assistants.DeleteCredential)
	mux.HandleFunc("GET /api/assistants/{id}/projects/{project_id}/threads/{thread_id}/events", assistants.Events)
	mux.HandleFunc("POST /api/assistants/{id}/projects/{project_id}/threads/{thread_id}/abort", assistants.Abort)
	mux.HandleFunc("POST /api/assistants/{id}/projects/{project_id}/threads/{thread_id}/invoke", assistants.Invoke)
	// Assistant tools
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tools", assistants.Tools)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool}", assistants.RemoveTool)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool}/custom", assistants.DeleteTool)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tools/{tools}/authenticate", projects.Authenticate)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tools}/deauthenticate", projects.DeAuthenticate)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool}", assistants.AddTool)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/tools", assistants.SetTools)
	// Assistant files
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/files", assistants.Files)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}", assistants.GetFile)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}", assistants.UploadFile)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/files/{file...}", assistants.DeleteFile)
	// Assistant knowledge files
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/knowledge", assistants.Knowledge)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file}", assistants.GetKnowledgeFile)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file}", assistants.UploadKnowledge)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file...}", assistants.DeleteKnowledge)
	// Env
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/env", assistants.GetEnv)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/env", assistants.SetEnv)

	if services.SupportDocker {
		shell, err := handlers.NewShellHandler(services.Invoker)
		if err != nil {
			return nil, err
		}
		mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/shell", shell.Shell)

		// Tools
		mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tools", tools.Create)
		mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/env", tools.SetEnv)
		mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/test", tools.Test)
	}
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}", tools.Get)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/env", tools.GetEnv)

	// Just Tasks
	mux.HandleFunc("DELETE /api/tasks/{id}/authenticate", tasks.Authenticate)
	mux.HandleFunc("DELETE /api/tasks/{id}/deauthenticate", tasks.DeAuthenticate)
	mux.HandleFunc("GET /api/tasks/{id}/script", tasks.Script)
	mux.HandleFunc("GET /api/tasks/{id}/script.gpt", tasks.Script)
	mux.HandleFunc("GET /api/tasks/{id}/script/tool.gpt", tasks.Script)
	mux.HandleFunc("POST /api/tasks/{id}/oauth-credentials/{ref}/login", tasks.EnsureCredentialForKnowledgeSource)
	mux.HandleFunc("GET /api/tasks", tasks.List)
	mux.HandleFunc("GET /api/tasks/{id}", tasks.Get)
	mux.HandleFunc("DELETE /api/tasks/{id}", tasks.Delete)
	mux.HandleFunc("PUT /api/tasks/{id}", tasks.Update)
	mux.HandleFunc("POST /api/tasks/{id}/run", tasks.Run)
	mux.HandleFunc("POST /api/tasks/{id}/runs/{run_id}/abort", tasks.AbortRun)
	mux.HandleFunc("DELETE /api/tasks/{id}/runs/{run_id}", tasks.DeleteRun)
	mux.HandleFunc("GET /api/tasks/{id}/runs/{run_id}/events", tasks.Events)
	mux.HandleFunc("POST /api/tasks/{id}/runs/{run_id}/events", tasks.Abort)
	mux.HandleFunc("GET /api/tasks/{id}/files", agents.ListFiles)
	mux.HandleFunc("GET /api/tasks/{id}/file/{file...}", agents.GetFile)
	mux.HandleFunc("POST /api/tasks/{id}/files/{file}", agents.UploadFile)
	mux.HandleFunc("DELETE /api/tasks/{id}/files/{file}", agents.DeleteFile)

	// User Tasks
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks", tasks.ListFromScope)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks", tasks.ListFromScope)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}", tasks.GetFromScope)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks/{id}", tasks.GetFromScope)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks", tasks.CreateFromScope)
	mux.HandleFunc("POST /api/threads/{thread_id}/tasks", tasks.CreateFromScope)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}", tasks.UpdateFromScope)
	mux.HandleFunc("PUT /api/threads/{thread_id}/tasks/{id}", tasks.UpdateFromScope)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}", tasks.DeleteFromScope)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/run", tasks.RunFromScope)
	mux.HandleFunc("POST /api/threads/{thread_id}/tasks/{id}/run", tasks.RunFromScope)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs", tasks.ListRunsFromScope)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks/{id}/runs", tasks.ListRunsFromScope)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}", tasks.GetRunFromScope)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks/{id}/runs/{run_id}", tasks.GetRunFromScope)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}/abort", tasks.AbortRunFromScope)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}", tasks.DeleteRunFromScope)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files", assistants.Files)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/file/{file...}", assistants.GetFile)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/file/{file...}", assistants.UploadFile)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files/{file...}", assistants.DeleteFile)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/events", tasks.EventsFromScope)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/events", tasks.AbortFromScope)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}/events", tasks.EventsFromScope)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}/events", tasks.AbortFromScope)

	// Projects
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects", projects.ListProjects)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects", projects.CreateProject)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}", projects.GetProject)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/authorizations", projects.ListAuthorizations)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/authorizations", projects.UpdateAuthorizations)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/pending-authorizations", projects.ListPendingAuthorizations)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/pending-authorizations/{project_id}", projects.AcceptPendingAuthorization)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/pending-authorizations/{project_id}", projects.RejectPendingAuthorization)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}", projects.UpdateProject)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}", projects.DeleteProject)

	// Project Threads
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/threads", projects.ListProjectThreads)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/threads", projects.CreateProjectThread)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}", projects.DeleteProjectThread)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/projects/{project_id}/threads/{id}", threads.Update)

	// Project Templates
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/templates", projects.ListTemplates)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/projects/{project_id}/templates", projects.CreateTemplate)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/templates/{id}", projects.GetTemplate)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/projects/{project_id}/templates/{id}", projects.DeleteTemplate)
	mux.HandleFunc("POST /api/templates/{template_id}/projects", projects.CreateProject)

	// Tables
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tables", tables.ListTables)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/projects/{project_id}/tables/{table_name}/rows", tables.GetRows)

	// Agent files
	mux.HandleFunc("GET /api/agents/{id}/files", agents.ListFiles)
	mux.HandleFunc("GET /api/agents/{id}/file/{file...}", agents.GetFile)
	mux.HandleFunc("POST /api/agents/{id}/files/{file}", agents.UploadFile)
	mux.HandleFunc("DELETE /api/agents/{id}/files/{file}", agents.DeleteFile)

	// Agent knowledge files
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-files", agents.ListKnowledgeFiles)
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-files/{file}", agents.GetKnowledgeFile)
	mux.HandleFunc("POST /api/agents/{id}/knowledge-files/{file...}", agents.UploadKnowledgeFile)
	mux.HandleFunc("DELETE /api/agents/{id}/knowledge-files/{file...}", agents.DeleteKnowledgeFile)
	mux.HandleFunc("POST /api/agents/{agent_id}/knowledge-files/{file_id}/ingest", agents.ReIngestKnowledgeFile)

	// Agent approve file
	mux.HandleFunc("POST /api/agents/{agent_id}/approve-file/{file_id}", agents.ApproveKnowledgeFile)

	// Remote Knowledge Sources
	mux.HandleFunc("POST /api/agents/{agent_id}/knowledge-sources", agents.CreateKnowledgeSource)
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-sources", agents.ListKnowledgeSources)
	mux.HandleFunc("POST /api/agents/{agent_id}/knowledge-sources/{id}/sync", agents.ReSyncKnowledgeSource)
	mux.HandleFunc("PUT /api/agents/{agent_id}/knowledge-sources/{id}", agents.UpdateKnowledgeSource)
	mux.HandleFunc("DELETE /api/agents/{agent_id}/knowledge-sources/{id}", agents.DeleteKnowledgeSource)
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-sources/{knowledge_source_id}/knowledge-files", agents.ListKnowledgeFiles)
	mux.HandleFunc("POST /api/agents/{agent_id}/knowledge-sources/{knowledge_source_id}/knowledge-files/{file_id}/ingest", agents.ReIngestKnowledgeFile)
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-sources/{knowledge_source_id}/knowledge-files/watch", agents.WatchKnowledgeFile)

	// Invoker
	mux.HandleFunc("POST /api/invoke/{id}", invoker.Invoke)
	mux.HandleFunc("POST /api/invoke/{id}/thread/{thread}", invoker.Invoke)
	mux.HandleFunc("POST /api/invoke/{id}/threads/{thread}", invoker.Invoke)

	// Threads
	mux.HandleFunc("GET /api/threads", threads.List)
	mux.HandleFunc("GET /api/threads/{id}", threads.ByID)
	mux.HandleFunc("POST /api/threads/{id}/abort", threads.Abort)
	mux.HandleFunc("GET /api/threads/{id}/events", threads.Events)
	mux.HandleFunc("GET /api/threads/{id}/workflows", threads.Workflows)
	mux.HandleFunc("GET /api/threads/{id}/workflows/{workflow_id}/executions", threads.WorkflowExecutions)
	mux.HandleFunc("DELETE /api/threads/{id}", threads.Delete)
	mux.HandleFunc("PUT /api/threads/{id}", threads.Update)
	mux.HandleFunc("GET /api/agents/{agent}/threads", threads.List)

	// Thread files
	mux.HandleFunc("GET /api/threads/{id}/files", threads.Files)
	mux.HandleFunc("GET /api/threads/{id}/files/{file...}", threads.GetFile)
	mux.HandleFunc("POST /api/threads/{id}/files/{file...}", threads.UploadFile)
	mux.HandleFunc("DELETE /api/threads/{id}/files/{file...}", threads.DeleteFile)

	// Thread knowledge files
	mux.HandleFunc("GET /api/threads/{id}/knowledge-files", threads.Knowledge)
	mux.HandleFunc("GET /api/threads/{id}/knowledge-files/{file}", threads.GetKnowledgeFile)
	mux.HandleFunc("POST /api/threads/{id}/knowledge-files/{file}", threads.UploadKnowledge)
	mux.HandleFunc("DELETE /api/threads/{id}/knowledge-files/{file...}", threads.DeleteKnowledge)

	// Thread tables
	mux.HandleFunc("GET /api/threads/{id}/tables", threads.Tables)
	mux.HandleFunc("GET /api/threads/{id}/tables/{table}/rows", threads.TableRows)

	// ToolRefs
	mux.HandleFunc("GET /api/tool-references", toolRefs.List)
	mux.HandleFunc("GET /api/tool-references/{id}", toolRefs.ByID)
	mux.HandleFunc("POST /api/tool-references", toolRefs.Create)
	mux.HandleFunc("DELETE /api/tool-references/{id}", toolRefs.Delete)
	mux.HandleFunc("PUT /api/tool-references/{id}", toolRefs.Update)
	mux.HandleFunc("POST /api/tool-references/{id}/force-refresh", toolRefs.ForceRefresh)

	// Runs
	mux.HandleFunc("GET /api/runs", runs.List)
	mux.HandleFunc("GET /api/runs/{id}", runs.ByID)
	mux.HandleFunc("DELETE /api/runs/{id}", runs.Delete)
	mux.HandleFunc("GET /api/runs/{id}/debug", runs.Debug)
	mux.HandleFunc("GET /api/runs/{id}/events", runs.Events)
	mux.HandleFunc("GET /api/threads/{thread}/runs", runs.List)
	mux.HandleFunc("GET /api/agents/{agent}/runs", runs.List)
	mux.HandleFunc("GET /api/agents/{agent}/threads/{thread}/runs", runs.List)
	mux.HandleFunc("GET /api/workflows/{workflow}/runs", runs.List)
	mux.HandleFunc("GET /api/workflows/{workflow}/threads/{thread}/runs", runs.List)

	// Credentials
	mux.HandleFunc("GET /api/threads/{context}/credentials", handlers.ListCredentials)
	mux.HandleFunc("GET /api/agents/{context}/credentials", handlers.ListCredentials)
	mux.HandleFunc("GET /api/workflows/{context}/credentials", handlers.ListCredentials)
	mux.HandleFunc("GET /api/credentials", handlers.ListCredentials)
	mux.HandleFunc("DELETE /api/threads/{context}/credentials/{id}", handlers.DeleteCredential)
	mux.HandleFunc("DELETE /api/agents/{context}/credentials/{id}", handlers.DeleteCredential)
	mux.HandleFunc("DELETE /api/workflows/{context}/credentials/{id}", handlers.DeleteCredential)
	mux.HandleFunc("DELETE /api/credentials/{id}", handlers.DeleteCredential)

	// Environment variable credentials
	mux.HandleFunc("POST /api/workflows/{id}/env", handlers.SetEnv)
	mux.HandleFunc("GET /api/workflows/{id}/env", handlers.RevealEnv)
	mux.HandleFunc("POST /api/agents/{id}/env", handlers.SetEnv)
	mux.HandleFunc("GET /api/agents/{id}/env", handlers.RevealEnv)

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

	// Email Receivers for generic create
	mux.HandleFunc("POST /api/emailreceivers", emailReceiver.Create)
	mux.HandleFunc("GET /api/emailreceivers/{id}", emailReceiver.ByID)

	// CronJobs
	mux.HandleFunc("POST /api/cronjobs", cronJobs.Create)
	mux.HandleFunc("GET /api/cronjobs", cronJobs.List)
	mux.HandleFunc("GET /api/cronjobs/{id}", cronJobs.ByID)
	mux.HandleFunc("DELETE /api/cronjobs/{id}", cronJobs.Delete)
	mux.HandleFunc("PUT /api/cronjobs/{id}", cronJobs.Update)
	mux.HandleFunc("POST /api/cronjobs/{id}", cronJobs.Execute)

	// debug
	mux.HTTPHandle("GET /debug/pprof/", http.DefaultServeMux)

	// Model providers
	mux.HandleFunc("GET /api/model-providers", modelProviders.List)
	mux.HandleFunc("GET /api/model-providers/{id}", modelProviders.ByID)
	mux.HandleFunc("POST /api/model-providers/{id}/validate", modelProviders.Validate)
	mux.HandleFunc("POST /api/model-providers/{id}/configure", modelProviders.Configure)
	mux.HandleFunc("POST /api/model-providers/{id}/deconfigure", modelProviders.Deconfigure)
	mux.HandleFunc("POST /api/model-providers/{id}/reveal", modelProviders.Reveal)
	mux.HandleFunc("POST /api/model-providers/{id}/refresh-models", modelProviders.RefreshModels)

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

	// Models
	mux.HandleFunc("POST /api/models", models.Create)
	mux.HandleFunc("PUT /api/models/{id}", models.Update)
	mux.HandleFunc("DELETE /api/models/{id}", models.Delete)
	mux.HandleFunc("GET /api/models", models.List)
	mux.HandleFunc("GET /api/models/{id}", models.ByID)

	// Available Models
	mux.HandleFunc("GET /api/available-models", availableModels.List)
	mux.HandleFunc("GET /api/available-models/{model_provider}", availableModels.ListForModelProvider)

	// Default Model Aliases
	mux.HandleFunc("GET /api/default-model-aliases", defaultModelAliases.List)
	mux.HandleFunc("GET /api/default-model-aliases/{id}", defaultModelAliases.GetByID)
	mux.HandleFunc("POST /api/default-model-aliases", defaultModelAliases.Create)
	mux.HandleFunc("PUT /api/default-model-aliases/{id}", defaultModelAliases.Update)
	mux.HandleFunc("DELETE /api/default-model-aliases/{id}", defaultModelAliases.Delete)

	// Prompt
	mux.HandleFunc("POST /api/prompt", prompt.Prompt)

	// Catch all 404 for API
	mux.HTTPHandle("/api/", http.NotFoundHandler())

	// Auth Provider tools
	mux.HandleFunc("/oauth2/", services.ProxyManager.HandlerFunc)

	// Gateway APIs
	services.GatewayServer.AddRoutes(services.APIServer)

	services.APIServer.HTTPHandle("/", ui.Handler(services.DevUIPort, services.StorageClient))

	return services.APIServer, nil
}
