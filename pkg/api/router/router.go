package router

import (
	"net/http"

	"github.com/otto8-ai/otto8/pkg/api/handlers"
	"github.com/otto8-ai/otto8/pkg/services"
	"github.com/otto8-ai/otto8/ui"
)

func Router(services *services.Services) (http.Handler, error) {
	w := services.APIServer.Wrap
	mux := http.NewServeMux()

	agents := handlers.NewAgentHandler(services.WorkspaceClient, "directory")
	workflows := handlers.NewWorkflowHandler(services.WorkspaceClient, "directory")
	invoker := handlers.NewInvokeHandler(services.Invoker)
	threads := handlers.NewThreadHandler(services.WorkspaceClient, services.Events)
	runs := handlers.NewRunHandler(services.Events)
	toolRefs := handlers.NewToolReferenceHandler(services.WorkspaceClient)
	webhooks := handlers.NewWebhookHandler()
	cronJobs := handlers.NewCronJobHandler()

	// Agents
	mux.Handle("GET /api/agents", w(agents.List))
	mux.Handle("GET /api/agents/{id}", w(agents.ByID))
	mux.Handle("GET /api/agents/{id}/script", w(agents.Script))
	mux.Handle("GET /api/agents/{id}/script.gpt", w(agents.Script))
	mux.Handle("GET /api/agents/{id}/script/tool.gpt", w(agents.Script))
	mux.Handle("POST /api/agents", w(agents.Create))
	mux.Handle("PUT /api/agents/{id}", w(agents.Update))
	mux.Handle("DELETE /api/agents/{id}", w(agents.Delete))

	// Agent files
	mux.Handle("GET /api/agents/{id}/files", w(agents.Files))
	mux.Handle("POST /api/agents/{id}/files/{file}", w(agents.UploadFile))
	mux.Handle("DELETE /api/agents/{id}/files/{file}", w(agents.DeleteFile))

	// Agent knowledge files
	mux.Handle("GET /api/agents/{id}/knowledge", w(agents.Knowledge))
	mux.Handle("POST /api/agents/{id}/knowledge/{file}", w(agents.UploadKnowledge))
	mux.Handle("DELETE /api/agents/{id}/knowledge/{file...}", w(agents.DeleteKnowledge))

	mux.Handle("POST /api/agents/{agent_id}/remote-knowledge-sources", w(agents.CreateRemoteKnowledgeSource))
	mux.Handle("GET /api/agents/{agent_id}/remote-knowledge-sources", w(agents.GetRemoteKnowledgeSources))
	mux.Handle("PATCH /api/agents/{agent_id}/remote-knowledge-sources/{id}", w(agents.ReSyncRemoteKnowledgeSource))
	mux.Handle("PUT /api/agents/{agent_id}/remote-knowledge-sources/{id}", w(agents.UpdateRemoteKnowledgeSource))
	mux.Handle("DELETE /api/agents/{agent_id}/remote-knowledge-sources/{id}", w(agents.DeleteRemoteKnowledgeSource))

	// Workflows
	mux.Handle("GET /api/workflows", w(workflows.List))
	mux.Handle("GET /api/workflows/{id}", w(workflows.ByID))
	mux.Handle("GET /api/workflows/{id}/script", w(workflows.Script))
	mux.Handle("GET /api/workflows/{id}/script.gpt", w(workflows.Script))
	mux.Handle("GET /api/workflows/{id}/script/tool.gpt", w(workflows.Script))
	mux.Handle("POST /api/workflows", w(workflows.Create))
	mux.Handle("PUT /api/workflows/{id}", w(workflows.Update))
	mux.Handle("DELETE /api/workflows/{id}", w(workflows.Delete))

	// Workflow files
	mux.Handle("GET /api/workflows/{id}/files", w(workflows.Files))
	mux.Handle("POST /api/workflows/{id}/files/{file}", w(workflows.UploadFile))
	mux.Handle("DELETE /api/workflows/{id}/files/{file}", w(workflows.DeleteFile))

	// Invoker
	mux.Handle("POST /api/invoke/{id}", w(invoker.Invoke))
	mux.Handle("POST /api/invoke/{id}/threads/{thread}", w(invoker.Invoke))

	// Threads
	mux.Handle("GET /api/threads", w(threads.List))
	mux.Handle("GET /api/threads/{id}", w(threads.ByID))
	mux.Handle("GET /api/threads/{id}/events", w(threads.Events))
	mux.Handle("DELETE /api/threads/{id}", w(threads.Delete))
	mux.Handle("PUT /api/threads/{id}", w(threads.Update))
	mux.Handle("GET /api/agents/{agent}/threads", w(threads.List))

	// Thread files
	mux.Handle("GET /api/threads/{id}/files", w(threads.Files))
	mux.Handle("POST /api/threads/{id}/files/{file}", w(threads.UploadFile))
	mux.Handle("DELETE /api/threads/{id}/files/{file}", w(threads.DeleteFile))

	// Thread knowledge files
	mux.Handle("GET /api/threads/{id}/knowledge", w(threads.Knowledge))
	mux.Handle("POST /api/threads/{id}/knowledge/{file}", w(threads.UploadKnowledge))
	mux.Handle("DELETE /api/threads/{id}/knowledge/{file...}", w(threads.DeleteKnowledge))

	// ToolRefs
	mux.Handle("GET /api/toolreferences", w(toolRefs.List))
	mux.Handle("GET /api/toolreferences/{id}", w(toolRefs.ByID))
	mux.Handle("POST /api/toolreferences", w(toolRefs.Create))
	mux.Handle("DELETE /api/toolreferences/{id}", w(toolRefs.Delete))
	mux.Handle("PUT /api/toolreferences/{id}", w(toolRefs.Update))

	// Runs
	mux.Handle("GET /api/runs", w(runs.List))
	mux.Handle("GET /api/runs/{id}", w(runs.ByID))
	mux.Handle("DELETE /api/runs/{id}", w(runs.Delete))
	mux.Handle("GET /api/runs/{id}/debug", w(runs.Debug))
	mux.Handle("GET /api/runs/{id}/events", w(runs.Events))
	mux.Handle("GET /api/threads/{thread}/runs", w(runs.List))
	mux.Handle("GET /api/agents/{agent}/runs", w(runs.List))
	mux.Handle("GET /api/agents/{agent}/threads/{thread}/runs", w(runs.List))
	mux.Handle("GET /api/workflows/{workflow}/runs", w(runs.List))
	mux.Handle("GET /api/workflows/{workflow}/threads/{thread}/runs", w(runs.List))

	// Credentials
	mux.Handle("GET /api/threads/{context}/credentials", w(handlers.ListCredentials))
	mux.Handle("GET /api/agents/{context}/credentials", w(handlers.ListCredentials))
	mux.Handle("GET /api/workflows/{context}/credentials", w(handlers.ListCredentials))
	mux.Handle("GET /api/credentials", w(handlers.ListCredentials))
	mux.Handle("DELETE /api/threads/{context}/credentials/{id}", w(handlers.DeleteCredential))
	mux.Handle("DELETE /api/agents/{context}/credentials/{id}", w(handlers.DeleteCredential))
	mux.Handle("DELETE /api/workflows/{context}/credentials/{id}", w(handlers.DeleteCredential))
	mux.Handle("DELETE /api/credentials/{id}", w(handlers.DeleteCredential))

	// Webhooks
	mux.Handle("POST /api/webhooks", w(webhooks.Create))
	mux.Handle("GET /api/webhooks", w(webhooks.List))
	mux.Handle("GET /api/webhooks/{id}", w(webhooks.ByID))
	mux.Handle("DELETE /api/webhooks/{id}", w(webhooks.Delete))
	mux.Handle("PUT /api/webhooks/{id}", w(webhooks.Update))
	mux.Handle("POST /api/webhooks/{id}", w(webhooks.Execute))

	// CronJobs
	mux.Handle("POST /api/cronjobs", w(cronJobs.Create))
	mux.Handle("GET /api/cronjobs", w(cronJobs.List))
	mux.Handle("GET /api/cronjobs/{id}", w(cronJobs.ByID))
	mux.Handle("DELETE /api/cronjobs/{id}", w(cronJobs.Delete))
	mux.Handle("PUT /api/cronjobs/{id}", w(cronJobs.Update))
	mux.Handle("POST /api/cronjobs/{id}", w(cronJobs.Execute))

	// Gateway APIs
	services.GatewayServer.AddRoutes(w, mux)

	// UI
	mux.Handle("/", ui.Handler())

	return mux, nil
}
