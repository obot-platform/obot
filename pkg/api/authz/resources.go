package authz

import (
	"net/http"
	"slices"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apiserver/pkg/authentication/user"
)

var apiResources = []string{
	"GET    /api/assistants",
	"GET    /api/assistants/{assistant_id}",
	"GET    /api/assistants/{assistant_id}/pending-authorizations",
	"DELETE /api/assistants/{assistant_id}/pending-authorizations/{pending_authorization_id}",
	"PUT    /api/assistants/{assistant_id}/pending-authorizations/{pending_authorization_id}",
	"GET    /api/assistants/{assistant_id}/projects",
	"POST   /api/assistants/{assistant_id}/projects",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/authorizations",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/authorizations",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/copy",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/credentials",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/credentials/{credential_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/env",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/env",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/files",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/knowledge",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/local-credentials",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/local-credentials/{credential_id}",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/share",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/share",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/share",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/share",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/shell",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tables",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tables/{table_name}/rows",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/events",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/events",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/run",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/abort",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/events",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/events",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/file/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/file/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/threads",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}",
	"POST 	/api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/abort",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/events",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/file/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/file/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/file/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/files",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/invoke",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tools",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tools",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/tools",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/authenticate",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/custom",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/deauthenticate",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/env",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/env",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/test",
	"GET    /api/projects",
	"GET    /api/projects/{project_id}",
	"POST   /api/prompt",
	"DELETE /api/threads/{thread_id}",
	"GET    /api/threads/{thread_id}",
	"PUT    /api/threads/{thread_id}",
	"POST   /api/threads/{thread_id}/abort",
	"GET    /api/threads/{thread_id}/events",
	"GET    /api/threads/{thread_id}/files",
	"DELETE /api/threads/{thread_id}/files/{file...}",
	"GET    /api/threads/{thread_id}/files/{file...}",
	"POST   /api/threads/{thread_id}/files/{file...}",
	"GET    /api/threads/{thread_id}/knowledge-files",
	"DELETE /api/threads/{thread_id}/knowledge-files/{file...}",
	"POST   /api/threads/{thread_id}/knowledge-files/{file}",
	"GET    /api/threads/{thread_id}/tables",
	"GET    /api/threads/{thread_id}/tables/{table}/rows",
	"GET    /api/threads/{thread_id}/tasks",
	"POST   /api/threads/{thread_id}/tasks",
	"GET    /api/threads/{thread_id}/tasks/{task_id}",
	"PUT    /api/threads/{thread_id}/tasks/{task_id}",
	"POST   /api/threads/{thread_id}/tasks/{task_id}/run",
	"GET    /api/threads/{thread_id}/tasks/{task_id}/runs",
	"GET    /api/threads/{thread_id}/tasks/{task_id}/runs/{run_id}",
	"GET    /api/threads/{thread_id}/workflows",
	"GET    /api/threads/{thread_id}/workflows/{workflow_id}/executions",
	"GET    /api/shares",
	"POST   /api/shares/{share_public_id}",
	"GET    /{ui}/projects/{id}",
}

type Resources struct {
	AssistantID            string
	ProjectID              string
	ThreadID               string
	ThreadShareID          string
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
	ThreadShare          *v1.ThreadShare
	Task                 *v1.Workflow
	Run                  *v1.WorkflowExecution
	Workflow             *v1.Workflow
	PendingAuthorization *v1.ThreadAuthorization
}

func (a *Authorizer) evaluateResources(req *http.Request, vars GetVar, user user.Info) (bool, error) {
	resources := Resources{
		AssistantID:            vars("assistant_id"),
		ProjectID:              vars("project_id"),
		ThreadID:               vars("thread_id"),
		TaskID:                 vars("task_id"),
		RunID:                  vars("run_id"),
		WorkflowID:             vars("workflow_id"),
		PendingAuthorizationID: vars("pending_authorization_id"),
		ThreadShareID:          vars("share_public_id"),
	}

	if ok, err := a.checkAssistant(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkProject(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkThreadShare(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkThread(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkTask(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkRun(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkWorkflow(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkPendingAuthorization(req, &resources, user); !ok || err != nil {
		return false, err
	}

	return true, nil
}

func (a *Authorizer) authorizeAPIResources(req *http.Request, user user.Info) bool {
	vars, matches := a.apiResources.Match(req)
	if !matches {
		return false
	}

	if !slices.Contains(user.GetGroups(), AuthenticatedGroup) {
		// All API resources access must be authenticated
		return false
	}

	ok, err := a.evaluateResources(req, vars, user)
	if err != nil {
		return false
	}

	return ok
}
