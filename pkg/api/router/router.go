package router

import (
	"net/http"

	"github.com/otto8-ai/otto8/apiclient"
	"github.com/otto8-ai/otto8/pkg/api/handlers"
	"github.com/otto8-ai/otto8/pkg/services"
	"github.com/otto8-ai/otto8/ui/router"
)

func Router(services *services.Services) (http.Handler, error) {
	ui := services.ProxyServer.Wrap(router.Init(&apiclient.Client{
		BaseURL: "http://localhost:8080",
	}, false))

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
	mux.Handle("GET /agents", w(agents.List))
	mux.Handle("GET /agents/{id}", w(agents.ByID))
	mux.Handle("GET /agents/{id}/script", w(agents.Script))
	mux.Handle("GET /agents/{id}/script.gpt", w(agents.Script))
	mux.Handle("GET /agents/{id}/script/tool.gpt", w(agents.Script))
	mux.Handle("POST /agents", w(agents.Create))
	mux.Handle("PUT /agents/{id}", w(agents.Update))
	mux.Handle("DELETE /agents/{id}", w(agents.Delete))

	// Agent files
	mux.Handle("GET /agents/{id}/files", w(agents.Files))
	mux.Handle("POST /agents/{id}/files/{file}", w(agents.UploadFile))
	mux.Handle("DELETE /agents/{id}/files/{file}", w(agents.DeleteFile))

	// Agent knowledge files
	mux.Handle("GET /agents/{id}/knowledge", w(agents.Knowledge))
	mux.Handle("POST /agents/{id}/knowledge/{file}", w(agents.UploadKnowledge))
	mux.Handle("DELETE /agents/{id}/knowledge/{file...}", w(agents.DeleteKnowledge))

	mux.Handle("POST /agents/{agent_id}/remote-knowledge-sources", w(agents.CreateRemoteKnowledgeSource))
	mux.Handle("GET /agents/{agent_id}/remote-knowledge-sources", w(agents.GetRemoteKnowledgeSources))
	mux.Handle("PATCH /agents/{agent_id}/remote-knowledge-sources/{id}", w(agents.ReSyncRemoteKnowledgeSource))
	mux.Handle("PUT /agents/{agent_id}/remote-knowledge-sources/{id}", w(agents.UpdateRemoteKnowledgeSource))
	mux.Handle("DELETE /agents/{agent_id}/remote-knowledge-sources/{id}", w(agents.DeleteRemoteKnowledgeSource))

	// Workflows
	mux.Handle("GET /workflows", w(workflows.List))
	mux.Handle("GET /workflows/{id}", w(workflows.ByID))
	mux.Handle("GET /workflows/{id}/script", w(workflows.Script))
	mux.Handle("GET /workflows/{id}/script.gpt", w(workflows.Script))
	mux.Handle("GET /workflows/{id}/script/tool.gpt", w(workflows.Script))
	mux.Handle("POST /workflows", w(workflows.Create))
	mux.Handle("PUT /workflows/{id}", w(workflows.Update))
	mux.Handle("DELETE /workflows/{id}", w(workflows.Delete))

	// Workflow files
	mux.Handle("GET /workflows/{id}/files", w(workflows.Files))
	mux.Handle("POST /workflows/{id}/files/{file}", w(workflows.UploadFile))
	mux.Handle("DELETE /workflows/{id}/files/{file}", w(workflows.DeleteFile))

	// Invoker
	mux.Handle("POST /invoke/{id}", w(invoker.Invoke))
	mux.Handle("POST /invoke/{id}/threads/{thread}", w(invoker.Invoke))

	// Threads
	mux.Handle("GET /threads", w(threads.List))
	mux.Handle("GET /threads/{id}", w(threads.ByID))
	mux.Handle("GET /threads/{id}/events", w(threads.Events))
	mux.Handle("DELETE /threads/{id}", w(threads.Delete))
	mux.Handle("PUT /threads/{id}", w(threads.Update))
	mux.Handle("GET /agents/{agent}/threads", w(threads.List))

	// Thread files
	mux.Handle("GET /threads/{id}/files", w(threads.Files))
	mux.Handle("POST /threads/{id}/files/{file}", w(threads.UploadFile))
	mux.Handle("DELETE /threads/{id}/files/{file}", w(threads.DeleteFile))

	// Thread knowledge files
	mux.Handle("GET /threads/{id}/knowledge", w(threads.Knowledge))
	mux.Handle("POST /threads/{id}/knowledge/{file}", w(threads.UploadKnowledge))
	mux.Handle("DELETE /threads/{id}/knowledge/{file...}", w(threads.DeleteKnowledge))

	// ToolRefs
	mux.Handle("GET /toolreferences", w(toolRefs.List))
	mux.Handle("GET /toolreferences/{id}", w(toolRefs.ByID))
	mux.Handle("POST /toolreferences", w(toolRefs.Create))
	mux.Handle("DELETE /toolreferences/{id}", w(toolRefs.Delete))
	mux.Handle("PUT /toolreferences/{id}", w(toolRefs.Update))

	// Runs
	mux.Handle("GET /runs", w(runs.List))
	mux.Handle("GET /runs/{id}", w(runs.ByID))
	mux.Handle("DELETE /runs/{id}", w(runs.Delete))
	mux.Handle("GET /runs/{id}/debug", w(runs.Debug))
	mux.Handle("GET /runs/{id}/events", w(runs.Events))
	mux.Handle("GET /threads/{thread}/runs", w(runs.List))
	mux.Handle("GET /agents/{agent}/runs", w(runs.List))
	mux.Handle("GET /agents/{agent}/threads/{thread}/runs", w(runs.List))
	mux.Handle("GET /workflows/{workflow}/runs", w(runs.List))
	mux.Handle("GET /workflows/{workflow}/threads/{thread}/runs", w(runs.List))

	// Credentials
	mux.Handle("GET /threads/{context}/credentials", w(handlers.ListCredentials))
	mux.Handle("GET /agents/{context}/credentials", w(handlers.ListCredentials))
	mux.Handle("GET /workflows/{context}/credentials", w(handlers.ListCredentials))
	mux.Handle("GET /credentials", w(handlers.ListCredentials))
	mux.Handle("DELETE /threads/{context}/credentials/{id}", w(handlers.DeleteCredential))
	mux.Handle("DELETE /agents/{context}/credentials/{id}", w(handlers.DeleteCredential))
	mux.Handle("DELETE /workflows/{context}/credentials/{id}", w(handlers.DeleteCredential))
	mux.Handle("DELETE /credentials/{id}", w(handlers.DeleteCredential))

	// Webhooks
	mux.Handle("POST /webhooks", w(webhooks.Create))
	mux.Handle("GET /webhooks", w(webhooks.List))
	mux.Handle("GET /webhooks/{id}", w(webhooks.ByID))
	mux.Handle("DELETE /webhooks/{id}", w(webhooks.Delete))
	mux.Handle("PUT /webhooks/{id}", w(webhooks.Update))
	mux.Handle("POST /webhooks/{id}", w(webhooks.Execute))

	// CronJobs
	mux.Handle("POST /cronjobs", w(cronJobs.Create))
	mux.Handle("GET /cronjobs", w(cronJobs.List))
	mux.Handle("GET /cronjobs/{id}", w(cronJobs.ByID))
	mux.Handle("DELETE /cronjobs/{id}", w(cronJobs.Delete))
	mux.Handle("PUT /cronjobs/{id}", w(cronJobs.Update))
	mux.Handle("POST /cronjobs/{id}", w(cronJobs.Execute))

	// Gateway APIs
	services.GatewayServer.AddRoutes(w, mux)

	// UI
	mux.Handle("/", ui)

	return mux, nil
}
