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
	threads := handlers.NewThreadHandler(services.WorkspaceClient)
	runs := handlers.NewRunHandler()

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
	mux.Handle("DELETE /agents/{id}/knowledge/{file}", w(agents.DeleteKnowledge))

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
	mux.Handle("DELETE /workflows/{id}/knowledge/{file}", w(workflows.DeleteKnowledge))

	mux.Handle("POST /workflows/{workflow_id}/onedrive-links", w(workflows.CreateOnedriveLinks))
	mux.Handle("GET /workflows/{workflow_id}/onedrive-links", w(workflows.GetOnedriveLinks))
	mux.Handle("PATCH /workflows/{workflow_id}/onedrive-links/{id}", w(workflows.ReSyncOnedriveLinks))
	mux.Handle("PUT /workflows/{workflow_id}/onedrive-links/{id}", w(workflows.UpdateOnedriveLinks))
	mux.Handle("DELETE /workflows/{workflow_id}/onedrive-links/{id}", w(workflows.DeleteOnedriveLinks))

	// Invoker
	mux.Handle("POST /invoke/{agent}", w(invoker.Invoke))
	mux.Handle("POST /invoke/{agent}/threads/{thread}", w(invoker.Invoke))

	// Threads
	mux.Handle("GET /threads", w(threads.List))
	mux.Handle("DELETE /threads/{id}", w(threads.Delete))
	mux.Handle("GET /agents/{agent}/threads", w(threads.List))

	// Thread files
	mux.Handle("GET /threads/{id}/files", w(threads.Files))
	mux.Handle("POST /threads/{id}/files/{file}", w(threads.UploadFile))
	mux.Handle("DELETE /threads/{id}/files/{file}", w(threads.DeleteFile))

	// Thread knowledge files
	mux.Handle("GET /threads/{id}/knowledge", w(threads.Knowledge))
	mux.Handle("POST /threads/{id}/knowledge", w(threads.IngestKnowledge))
	mux.Handle("POST /threads/{id}/knowledge/{file}", w(threads.UploadKnowledge))
	mux.Handle("DELETE /threads/{id}/knowledge/{file}", w(threads.DeleteKnowledge))

	// Runs
	mux.Handle("GET /runs", w(runs.List))
	mux.Handle("GET /runs/{id}", w(runs.ByID))
	mux.Handle("GET /runs/{id}/debug", w(runs.Debug))
	mux.Handle("GET /threads/{thread}/runs", w(runs.List))
	mux.Handle("GET /agents/{agent}/runs", w(runs.List))
	mux.Handle("GET /agents/{agent}/threads/{thread}/runs", w(runs.List))

	return mux, nil
}
