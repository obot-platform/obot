package authz

import (
	"net/http"
	"slices"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apiserver/pkg/authentication/user"
)

var apiResources = []string{
	"GET    /api/all-mcps/servers/{mcpserver_id}/tools",
	"GET    /api/all-mcps/servers/{mcpserver_id}/resources",
	"GET    /api/all-mcps/servers/{mcpserver_id}/resources/{resource_uri}",
	"GET    /api/all-mcps/servers/{mcpserver_id}/prompts",
	"GET    /api/all-mcps/servers/{mcpserver_id}/prompts/{prompt_name}",
	"GET    /oauth/callback/{oauth_request_id}/{mcp_id}",
	"GET    /oauth/mcp/callback",
	"GET    /mcp-connect/{mcp_id}",
	"POST   /mcp-connect/{mcp_id}",
	"DELETE /mcp-connect/{mcp_id}",
	"GET    /api/assistants",
	"GET    /api/assistants/{assistant_id}",
	"GET    /api/assistants/{assistant_id}/projects",
	"POST   /api/assistants/{assistant_id}/projects",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/copy",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/credentials",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/credentials/{credential_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/default-model",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/env",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/env",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/file/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/files",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/files/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/files/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/files/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/invitations",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/invitations",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/invitations/{code}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/knowledge",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/knowledge/{file}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/local-credentials",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/local-credentials/{credential_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/members",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/members/{member_id}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/memories",
	"PUT /api/assistants/{assistant_id}/projects/{project_id}/memories/{memory_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/memories",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/memories",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/memories/{memory_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/mcpservers",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/mcpservers",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/launch",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/check-oauth",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/oauth-url",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/prompts",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/prompts/{prompt_name}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/resources",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/resources/{resource_uri}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/tools",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/tools",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/tools/{thread_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/mcpservers/{project_mcp_server_id}/tools/{thread_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/model-providers",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/model-providers/{model_provider_id}/configure",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/model-providers/{model_provider_id}/deconfigure",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/model-providers/{model_provider_id}/reveal",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/model-providers/{model_provider_id}/validate",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/model-providers/{model_provider_id}/available-models",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/share",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/share",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/share",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/share",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/shell",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/slack",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/slack",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/slack",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tables",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tables/{table_name}/rows",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}",
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
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/files/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tasks/{task_id}/runs/{run_id}/steps/{step_id}/run",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/template",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/template",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/template",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/threads",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}",
	"POST 	/api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/abort",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/events",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/file/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/file/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/file/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/files",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/files/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/files/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/files/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/invoke",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/knowledge-files",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/knowledge-files/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/knowledge-files/{file...}",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/knowledge-files/{file...}",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/tools",
	"PUT    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/tools",
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/threads/{thread_id}/default-model",
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
	"GET    /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/local-authenticate",
	"DELETE /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/local-deauthenticate",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/tools/{tool_id}/test",
	"POST   /api/assistants/{assistant_id}/projects/{project_id}/upgrade-from-template",
	"GET    /api/mcp-server-instances",
	"GET    /api/mcp-server-instances/{mcp_server_instance_id}",
	"POST   /api/mcp-server-instances",
	"DELETE /api/mcp-server-instances/{mcp_server_instance_id}",
	"DELETE /api/mcp-server-instances/{mcp_server_instance_id}/oauth",
	"GET    /api/mcp-servers",
	"GET    /api/mcp-servers/{mcpserver_id}",
	"POST   /api/mcp-servers/{mcpserver_id}/launch",
	"POST   /api/mcp-servers/{mcpserver_id}/check-oauth",
	"GET    /api/mcp-servers/{mcpserver_id}/oauth-url",
	"POST   /api/mcp-servers",
	"DELETE /api/mcp-servers/{mcpserver_id}",
	"DELETE /api/mcp-servers/{mcpserver_id}/oauth",
	"PUT	/api/mcp-servers/{mcpserver_id}/alias",
	"POST   /api/mcp-servers/{mcpserver_id}/update-url",
	"POST   /api/mcp-servers/{mcpserver_id}/configure",
	"POST   /api/mcp-servers/{mcpserver_id}/deconfigure",
	"POST   /api/mcp-servers/{mcpserver_id}/reveal",
	"GET    /api/mcp-servers/{mcpserver_id}/tools",
	"GET    /api/mcp-servers/{mcpserver_id}/resources",
	"GET    /api/mcp-servers/{mcpserver_id}/resources/{resource_uri}",
	"GET    /api/mcp-servers/{mcpserver_id}/prompts",
	"GET    /api/mcp-servers/{mcpserver_id}/prompts/{prompt_name}",
	"GET    /api/projects",
	"GET    /api/projects/{project_id}",
	"POST   /api/prompt",
	"GET    /api/shares",
	"POST   /api/shares/{share_public_id}",
	"GET    /api/shares/{share_public_id}",
	"GET    /api/templates",
	"GET    /api/templates/{template_public_id}",
	"POST   /api/templates/{template_public_id}",
	"DELETE /api/threads/{thread_id}",
	"GET    /api/threads/{thread_id}",
	"PUT    /api/threads/{thread_id}",
	"POST   /api/threads/{thread_id}/abort",
	"GET    /api/threads/{thread_id}/events",
	"DELETE /api/threads/{thread_id}/file/{file...}",
	"GET    /api/threads/{thread_id}/file/{file...}",
	"POST   /api/threads/{thread_id}/file/{file...}",
	"GET    /api/threads/{thread_id}/files",
	"DELETE /api/threads/{thread_id}/files/{file...}",
	"GET    /api/threads/{thread_id}/files/{file...}",
	"POST   /api/threads/{thread_id}/files/{file...}",
	"GET    /api/threads/{thread_id}/knowledge-files",
	"DELETE /api/threads/{thread_id}/knowledge-files/{file...}",
	"GET    /api/threads/{thread_id}/knowledge-files/{file...}",
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
	"GET    /api/tool-references",
	"GET    /api/tool-references/{id}",
	"GET    /{ui}/projects/{id}",
	"GET    /api/users/{user_id}",
	"PATCH  /api/users/{user_id}",
	"GET    /api/users/{user_id}/activities",
	"GET    /api/users/{user_id}/token-usage",
	"GET    /api/users/{user_id}/total-token-usage",
	"GET    /api/users/{user_id}/remaining-token-usage",
	"GET    /api/workspaces",
	"GET    /api/workspaces/{workspace_id}",
	"GET    /api/workspaces/{workspace_id}/servers",
	"POST   /api/workspaces/{workspace_id}/servers",
	"DELETE /api/workspaces/{workspace_id}/servers/{mcp_server_id}",
	"GET    /api/workspaces/{workspace_id}/servers/{mcp_server_id}",
	"PUT    /api/workspaces/{workspace_id}/servers/{mcp_server_id}",
	"POST   /api/workspaces/{workspace_id}/servers/{mcp_server_id}/launch",
	"POST   /api/workspaces/{workspace_id}/servers/{mcp_server_id}/check-oauth",
	"GET    /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth-url",
	"DELETE /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth",
	"POST   /api/workspaces/{workspace_id}/servers/{mcp_server_id}/configure",
	"POST   /api/workspaces/{workspace_id}/servers/{mcp_server_id}/deconfigure",
	"POST   /api/workspaces/{workspace_id}/servers/{mcp_server_id}/reveal",
	"GET    /api/workspaces/{workspace_id}/servers/{mcp_server_id}/instances",
	"GET    /api/workspaces/{workspace_id}/entries",
	"POST   /api/workspaces/{workspace_id}/entries",
	"DELETE /api/workspaces/{workspace_id}/entries/{entry_id}",
	"GET    /api/workspaces/{workspace_id}/entries/{entry_id}",
	"GET    /api/workspaces/{workspace_id}/entries/{entry_id}/servers",
	"GET    /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcpserver_id}",
	"GET    /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcpserver_id}/details",
	"GET    /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcpserver_id}/logs",
	"POST   /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcpserver_id}/restart",
	"PUT    /api/workspaces/{workspace_id}/entries/{entry_id}",
	"POST   /api/workspaces/{workspace_id}/entries/{entry_id}/generate-tool-previews",
	"POST   /api/workspaces/{workspace_id}/entries/{entry_id}/generate-tool-previews/oauth-url",
	"GET    /api/workspaces/{workspace_id}/access-control-rules",
	"POST   /api/workspaces/{workspace_id}/access-control-rules",
	"DELETE /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}",
	"GET    /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}",
	"PUT    /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}",
}

type Resources struct {
	AssistantID         string
	ProjectID           string
	ThreadID            string
	ThreadShareID       string
	TemplateID          string
	TaskID              string
	MCPServerID         string
	MCPServerInstanceID string
	ProjectMCPServerID  string
	// MCPID can be the ID of an MCPServer, an MCPServerInstance, or MCPServerCatalogEntry. It is used for interaction with the MCP gateway.
	MCPID                  string
	RunID                  string
	WorkflowID             string
	PendingAuthorizationID string
	ToolID                 string
	WorkspaceID            string
	Authorizated           ResourcesAuthorized
}

type ResourcesAuthorized struct {
	Assistant          *v1.Agent
	Project            *v1.Thread
	Thread             *v1.Thread
	ThreadShare        *v1.ThreadShare
	Task               *v1.Workflow
	MCPServer          *v1.MCPServer
	MCPServerInstance  *v1.MCPServerInstance
	Run                *v1.WorkflowExecution
	Workflow           *v1.Workflow
	Tool               *v1.Tool
	PowerUserWorkspace *v1.PowerUserWorkspace
}

func (a *Authorizer) evaluateResources(req *http.Request, vars GetVar, user user.Info) (bool, error) {
	resources := Resources{
		AssistantID:            vars("assistant_id"),
		ProjectID:              vars("project_id"),
		ThreadID:               vars("thread_id"),
		TaskID:                 vars("task_id"),
		RunID:                  vars("run_id"),
		WorkflowID:             vars("workflow_id"),
		MCPServerID:            vars("mcpserver_id"),
		MCPServerInstanceID:    vars("mcp_server_instance_id"),
		ProjectMCPServerID:     vars("project_mcp_server_id"),
		MCPID:                  vars("mcp_id"), // this will be either a server ID or a server instance ID
		PendingAuthorizationID: vars("pending_authorization_id"),
		ThreadShareID:          vars("share_public_id"),
		TemplateID:             vars("template_public_id"),
		ToolID:                 vars("tool_id"),
		WorkspaceID:            vars("workspace_id"),
	}

	if !a.checkUser(user, vars("user_id")) {
		return false, nil
	}

	if ok, err := a.checkPowerUserWorkspace(req, &resources, user); !ok || err != nil {
		return false, err
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

	if ok, err := a.checkTemplate(req, &resources); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkThread(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkTask(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkMCPServer(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkMCPServerInstance(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkProjectMCPServer(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkMCPID(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkRun(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkWorkflow(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkTools(req, &resources, user); !ok || err != nil {
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
