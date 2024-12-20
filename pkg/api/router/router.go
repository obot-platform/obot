package router

import (
	"net/http"

	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/obot-platform/obot/ui"
)

func Router(services *services.Services) (http.Handler, error) {
	mux := services.APIServer

	agents := handlers.NewAgentHandler(services.GPTClient, services.ServerURL)
	assistants := handlers.NewAssistantHandler(services.Invoker, services.Events, services.GPTClient)
	tasks := handlers.NewTaskHandler(services.Invoker, services.Events)
	workflows := handlers.NewWorkflowHandler(services.GPTClient, services.ServerURL, services.Invoker)
	invoker := handlers.NewInvokeHandler(services.Invoker)
	threads := handlers.NewThreadHandler(services.GPTClient, services.Events)
	runs := handlers.NewRunHandler(services.Events)
	toolRefs := handlers.NewToolReferenceHandler(services.GPTClient)
	webhooks := handlers.NewWebhookHandler()
	cronJobs := handlers.NewCronJobHandler()
	models := handlers.NewModelHandler()
	availableModels := handlers.NewAvailableModelsHandler(services.GPTClient, services.ModelProviderDispatcher)
	modelProviders := handlers.NewModelProviderHandler(services.GPTClient, services.ModelProviderDispatcher)
	prompt := handlers.NewPromptHandler(services.GPTClient)
	emailreceiver := handlers.NewEmailReceiverHandler(services.EmailServerName)
	defaultModelAliases := handlers.NewDefaultModelAliasHandler()
	version := handlers.NewVersionHandler(services.EmailServerName)
	tables := handlers.NewTableHandler(services.GPTClient)

	// Version
	mux.HandleFunc("GET /api/version", version.GetVersion)

	// Agents
	mux.HandleFunc("GET /api/agents", agents.List)
	mux.HandleFunc("GET /api/agents/{id}", agents.ByID)
	mux.HandleFunc("PUT /api/agents/{id}/setdefault", agents.SetDefault)
	mux.HandleFunc("GET /api/agents/{id}/script", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/script.gpt", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/script/tool.gpt", agents.Script)
	mux.HandleFunc("POST /api/agents", agents.Create)
	mux.HandleFunc("PUT /api/agents/{id}", agents.Update)
	mux.HandleFunc("DELETE /api/agents/{id}", agents.Delete)
	mux.HandleFunc("POST /api/agents/{id}/oauth-credentials/{ref}/login", agents.EnsureCredentialForKnowledgeSource)

	// Assistants
	mux.HandleFunc("GET /api/assistants", assistants.List)
	mux.HandleFunc("GET /api/assistants/{id}/credentials", assistants.ListCredentials)
	mux.HandleFunc("DELETE /api/assistants/{id}/credentials/{cred_id}", assistants.DeleteCredential)
	mux.HandleFunc("GET /api/assistants/{id}/events", assistants.Events)
	mux.HandleFunc("POST /api/assistants/{id}/abort", assistants.Abort)
	mux.HandleFunc("POST /api/assistants/{id}/invoke", assistants.Invoke)
	// Assistant tools
	mux.HandleFunc("GET /api/assistants/{id}/tools", assistants.Tools)
	mux.HandleFunc("DELETE /api/assistants/{id}/tools/{tool}", assistants.RemoveTool)
	mux.HandleFunc("PUT /api/assistants/{id}/tools/{tool}", assistants.AddTool)
	// Assistant files
	mux.HandleFunc("GET /api/assistants/{assistant_id}/files", assistants.Files)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/file/{file...}", assistants.GetFile)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/file/{file...}", assistants.UploadFile)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/files/{file...}", assistants.DeleteFile)
	// Assistant knowledge files
	mux.HandleFunc("GET /api/assistants/{id}/knowledge", assistants.Knowledge)
	mux.HandleFunc("POST /api/assistants/{id}/knowledge/{file}", assistants.UploadKnowledge)
	mux.HandleFunc("DELETE /api/assistants/{id}/knowledge/{file...}", assistants.DeleteKnowledge)

	// Tasks
	mux.HandleFunc("GET /api/assistants/{assistant_id}/tasks", tasks.List)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks", tasks.List)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/tasks/{id}", tasks.Get)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks/{id}", tasks.Get)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/tasks", tasks.Create)
	mux.HandleFunc("POST /api/threads/{thread_id}/tasks", tasks.Create)
	mux.HandleFunc("PUT /api/assistants/{assistant_id}/tasks/{id}", tasks.Update)
	mux.HandleFunc("PUT /api/threads/{thread_id}/tasks/{id}", tasks.Update)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/tasks/{id}", tasks.Delete)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/tasks/{id}/run", tasks.Run)
	mux.HandleFunc("POST /api/threads/{thread_id}/tasks/{id}/run", tasks.Run)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/tasks/{id}/runs", tasks.ListRuns)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks/{id}/runs", tasks.ListRuns)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/tasks/{id}/runs/{run_id}", tasks.GetRun)
	mux.HandleFunc("GET /api/threads/{thread_id}/tasks/{id}/runs/{run_id}", tasks.GetRun)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/tasks/{id}/runs/{run_id}/abort", tasks.AbortRun)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/tasks/{id}/runs/{run_id}", tasks.DeleteRun)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/tasks/{task_id}/runs/{run_id}/files", assistants.Files)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/tasks/{task_id}/runs/{run_id}/file/{file...}", assistants.GetFile)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/tasks/{task_id}/runs/{run_id}/file/{file...}", assistants.UploadFile)
	mux.HandleFunc("DELETE /api/assistants/{assistant_id}/tasks/{task_id}/runs/{run_id}/files/{file...}", assistants.DeleteFile)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/tasks/{id}/events", tasks.Events)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/tasks/{id}/events", tasks.Abort)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/tasks/{id}/runs/{run_id}/events", tasks.Events)
	mux.HandleFunc("POST /api/assistants/{assistant_id}/tasks/{id}/runs/{run_id}/events", tasks.Abort)

	// Tables
	mux.HandleFunc("GET /api/assistants/{assistant_id}/tables", tables.ListTables)
	mux.HandleFunc("GET /api/assistants/{assistant_id}/tables/{table_name}/rows", tables.GetRows)

	// Agent files
	mux.HandleFunc("GET /api/agents/{id}/files", agents.ListFiles)
	mux.HandleFunc("GET /api/agents/{id}/file/{file...}", agents.GetFile)
	mux.HandleFunc("POST /api/agents/{id}/files/{file}", agents.UploadFile)
	mux.HandleFunc("DELETE /api/agents/{id}/files/{file}", agents.DeleteFile)

	// Agent knowledge files
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-files", agents.ListKnowledgeFiles)
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

	// Workflows
	mux.HandleFunc("GET /api/workflows", workflows.List)
	mux.HandleFunc("GET /api/workflows/{id}", workflows.ByID)
	mux.HandleFunc("GET /api/workflows/{id}/executions", workflows.WorkflowExecutions)
	mux.HandleFunc("GET /api/workflows/{id}/script", workflows.Script)
	mux.HandleFunc("GET /api/workflows/{id}/script.gpt", workflows.Script)
	mux.HandleFunc("GET /api/workflows/{id}/script/tool.gpt", workflows.Script)
	mux.HandleFunc("POST /api/workflows", workflows.Create)
	mux.HandleFunc("POST /api/workflows/{id}/authenticate", workflows.Authenticate)
	mux.HandleFunc("PUT /api/workflows/{id}", workflows.Update)
	mux.HandleFunc("DELETE /api/workflows/{id}", workflows.Delete)
	mux.HandleFunc("POST /api/workflows/{id}/oauth-credentials/{ref}/login", workflows.EnsureCredentialForKnowledgeSource)

	// Workflow knowledge files
	mux.HandleFunc("GET /api/workflows/{agent_id}/knowledge-files", agents.ListKnowledgeFiles)
	mux.HandleFunc("POST /api/workflows/{id}/knowledge-files/{file...}", agents.UploadKnowledgeFile)
	mux.HandleFunc("DELETE /api/workflows/{id}/knowledge-files/{file...}", agents.DeleteKnowledgeFile)
	mux.HandleFunc("POST /api/workflows/{agent_id}/knowledge-files/{file_id}/ingest", agents.ReIngestKnowledgeFile)

	// Workflow approve file
	mux.HandleFunc("POST /api/workflows/{agent_id}/approve-file/{file_id}", agents.ApproveKnowledgeFile)

	// Workspace Remote Knowledge Sources
	mux.HandleFunc("POST /api/workflows/{agent_id}/knowledge-sources", agents.CreateKnowledgeSource)
	mux.HandleFunc("GET /api/workflows/{agent_id}/knowledge-sources", agents.ListKnowledgeSources)
	mux.HandleFunc("POST /api/workflows/{agent_id}/knowledge-sources/{id}/sync", agents.ReSyncKnowledgeSource)
	mux.HandleFunc("PUT /api/workflows/{agent_id}/knowledge-sources/{id}", agents.UpdateKnowledgeSource)
	mux.HandleFunc("DELETE /api/workflows/{agent_id}/knowledge-sources/{id}", agents.DeleteKnowledgeSource)
	mux.HandleFunc("GET /api/workflows/{agent_id}/knowledge-sources/{knowledge_source_id}/knowledge-files", agents.ListKnowledgeFiles)
	mux.HandleFunc("POST /api/workflows/{agent_id}/knowledge-sources/{knowledge_source_id}/knowledge-files/{file_id}/ingest", agents.ReIngestKnowledgeFile)

	// Workflow files
	mux.HandleFunc("GET /api/workflows/{id}/files", agents.ListFiles)
	mux.HandleFunc("GET /api/workflows/{id}/file/{file...}", agents.GetFile)
	mux.HandleFunc("POST /api/workflows/{id}/files/{file}", agents.UploadFile)
	mux.HandleFunc("DELETE /api/workflows/{id}/files/{file}", agents.DeleteFile)

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
	mux.HandleFunc("GET /api/threads/{id}/file/{file...}", threads.GetFile)
	mux.HandleFunc("POST /api/threads/{id}/files/{file...}", threads.UploadFile)
	mux.HandleFunc("DELETE /api/threads/{id}/files/{file...}", threads.DeleteFile)

	// Thread knowledge files
	mux.HandleFunc("GET /api/threads/{id}/knowledge-files", threads.Knowledge)
	mux.HandleFunc("POST /api/threads/{id}/knowledge-files/{file}", threads.UploadKnowledge)
	mux.HandleFunc("DELETE /api/threads/{id}/knowledge-files/{file...}", threads.DeleteKnowledge)

	// ToolRefs
	mux.HandleFunc("GET /api/tool-references", toolRefs.List)
	mux.HandleFunc("GET /api/tool-references/{id}", toolRefs.ByID)
	mux.HandleFunc("POST /api/tool-references", toolRefs.Create)
	mux.HandleFunc("DELETE /api/tool-references/{id}", toolRefs.Delete)
	mux.HandleFunc("PUT /api/tool-references/{id}", toolRefs.Update)

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

	// Email Receivers
	mux.HandleFunc("POST /api/email-receivers", emailreceiver.Create)
	mux.HandleFunc("GET /api/email-receivers", emailreceiver.List)
	mux.HandleFunc("GET /api/email-receivers/{id}", emailreceiver.ByID)
	mux.HandleFunc("DELETE /api/email-receivers/{id}", emailreceiver.Delete)
	mux.HandleFunc("PUT /api/email-receivers/{id}", emailreceiver.Update)

	// Email Receivers for generic create
	mux.HandleFunc("POST /api/emailreceivers", emailreceiver.Create)
	mux.HandleFunc("GET /api/emailreceivers/{id}", emailreceiver.ByID)

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
	mux.HandleFunc("POST /api/model-providers/{id}/configure", modelProviders.Configure)
	mux.HandleFunc("POST /api/model-providers/{id}/reveal", modelProviders.Reveal)
	mux.HandleFunc("POST /api/model-providers/{id}/refresh-models", modelProviders.RefreshModels)

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

	// Gateway APIs
	services.GatewayServer.AddRoutes(services.APIServer)

	services.APIServer.HTTPHandle("/", ui.Handler(services.DevUIPort, services.StorageClient))

	return services.APIServer, nil
}
