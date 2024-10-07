package router

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/gptscript-ai/otto/apiclient"
	"github.com/gptscript-ai/otto/ui/handlers"
	"github.com/gptscript-ai/otto/ui/layouts"
	"github.com/gptscript-ai/otto/ui/static"
	"github.com/gptscript-ai/otto/ui/webcontext"
)

func Init(client *apiclient.Client, devMode bool) http.Handler {
	router := http.NewServeMux()

	router.Handle("GET /write", templ.Handler(layouts.Write()))
	router.Handle("POST /chat", errors(handlers.Chat))
	router.Handle("GET /chat", errors(handlers.Chat))
	router.Handle("GET /events", errors(handlers.Events))

	// Workflows
	router.Handle("GET /ui/workflows/{id}/edit", errors(handlers.EditWorkflow))
	router.Handle("POST /ui/workflows/{id}/run", errors(handlers.RunWorkflow))

	// Steps
	router.Handle("PUT /ui/workflows/{workflow_id}/steps/{id}", errors(handlers.UpdateStep))
	router.Handle("DELETE /ui/workflows/{workflow_id}/steps/{id}", errors(handlers.DeleteStep))
	router.Handle("DELETE /ui/workflows/{workflow_id}", errors(handlers.DeleteWorkflow))
	router.Handle("POST /ui/workflows/new", errors(handlers.CreateWorkflow))
	router.Handle("GET /ui/workflows/{workflow_id}/steps/new", errors(handlers.NewStep))
	router.Handle("GET /ui/workflows/{workflow_id}/threads/{thread_id}", errors(handlers.WorkflowThread))
	router.Handle("GET /ui/workflows", errors(handlers.Workflows))
	router.Handle("POST /ui/workflows/{workflow_id}/steps/{parent_id}/add/{id}", errors(handlers.AddStep))

	// Threads
	router.Handle("GET /ui/threads/{id}", errors(handlers.Thread))
	router.Handle("GET /ui/threads/{id}/events", errors(handlers.ThreadEvents))
	router.Handle("GET /ui/login/complete", errors(handlers.LoginComplete))
	if devMode {
		router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	} else {
		router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static.FS)))
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := webcontext.WithClient(r.Context(), client.WithCookie(r.Header.Get("Cookie")))
		router.ServeHTTP(w, r.WithContext(ctx))
	})
}
