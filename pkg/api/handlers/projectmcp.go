package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/projects"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ProjectMCPHandler struct {
	mcpSessionManager *mcp.SessionManager
	mcpOAuthChecker   MCPOAuthChecker
	acrHelper         *accesscontrolrule.Helper
	serverURL         string
}

func NewProjectMCPHandler(mcpLoader *mcp.SessionManager, acrHelper *accesscontrolrule.Helper, mcpOAuthChecker MCPOAuthChecker, serverURL string) *ProjectMCPHandler {
	return &ProjectMCPHandler{
		mcpSessionManager: mcpLoader,
		mcpOAuthChecker:   mcpOAuthChecker,
		acrHelper:         acrHelper,
		serverURL:         serverURL,
	}
}

func convertProjectMCPServer(projectServer *v1.ProjectMCPServer, mcpServer *v1.MCPServer) types.ProjectMCPServer {
	return types.ProjectMCPServer{
		Metadata:                 MetadataFrom(projectServer),
		ProjectMCPServerManifest: projectServer.Spec.Manifest,
		Name:                     mcpServer.Spec.Manifest.Name,
		Description:              mcpServer.Spec.Manifest.Description,
		Icon:                     mcpServer.Spec.Manifest.Icon,
		UserID:                   projectServer.Spec.UserID,
	}
}

func getMCPServerForProjectServer(ctx context.Context, client kclient.Client, projectServer v1.ProjectMCPServer) (*v1.MCPServer, error) {
	var mcpServerName string
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		var mcpServerInstance v1.MCPServerInstance
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: projectServer.Namespace, Name: projectServer.Spec.Manifest.MCPID}, &mcpServerInstance); err != nil {
			return nil, fmt.Errorf("failed to get MCP server instance %q: %w", projectServer.Spec.Manifest.MCPID, err)
		}
		mcpServerName = mcpServerInstance.Spec.MCPServerName
	} else {
		mcpServerName = projectServer.Spec.Manifest.MCPID
	}

	var mcpServer v1.MCPServer
	if err := client.Get(ctx, kclient.ObjectKey{Namespace: projectServer.Namespace, Name: mcpServerName}, &mcpServer); err != nil {
		return nil, fmt.Errorf("failed to get MCP server %q: %w", mcpServerName, err)
	}

	return &mcpServer, nil
}

func (p *ProjectMCPHandler) ListServer(req api.Context) error {
	project, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var servers v1.ProjectMCPServerList
	if err = req.List(&servers, kclient.MatchingFields{
		"spec.threadName": project.Name,
	}); err != nil {
		return nil
	}

	credCtxs := make([]string, 0, len(servers.Items))

	for _, server := range servers.Items {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", project.Name, server.Name))
		if project.IsSharedProject() {
			// Add default credentials shared by the agent for this MCP server if available.
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s-shared", server.Spec.ThreadName, server.Name))
		}
	}

	creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: credCtxs,
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	credMap := make(map[string]map[string]string, len(creds))
	for _, cred := range creds {
		if _, ok := credMap[cred.ToolName]; !ok {
			c, err := req.GPTClient.RevealCredential(req.Context(), []string{cred.Context}, cred.ToolName)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to find credential: %w", err)
			}
			credMap[cred.ToolName] = c.Env
		}
	}

	var (
		mcpServer *v1.MCPServer

		items = make([]types.ProjectMCPServer, 0, len(servers.Items))
	)
	for _, server := range servers.Items {
		mcpServer, err = getMCPServerForProjectServer(req.Context(), req.Storage, server)
		if err != nil {
			return err
		}

		items = append(items, convertProjectMCPServer(&server, mcpServer))
	}

	return req.Write(types.ProjectMCPServerList{Items: items})
}

func (p *ProjectMCPHandler) CreateServer(req api.Context) error {
	var input types.ProjectMCPServerManifest
	if err := req.Read(&input); err != nil {
		return err
	}

	t, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	projectServer := v1.ProjectMCPServer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ProjectMCPServerPrefix,
			Namespace:    req.Namespace(),
			Finalizers:   []string{v1.ProjectMCPServerFinalizer},
		},
		Spec: v1.ProjectMCPServerSpec{
			Manifest:   input,
			ThreadName: t.Name,
			UserID:     req.User.GetUID(),
		},
	}

	mcpServer, err := getMCPServerForProjectServer(req.Context(), req.Storage, projectServer)
	if err != nil {
		return err
	}

	if !req.UserIsAdmin() && mcpServer.Spec.UserID != req.User.GetUID() {
		return types.NewErrNotFound("MCP server %s is not found", mcpServer.Name)
	}

	if err = req.Create(&projectServer); err != nil {
		return err
	}

	return req.WriteCreated(convertProjectMCPServer(&projectServer, mcpServer))
}

func (p *ProjectMCPHandler) GetServer(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	if err := req.Get(&projectServer, req.PathValue("project_mcp_server_id")); err != nil {
		return err
	}

	mcpServer, err := getMCPServerForProjectServer(req.Context(), req.Storage, projectServer)
	if err != nil {
		return err
	}

	return req.Write(convertProjectMCPServer(&projectServer, mcpServer))
}

func (p *ProjectMCPHandler) DeleteServer(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	if err := req.Get(&projectServer, req.PathValue("project_mcp_server_id")); err != nil {
		return err
	}

	mcpServer, err := getMCPServerForProjectServer(req.Context(), req.Storage, projectServer)
	if err != nil {
		return err
	}

	if err = req.Delete(&projectServer); err != nil {
		return err
	}

	return req.Write(convertProjectMCPServer(&projectServer, mcpServer))
}

func (p *ProjectMCPHandler) CheckOAuth(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	err := req.Get(&projectServer, req.PathValue("project_mcp_server_id"))
	if err != nil {
		return err
	}

	var (
		server       v1.MCPServer
		serverConfig mcp.ServerConfig
	)
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		server, serverConfig, err = ServerFromMCPServerInstance(req, projectServer.Spec.Manifest.MCPID)
	} else {
		server, serverConfig, err = ServerForActionWithID(req, projectServer.Spec.Manifest.MCPID)
	}
	if err != nil {
		return err
	}

	if server.Spec.Manifest.URL != "" {
		var are nmcp.AuthRequiredErr
		if _, err = p.mcpSessionManager.PingServer(req.Context(), req.User.GetUID(), server, serverConfig); err != nil {
			if !errors.As(err, &are) {
				return fmt.Errorf("failed to ping MCP server: %w", err)
			}
			req.WriteHeader(http.StatusPreconditionFailed)
		}
	}

	return nil
}

func (p *ProjectMCPHandler) GetOAuthURL(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	err := req.Get(&projectServer, req.PathValue("project_mcp_server_id"))
	if err != nil {
		return err
	}

	var (
		server       v1.MCPServer
		serverConfig mcp.ServerConfig
	)
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		server, serverConfig, err = ServerFromMCPServerInstance(req, projectServer.Spec.Manifest.MCPID)
	} else {
		server, serverConfig, err = ServerForActionWithID(req, projectServer.Spec.Manifest.MCPID)
	}
	if err != nil {
		return err
	}

	var u string
	if server.Spec.Manifest.URL != "" {
		u, err = p.mcpOAuthChecker.CheckForMCPAuth(req.Context(), server, serverConfig, req.User.GetUID(), server.Name, "")
		if err != nil {
			return fmt.Errorf("failed to get OAuth URL: %w", err)
		}
	}

	return req.Write(map[string]string{"oauthURL": u})
}

func (p *ProjectMCPHandler) GetTools(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	err := req.Get(&projectServer, req.PathValue("project_mcp_server_id"))
	if err != nil {
		return err
	}

	var (
		server       v1.MCPServer
		serverConfig mcp.ServerConfig
	)
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		server, serverConfig, err = ServerFromMCPServerInstance(req, projectServer.Spec.Manifest.MCPID)
	} else {
		server, serverConfig, err = ServerForActionWithID(req, projectServer.Spec.Manifest.MCPID)
	}
	if err != nil {
		return err
	}

	caps, err := p.mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		return err
	}

	if caps.Tools == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support tools")
	}

	var allowedTools []string
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	thread, err = projects.GetFirst(req.Context(), req.Storage, thread, func(project *v1.Thread) (bool, error) {
		return project.Spec.Manifest.AllowedMCPTools[projectServer.Name] != nil, nil
	})
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	allowedTools = thread.Spec.Manifest.AllowedMCPTools[projectServer.Name]

	tools, err := toolsForServer(req.Context(), p.mcpSessionManager, req.User.GetUID(), server, serverConfig, allowedTools)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	return req.Write(tools)
}

func (p *ProjectMCPHandler) SetTools(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var projectMCPServer v1.ProjectMCPServer
	if err = req.Get(&projectMCPServer, req.PathValue("project_mcp_server_id")); err != nil {
		return err
	}

	var (
		mcpServer v1.MCPServer

		mcpServerName = projectMCPServer.Spec.Manifest.MCPID
	)
	if system.IsMCPServerInstanceID(projectMCPServer.Spec.Manifest.MCPID) {
		var mcpServerInstance v1.MCPServerInstance
		if err = req.Get(&mcpServerInstance, req.PathValue("mcp_server_instance_id")); err != nil {
			return err
		}

		mcpServerName = mcpServerInstance.Spec.MCPServerName
	}

	if err = req.Get(&mcpServer, mcpServerName); err != nil {
		return err
	}

	var tools []string
	if err = req.Read(&tools); err != nil {
		return err
	}

	project, err := getProjectThread(req)
	if err != nil {
		return err
	}

	credCtxs := []string{
		fmt.Sprintf("%s-%s", project.Name, projectMCPServer.Name),
	}
	if project.IsSharedProject() {
		// Add default credentials shared by the agent for this MCP server if available.
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s-shared", projectMCPServer.Spec.ThreadName, projectMCPServer.Name))
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), credCtxs, projectMCPServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}
	serverConfig, missingRequiredNames, err := mcp.ServerToServerConfig(mcpServer, project.Name, cred.Env, tools...)
	if err != nil {
		return fmt.Errorf("failed to get server config: %w", err)
	}

	if len(missingRequiredNames) > 0 {
		return types.NewErrBadRequest("MCP server %s is missing required parameters: %s", projectMCPServer.Name, strings.Join(missingRequiredNames, ", "))
	}

	mcpTools, err := toolsForServer(req.Context(), p.mcpSessionManager, req.User.GetUID(), mcpServer, serverConfig, tools)
	if err != nil {
		return fmt.Errorf("failed to render tools: %w", err)
	}

	if thread.Spec.Manifest.AllowedMCPTools == nil {
		thread.Spec.Manifest.AllowedMCPTools = make(map[string][]string)
	}

	if slices.Contains(tools, "*") {
		thread.Spec.Manifest.AllowedMCPTools[projectMCPServer.Name] = []string{"*"}
	} else {
		for _, t := range tools {
			if !slices.ContainsFunc(mcpTools, func(tool types.MCPServerTool) bool {
				return tool.ID == t
			}) {
				return types.NewErrBadRequest("tool %q is not a recognized tool for MCP server %q", t, projectMCPServer.Name)
			}
		}

		thread.Spec.Manifest.AllowedMCPTools[projectMCPServer.Name] = tools
	}

	if err = req.Update(thread); err != nil {
		return fmt.Errorf("failed to update thread: %w", err)
	}

	return nil
}

func (p *ProjectMCPHandler) GetResources(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	err := req.Get(&projectServer, req.PathValue("project_mcp_server_id"))
	if err != nil {
		return err
	}

	var (
		server       v1.MCPServer
		serverConfig mcp.ServerConfig
	)
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		server, serverConfig, err = ServerFromMCPServerInstance(req, projectServer.Spec.Manifest.MCPID)
	} else {
		server, serverConfig, err = ServerForActionWithID(req, projectServer.Spec.Manifest.MCPID)
	}
	if err != nil {
		return err
	}

	caps, err := p.mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		return err
	}

	if caps.Resources == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
	}

	resources, err := p.mcpSessionManager.ListResources(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		var are nmcp.AuthRequiredErr
		if strings.HasSuffix(err.Error(), "Method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
		} else if errors.As(err, &are) {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to list resources: %w", err)
	}

	return req.Write(resources)
}

func (p *ProjectMCPHandler) ReadResource(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	err := req.Get(&projectServer, req.PathValue("project_mcp_server_id"))
	if err != nil {
		return err
	}

	var (
		server       v1.MCPServer
		serverConfig mcp.ServerConfig
	)
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		server, serverConfig, err = ServerFromMCPServerInstance(req, projectServer.Spec.Manifest.MCPID)
	} else {
		server, serverConfig, err = ServerForActionWithID(req, projectServer.Spec.Manifest.MCPID)
	}
	if err != nil {
		return err
	}

	caps, err := p.mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		return err
	}

	if caps.Resources == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
	}

	contents, err := p.mcpSessionManager.ReadResource(req.Context(), req.User.GetUID(), server, serverConfig, req.PathValue("resource_uri"))
	if err != nil {
		var are nmcp.AuthRequiredErr
		if strings.HasSuffix(err.Error(), "Method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
		} else if errors.As(err, &are) {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to list resources: %w", err)
	}

	return req.Write(contents)
}

func (p *ProjectMCPHandler) GetPrompts(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	err := req.Get(&projectServer, req.PathValue("project_mcp_server_id"))
	if err != nil {
		return err
	}

	var (
		server       v1.MCPServer
		serverConfig mcp.ServerConfig
	)
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		server, serverConfig, err = ServerFromMCPServerInstance(req, projectServer.Spec.Manifest.MCPID)
	} else {
		server, serverConfig, err = ServerForActionWithID(req, projectServer.Spec.Manifest.MCPID)
	}
	if err != nil {
		return err
	}

	caps, err := p.mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		return err
	}

	if caps.Prompts == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
	}

	prompts, err := p.mcpSessionManager.ListPrompts(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		var are nmcp.AuthRequiredErr
		if strings.HasSuffix(err.Error(), "Method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
		} else if errors.As(err, &are) {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to list prompts: %w", err)
	}

	return req.Write(prompts)
}

func (p *ProjectMCPHandler) GetPrompt(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	err := req.Get(&projectServer, req.PathValue("project_mcp_server_id"))
	if err != nil {
		return err
	}

	var (
		server       v1.MCPServer
		serverConfig mcp.ServerConfig
	)
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		server, serverConfig, err = ServerFromMCPServerInstance(req, projectServer.Spec.Manifest.MCPID)
	} else {
		server, serverConfig, err = ServerForActionWithID(req, projectServer.Spec.Manifest.MCPID)
	}
	if err != nil {
		return err
	}

	caps, err := p.mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		return err
	}

	if caps.Prompts == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
	}

	var args map[string]string
	if err = req.Read(&args); err != nil {
		return fmt.Errorf("failed to read args: %w", err)
	}

	messages, description, err := p.mcpSessionManager.GetPrompt(req.Context(), req.User.GetUID(), server, serverConfig, req.PathValue("prompt_name"), args)
	if err != nil {
		var are nmcp.AuthRequiredErr
		if strings.HasSuffix(err.Error(), "Method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
		} else if errors.As(err, &are) {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to get prompt: %w", err)
	}

	return req.Write(map[string]any{
		"messages":    messages,
		"description": description,
	})
}
