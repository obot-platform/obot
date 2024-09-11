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
	invoker := handlers.NewInvokeHandler(services.Invoker)
	threads := handlers.NewThreadHandler(services.WorkspaceClient)
	runs := handlers.NewRunHandler()

	// Agents
	mux.Handle("GET /agents", w(agents.List))
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

	// Invoker
	mux.Handle("POST /invoke/{agent}", w(invoker.Invoke))
	mux.Handle("POST /invoke/{agent}/threads/{thread}", w(invoker.Invoke))

	// Threads
	mux.Handle("GET /threads", w(threads.List))
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
	mux.Handle("GET /runs/{run}/debug", w(runs.Debug))
	mux.Handle("GET /threads/{thread}/runs", w(runs.List))
	mux.Handle("GET /agents/{agent}/runs", w(runs.List))
	mux.Handle("GET /agents/{agent}/threads/{thread}/runs", w(runs.List))

	return mux, nil
}
