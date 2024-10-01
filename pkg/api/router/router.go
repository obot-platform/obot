package router

import (
	"net/http"

	"github.com/gptscript-ai/otto/pkg/api/handlers"
	"github.com/gptscript-ai/otto/pkg/services"
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

	// Agents
	mux.Handle("GET /agents", w(agents.List))
	mux.Handle("GET /agents/{id}", w(agents.ByID))
	mux.Handle("GET /agents/{id}/script", w(agents.Script))
	mux.Handle("POST /agents", w(agents.Create))
	mux.Handle("PUT /agents/{id}", w(agents.Update))
	mux.Handle("DELETE /agents/{id}", w(agents.Delete))

	// Agent files
	mux.Handle("GET /agents/{id}/files", w(agents.Files))
	mux.Handle("POST /agents/{id}/files/{file}", w(agents.UploadFile))
	mux.Handle("DELETE /agents/{id}/files/{file}", w(agents.DeleteFile))

	// Agent knowledge files
	mux.Handle("GET /agents/{id}/knowledge", w(agents.Knowledge))
	mux.Handle("POST /agents/{id}/knowledge", w(agents.IngestKnowledge))
	mux.Handle("POST /agents/{id}/knowledge/{file}", w(agents.UploadKnowledge))
	mux.Handle("DELETE /agents/{id}/knowledge/{file...}", w(agents.DeleteKnowledge))

	mux.Handle("POST /agents/{agent_id}/onedrive-links", w(agents.CreateOnedriveLinks))
	mux.Handle("GET /agents/{agent_id}/onedrive-links", w(agents.GetOnedriveLinks))
	mux.Handle("PATCH /agents/{agent_id}/onedrive-links/{id}", w(agents.ReSyncOnedriveLinks))
	mux.Handle("PUT /agents/{agent_id}/onedrive-links/{id}", w(agents.UpdateOnedriveLinks))
	mux.Handle("DELETE /agents/{agent_id}/onedrive-links/{id}", w(agents.DeleteOnedriveLinks))

	// Workflows
	mux.Handle("GET /workflows", w(workflows.List))
	mux.Handle("GET /workflows/{id}", w(workflows.ByID))
	mux.Handle("GET /workflows/{id}/script", w(workflows.Script))
	mux.Handle("POST /workflows", w(workflows.Create))
	mux.Handle("PUT /workflows/{id}", w(workflows.Update))
	mux.Handle("DELETE /workflows/{id}", w(workflows.Delete))

	// Workflow files
	mux.Handle("GET /workflows/{id}/files", w(workflows.Files))
	mux.Handle("POST /workflows/{id}/files/{file}", w(workflows.UploadFile))
	mux.Handle("DELETE /workflows/{id}/files/{file}", w(workflows.DeleteFile))

	// Workflow knowledge files
	mux.Handle("GET /workflows/{id}/knowledge", w(workflows.Knowledge))
	mux.Handle("POST /workflows/{id}/knowledge", w(workflows.IngestKnowledge))
	mux.Handle("POST /workflows/{id}/knowledge/{file}", w(workflows.UploadKnowledge))
	mux.Handle("DELETE /workflows/{id}/knowledge/{file...}", w(workflows.DeleteKnowledge))

	mux.Handle("POST /workflows/{workflow_id}/onedrive-links", w(workflows.CreateOnedriveLinks))
	mux.Handle("GET /workflows/{workflow_id}/onedrive-links", w(workflows.GetOnedriveLinks))
	mux.Handle("PATCH /workflows/{workflow_id}/onedrive-links/{id}", w(workflows.ReSyncOnedriveLinks))
	mux.Handle("PUT /workflows/{workflow_id}/onedrive-links/{id}", w(workflows.UpdateOnedriveLinks))
	mux.Handle("DELETE /workflows/{workflow_id}/onedrive-links/{id}", w(workflows.DeleteOnedriveLinks))

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
	mux.Handle("POST /threads/{id}/knowledge", w(threads.IngestKnowledge))
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

	return mux, nil
}
