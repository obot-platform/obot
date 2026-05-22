package authz

import (
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apiserver/pkg/authentication/user"
)

var apiResources = map[string][]string{
	types.GroupBasic: {
		"GET    /api/all-mcps/servers/{mcpserver_id}/tools",
		"GET    /api/all-mcps/servers/{mcpserver_id}/resources",
		"GET    /api/all-mcps/servers/{mcpserver_id}/resources/{resource_uri}",
		"GET    /api/all-mcps/servers/{mcpserver_id}/prompts",
		"GET    /api/all-mcps/servers/{mcpserver_id}/prompts/{prompt_name}",
		"GET    /oauth/callback/{oauth_request_id}",
		"GET    /oauth/callback/{oauth_request_id}/{mcp_id}",
		"GET    /oauth/mcp/callback",
		"GET    /auth/mcp/composite/{mcp_id}",
		"GET    /api/oauth/composite/{mcp_id}",
		"GET    /mcp-connect/{mcp_id}",
		"POST   /mcp-connect/{mcp_id}",
		"DELETE /mcp-connect/{mcp_id}",
		"GET    /mcp-connect/{mcp_id}/",
		"POST   /mcp-connect/{mcp_id}/",
		"DELETE /mcp-connect/{mcp_id}/",
		"GET    /api/mcp-stats/{mcp_id}",
		"GET    /api/mcp-audit-logs/{mcp_id}",
		"GET    /api/mcp-server-instances",
		"GET    /api/mcp-server-instances/{mcp_server_instance_id}",
		"POST   /api/mcp-server-instances",
		"DELETE /api/mcp-server-instances/{mcp_server_instance_id}",
		"DELETE /api/mcp-server-instances/{mcp_server_instance_id}/oauth",
		"POST   /api/mcp-server-instances/{mcp_server_instance_id}/reveal",
		"POST   /api/mcp-server-instances/{mcp_server_instance_id}/configure",
		"POST   /api/mcp-server-instances/{mcp_server_instance_id}/deconfigure",
		"GET    /api/mcp-servers",
		"GET    /api/mcp-servers/{mcpserver_id}",
		"POST   /api/mcp-servers/{mcpserver_id}/launch",
		"POST   /api/mcp-servers/{mcpserver_id}/check-oauth",
		"GET    /api/mcp-servers/{mcpserver_id}/oauth-url",
		"POST   /api/mcp-servers",
		"DELETE /api/mcp-servers/{mcpserver_id}",
		"DELETE /api/mcp-servers/{mcpserver_id}/oauth",
		"GET    /api/mcp-servers/{mcpserver_id}/logs",
		"PUT	/api/mcp-servers/{mcpserver_id}/alias",
		"POST   /api/mcp-servers/{mcpserver_id}/update-url",
		"POST   /api/mcp-servers/{mcpserver_id}/configure",
		"POST   /api/mcp-servers/{mcpserver_id}/deconfigure",
		"POST   /api/mcp-servers/{mcpserver_id}/reveal",
		"POST   /api/mcp-servers/{mcpserver_id}/restart",
		"POST   /api/mcp-servers/{mcpserver_id}/trigger-update",
		"GET    /api/mcp-servers/{mcpserver_id}/tools",
		"GET    /api/mcp-servers/{mcpserver_id}/resources",
		"GET    /api/mcp-servers/{mcpserver_id}/resources/{resource_uri}",
		"GET    /api/mcp-servers/{mcpserver_id}/prompts",
		"GET    /api/mcp-servers/{mcpserver_id}/prompts/{prompt_name}",

		"GET    /api/published-artifacts/{artifact_id}",
		"GET    /api/published-artifacts/{artifact_id}/download",
		"GET    /api/published-artifacts/{artifact_id}/{artifact_version}/skill",
		"PUT    /api/published-artifacts/{artifact_id}",
		"DELETE /api/published-artifacts/{artifact_id}",

		"GET    /api/tool-references",
		"GET    /api/tool-references/{id}",
		"GET    /api/users/{user_id}",
		"PATCH  /api/users/{user_id}",
		"GET    /api/users/{user_id}/activities",
		"GET    /api/users/{user_id}/token-usage",
		"GET    /api/users/{user_id}/total-token-usage",
		"GET    /api/users/{user_id}/remaining-token-usage",
		"GET    /api/workspaces",
		"GET    /api/projects/{project_id}",
		"PUT    /api/projects/{project_id}",
		"DELETE /api/projects/{project_id}",
		"POST   /api/projects/{project_id}/agents",
		"GET    /api/projects/{project_id}/agents",
		"GET    /api/projects/{project_id}/agents/{nanobot_agent_id}",
		"PUT    /api/projects/{project_id}/agents/{nanobot_agent_id}",
		"DELETE /api/projects/{project_id}/agents/{nanobot_agent_id}",
		"POST   /api/projects/{project_id}/agents/{nanobot_agent_id}/launch",
	},
	types.GroupPowerUser: {
		"GET    /api/workspaces/{workspace_id}",
		"GET    /api/workspaces/{workspace_id}/entries",
		"POST   /api/workspaces/{workspace_id}/entries",
		"DELETE /api/workspaces/{workspace_id}/entries/{entry_id}",
		"GET    /api/workspaces/{workspace_id}/entries/{entry_id}",
		"PUT    /api/workspaces/{workspace_id}/entries/{entry_id}",
		"GET    /api/workspaces/{workspace_id}/entries/{entry_id}/servers",
		"GET    /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcpserver_id}",
		"POST   /api/workspaces/{workspace_id}/entries/{entry_id}/generate-tool-previews",
		"POST   /api/workspaces/{workspace_id}/entries/{entry_id}/generate-tool-previews/oauth-url",
		"GET    /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcpserver_id}/details",
		"GET    /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcpserver_id}/logs",
		"POST   /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcpserver_id}/restart",
		"POST   /api/workspaces/{workspace_id}/entries/{entry_id}/servers/{mcpserver_id}/trigger-update",
		"GET    /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials",
		"POST   /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials",
		"DELETE /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials",
	},
	types.GroupPowerUserPlus: {
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
		"GET    /api/workspaces/{workspace_id}/servers/{mcp_server_id}/details",
		"GET    /api/workspaces/{workspace_id}/servers/{mcp_server_id}/logs",
		"POST   /api/workspaces/{workspace_id}/servers/{mcp_server_id}/restart",
		"GET    /api/workspaces/{workspace_id}/access-control-rules",
		"POST   /api/workspaces/{workspace_id}/access-control-rules",
		"DELETE /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}",
		"GET    /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}",
		"PUT    /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}",
	},
	types.GroupAPIKey: {
		"GET    /mcp-connect/{mcp_id}",
		"POST   /mcp-connect/{mcp_id}",
		"DELETE /mcp-connect/{mcp_id}",
		"GET    /mcp-connect/{mcp_id}/",
		"POST   /mcp-connect/{mcp_id}/",
		"DELETE /mcp-connect/{mcp_id}/",
		"GET    /api/published-artifacts/{artifact_id}",
		"PUT    /api/published-artifacts/{artifact_id}",
		"GET    /api/published-artifacts/{artifact_id}/download",
		"GET    /api/published-artifacts/{artifact_id}/{artifact_version}/skill",
	},
}

type Resources struct {
	MCPServerID         string
	MCPServerInstanceID string
	// MCPID can be the ID of an MCPServer, an MCPServerInstance, or MCPServerCatalogEntry. It is used for interaction with the MCP gateway.
	MCPID               string
	WorkspaceID         string
	NanobotAgentID      string
	ProjectID           string
	PublishedArtifactID string
	ArtifactVersion     string
	Authorizated        ResourcesAuthorized
}

type ResourcesAuthorized struct {
	MCPServer          *v1.MCPServer
	MCPServerInstance  *v1.MCPServerInstance
	PowerUserWorkspace *v1.PowerUserWorkspace
	NanobotAgent       *v1.NanobotAgent
	Project            *v1.Project
	PublishedArtifact  *v1.PublishedArtifact
}

func (a *Authorizer) evaluateResources(req *http.Request, vars GetVar, user user.Info) (bool, error) {
	resources := Resources{
		MCPServerID:         vars("mcpserver_id"),
		MCPServerInstanceID: vars("mcp_server_instance_id"),
		MCPID:               vars("mcp_id"), // this can be a server ID, server instance ID, or a catalog entry ID
		WorkspaceID:         vars("workspace_id"),
		NanobotAgentID:      vars("nanobot_agent_id"),
		ProjectID:           vars("project_id"),
		PublishedArtifactID: vars("artifact_id"),
		ArtifactVersion:     vars("artifact_version"),
	}

	if !a.checkUser(user, vars("user_id")) {
		return false, nil
	}

	if ok, err := a.checkPowerUserWorkspace(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkMCPServer(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkMCPServerInstance(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkMCPID(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkProject(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkNanobotAgent(req, &resources, user); !ok || err != nil {
		return false, err
	}

	if ok, err := a.checkPublishedArtifact(req, &resources, user); !ok || err != nil {
		return false, err
	}

	return true, nil
}

func (a *Authorizer) authorizeAPIResources(req *http.Request, user user.Info) bool {
	for _, group := range user.GetGroups() {
		vars, matches := a.apiResources[group].Match(req)
		if !matches {
			continue
		}

		ok, err := a.evaluateResources(req, vars, user)
		if err == nil && ok {
			return true
		}
	}

	return false
}
