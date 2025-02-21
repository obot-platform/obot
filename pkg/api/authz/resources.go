package authz

import (
	"bytes"
	"context"
	"io"
	"net/http"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
)

var resources = []string{
	"GET    /api/assistants",
	"GET    /api/assistants/{assistant_id}",
	"GET    /api/assistants/{assistant_id}/pending-authorizations",
	"PUT    /api/assistants/{assistant_id}/pending-authorizations/{pending_authorization_id}",
	"DELETE /api/assistants/{assistant_id}/pending-authorizations/{pending_authorization_id}",
	"GET    /api/assistants/{assistant_id}/projects",
	"POST   /api/assistants/{assistant_id}/projects",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/authorizations",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/authorizations",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/credentials",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/credentials/{credential_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/env",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/env",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/files",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/files/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/knowledge",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file}",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/shell",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tables",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tables/{table_name}/rows",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{id}/runs/{run_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/events",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/events",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/run",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/abort",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/events",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/events",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/file/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/file/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/templates",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/templates",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/templates/{template_id}",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/templates/{template_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/threads",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}",
	"POST 	/api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/abort",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/events",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/invoke",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tools",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/tools",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tools",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/authenticate",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/custom",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/deauthenticate",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/env",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/env",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/test",
	"GET    /api/projects",
	"POST   /api/prompt",
	"GET    /api/templates",
	"GET    /api/templates/{template_id}",
	"POST   /api/templates/{template_id}/projects",
	"PUT    /api/threads/{thread_id}",
	"DELETE /api/threads/{thread_id}",
	"POST   /api/threads/{thread_id}/abort",
	"GET    /api/threads/{thread_id}/events",
	"DELETE /api/threads/{thread_id}/files/{file...}",
	"GET    /api/threads/{thread_id}/files/{file...}",
	"POST   /api/threads/{thread_id}/files/{file...}",
	"GET    /api/threads/{thread_id}/files",
	"DELETE /api/threads/{thread_id}/knowledge-files/{file...}",
	"POST   /api/threads/{thread_id}/knowledge-files/{file}",
	"GET    /api/threads/{thread_id}/knowledge-files",
	"GET    /api/threads/{thread_id}/tables/{table}/rows",
	"GET    /api/threads/{thread_id}/tables",
	"GET    /api/threads/{thread_id}/tasks",
	"POST   /api/threads/{thread_id}/tasks",
	"GET    /api/threads/{thread_id}/tasks/{task_id}",
	"PUT    /api/threads/{thread_id}/tasks/{task_id}",
	"POST   /api/threads/{thread_id}/tasks/{task_id}/run",
	"GET    /api/threads/{thread_id}/tasks/{task_id}/runs",
	"GET    /api/threads/{thread_id}/tasks/{task_id}/runs/{run_id}",
	"GET    /api/threads/{thread_id}",
	"GET    /api/threads/{thread_id}/workflows",
	"GET    /api/threads/{thread_id}/workflows/{workflow_id}/executions",
	"GET    /{ui}/projects/{id}",
}

type Resources struct {
	AssistantID            string
	ProjectID              string
	ThreadID               string
	TemplateID             string
	TaskID                 string
	RunID                  string
	WorkflowID             string
	PendingAuthorizationID string
	Authorizated           ResourcesAuthorized
}

type ResourcesAuthorized struct {
	Assistant            *v1.Agent
	Project              *v1.Thread
	Thread               *v1.Thread
	Template             *v1.ThreadTemplate
	Task                 *v1.Workflow
	Run                  *v1.WorkflowExecution
	Workflow             *v1.Workflow
	PendingAuthorization *v1.ThreadAuthorization
}

func handleError(rw http.ResponseWriter, err error) {
	if apierrors.IsNotFound(err) {
		http.Error(rw, err.Error(), http.StatusNotFound)
	} else if err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
	} else {
		rw.WriteHeader(http.StatusForbidden)
	}
}

type userKey struct{}

func (a *Authorizer) evaluateResources(rw http.ResponseWriter, req *http.Request) {
	user, ok := req.Context().Value(userKey{}).(user.Info)
	if !ok {
		return
	}

	resources := Resources{
		AssistantID:            req.PathValue("assistant_id"),
		ProjectID:              req.PathValue("project_id"),
		ThreadID:               req.PathValue("thread_id"),
		TemplateID:             req.PathValue("template_id"),
		TaskID:                 req.PathValue("task_id"),
		RunID:                  req.PathValue("run_id"),
		WorkflowID:             req.PathValue("workflow_id"),
		PendingAuthorizationID: req.PathValue("pending_authorization_id"),
	}

	if ok, err := a.checkAssistant(req, &resources, user); !ok || err != nil {
		handleError(rw, err)
		return
	}

	if ok, err := a.checkProject(req, &resources, user); !ok || err != nil {
		handleError(rw, err)
		return
	}

	if ok, err := a.checkThread(req, &resources, user); !ok || err != nil {
		handleError(rw, err)
		return
	}

	if ok, err := a.checkTemplate(req, &resources, user); !ok || err != nil {
		handleError(rw, err)
		return
	}

	if ok, err := a.checkTask(req, &resources, user); !ok || err != nil {
		handleError(rw, err)
		return
	}

	if ok, err := a.checkRun(req, &resources, user); !ok || err != nil {
		handleError(rw, err)
		return
	}

	if ok, err := a.checkWorkflow(req, &resources, user); !ok || err != nil {
		handleError(rw, err)
		return
	}

	if ok, err := a.checkPendingAuthorization(req, &resources, user); !ok || err != nil {
		handleError(rw, err)
		return
	}

	if ok, err := a.checkUI(req, &resources, user); !ok || err != nil {
		handleError(rw, err)
		return
	}

	rw.WriteHeader(http.StatusAccepted)
}

type responseWriter struct {
	io.Writer
	code int
}

func (r *responseWriter) Header() http.Header {
	return http.Header{}
}

func (r *responseWriter) WriteHeader(statusCode int) {
	r.code = statusCode
}

func (a *Authorizer) authorizeResource(req *http.Request, user user.Info) bool {
	h, pattern := a.resourcesMux.Handler(req)
	if pattern == "" {
		return false
	}

	buffer := bytes.NewBuffer(nil)
	rw := responseWriter{
		Writer: buffer,
	}

	h.ServeHTTP(&rw, req.WithContext(context.WithValue(req.Context(), userKey{}, user)))
	return rw.code == http.StatusAccepted
}
