package router

import (
	"net/http"

	"github.com/otto8-ai/otto8/pkg/api/handlers"
	"github.com/otto8-ai/otto8/pkg/services"
	"github.com/otto8-ai/otto8/ui"
)

func Router(services *services.Services) (http.Handler, error) {
	mux := services.APIServer

	agents := handlers.NewAgentHandler(services.GPTClient, services.ServerURL)
	workflows := handlers.NewWorkflowHandler(services.GPTClient, services.ServerURL)
	invoker := handlers.NewInvokeHandler(services.Invoker)
	threads := handlers.NewThreadHandler(services.GPTClient, services.Events)
	runs := handlers.NewRunHandler(services.Events)
	toolRefs := handlers.NewToolReferenceHandler()
	webhooks := handlers.NewWebhookHandler()
	cronJobs := handlers.NewCronJobHandler()

	// Version
	mux.HandleFunc("GET /api/version", handlers.GetVersion)

	// Agents
	mux.HandleFunc("GET /api/agents", agents.List)
	mux.HandleFunc("GET /api/agents/{id}", agents.ByID)
	mux.HandleFunc("GET /api/agents/{id}/script", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/script.gpt", agents.Script)
	mux.HandleFunc("GET /api/agents/{id}/script/tool.gpt", agents.Script)
	mux.HandleFunc("POST /api/agents", agents.Create)
	mux.HandleFunc("PUT /api/agents/{id}", agents.Update)
	mux.HandleFunc("DELETE /api/agents/{id}", agents.Delete)
	mux.HandleFunc("POST /api/agents/{agent_id}/oauth-credentials/{ref}/login", agents.EnsureCredentialForKnowledgeSource)

	// Agent files
	mux.HandleFunc("GET /api/agents/{id}/files", agents.ListFiles)
	mux.HandleFunc("POST /api/agents/{id}/files/{file}", agents.UploadFile)
	mux.HandleFunc("DELETE /api/agents/{id}/files/{file}", agents.DeleteFile)

	// Agent knowledge files
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-files", agents.ListKnowledgeFiles)
	mux.HandleFunc("POST /api/agents/{id}/knowledge-files/{file...}", agents.UploadKnowledgeFile)
	mux.HandleFunc("DELETE /api/agents/{id}/knowledge-files/{file...}", agents.DeleteKnowledgeFile)

	// Agent approve file
	mux.HandleFunc("POST /api/agents/{id}/approve-file/{file_id}", agents.ApproveKnowledgeFile)

	// Remote Knowledge Sources
	mux.HandleFunc("POST /api/agents/{agent_id}/knowledge-sources", agents.CreateKnowledgeSource)
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-sources", agents.ListKnowledgeSources)
	mux.HandleFunc("POST /api/agents/{agent_id}/knowledge-sources/{id}/sync", agents.ReSyncKnowledgeSource)
	mux.HandleFunc("PUT /api/agents/{agent_id}/knowledge-sources/{id}", agents.UpdateKnowledgeSource)
	mux.HandleFunc("DELETE /api/agents/{agent_id}/knowledge-sources/{id}", agents.DeleteKnowledgeSource)
	mux.HandleFunc("GET /api/agents/{agent_id}/knowledge-sources/{knowledge_source_id}/knowledge-files", agents.ListKnowledgeFiles)
	mux.HandleFunc("POST /api/agents/{agent_id}/knowledge-sources/{knowledge_source_id}/knowledge-files/{id}/ingest", agents.ReIngestKnowledgeFile)

	// Workflows
	mux.HandleFunc("GET /api/workflows", workflows.List)
	mux.HandleFunc("GET /api/workflows/{id}", workflows.ByID)
	mux.HandleFunc("GET /api/workflows/{id}/script", workflows.Script)
	mux.HandleFunc("GET /api/workflows/{id}/script.gpt", workflows.Script)
	mux.HandleFunc("GET /api/workflows/{id}/script/tool.gpt", workflows.Script)
	mux.HandleFunc("POST /api/workflows", workflows.Create)
	mux.HandleFunc("PUT /api/workflows/{id}", workflows.Update)
	mux.HandleFunc("DELETE /api/workflows/{id}", workflows.Delete)

	// Workflow files
	mux.HandleFunc("GET /api/workflows/{id}/files", workflows.Files)
	mux.HandleFunc("POST /api/workflows/{id}/files/{file}", workflows.UploadFile)
	mux.HandleFunc("DELETE /api/workflows/{id}/files/{file}", workflows.DeleteFile)

	// Invoker
	mux.HandleFunc("POST /api/invoke/{id}", invoker.Invoke)
	mux.HandleFunc("POST /api/invoke/{id}/threads/{thread}", invoker.Invoke)

	// Threads
	mux.HandleFunc("GET /api/threads", threads.List)
	mux.HandleFunc("GET /api/threads/{id}", threads.ByID)
	mux.HandleFunc("GET /api/threads/{id}/events", threads.Events)
	mux.HandleFunc("DELETE /api/threads/{id}", threads.Delete)
	mux.HandleFunc("PUT /api/threads/{id}", threads.Update)
	mux.HandleFunc("GET /api/agents/{agent}/threads", threads.List)

	// Thread files
	mux.HandleFunc("GET /api/threads/{id}/files", threads.Files)
	mux.HandleFunc("GET /api/threads/{id}/file/{file...}", threads.GetFile)
	mux.HandleFunc("POST /api/threads/{id}/files/{file...}", threads.UploadFile)
	mux.HandleFunc("DELETE /api/threads/{id}/files/{file...}", threads.DeleteFile)

	// Thread knowledge files
	mux.HandleFunc("GET /api/threads/{id}/knowledge", threads.Knowledge)
	mux.HandleFunc("POST /api/threads/{id}/knowledge/{file}", threads.UploadKnowledge)
	mux.HandleFunc("DELETE /api/threads/{id}/knowledge/{file...}", threads.DeleteKnowledge)

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

	// Webhooks
	mux.HandleFunc("POST /api/webhooks", webhooks.Create)
	mux.HandleFunc("GET /api/webhooks", webhooks.List)
	mux.HandleFunc("GET /api/webhooks/{id}", webhooks.ByID)
	mux.HandleFunc("DELETE /api/webhooks/{id}", webhooks.Delete)
	mux.HandleFunc("PUT /api/webhooks/{id}", webhooks.Update)
	mux.HandleFunc("POST /api/webhooks/{id}", webhooks.Execute)

	// CronJobs
	mux.HandleFunc("POST /api/cronjobs", cronJobs.Create)
	mux.HandleFunc("GET /api/cronjobs", cronJobs.List)
	mux.HandleFunc("GET /api/cronjobs/{id}", cronJobs.ByID)
	mux.HandleFunc("DELETE /api/cronjobs/{id}", cronJobs.Delete)
	mux.HandleFunc("PUT /api/cronjobs/{id}", cronJobs.Update)
	mux.HandleFunc("POST /api/cronjobs/{id}", cronJobs.Execute)

	// debug
	mux.HTTPHandle("GET /debug/pprof/", http.DefaultServeMux)

	// Gateway APIs
	services.GatewayServer.AddRoutes(services.APIServer)

	// UI
	services.APIServer.HTTPHandle("/", services.ProxyServer.Wrap(ui.Handler(services.DevUIPort)))

	return services.APIServer, nil
}
