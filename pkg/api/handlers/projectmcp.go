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
	"github.com/obot-platform/obot/pkg/jwt/ephemeral"

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
	tokenService      *ephemeral.TokenService
	serverURL         string
}

func NewProjectMCPHandler(mcpLoader *mcp.SessionManager, acrHelper *accesscontrolrule.Helper, tokenService *ephemeral.TokenService, mcpOAuthChecker MCPOAuthChecker, serverURL string) *ProjectMCPHandler {
	return &ProjectMCPHandler{
		mcpSessionManager: mcpLoader,
		mcpOAuthChecker:   mcpOAuthChecker,
		acrHelper:         acrHelper,
		tokenService:      tokenService,
		serverURL:         serverURL,
	}
}

func convertProjectMCPServer(projectServer *v1.ProjectMCPServer, mcpServer *v1.MCPServer, cred map[string]string) types.ProjectMCPServer {
	pmcp := types.ProjectMCPServer{
		Metadata:                 MetadataFrom(projectServer),
		ProjectMCPServerManifest: projectServer.Spec.Manifest,
		Name:                     mcpServer.Spec.Manifest.Name,
		Description:              mcpServer.Spec.Manifest.Description,
		Icon:                     mcpServer.Spec.Manifest.Icon,
		UserID:                   projectServer.Spec.UserID,

		// Default values to show to the user for shared servers:
		Configured:  true,
		NeedsURL:    false,
		NeedsUpdate: false,
	}
	pmcp.Alias = mcpServer.Spec.Alias

	if mcpServer.Spec.MCPCatalogID == "" {
		// For single-user servers, grab more status information from the MCP server.
		// We don't show this for shared servers, because the user can't do anything about it
		// if something is wrong with one of those; only the admin can.
		// We don't care about the connect URL here, so passing empty string for both URL an slug.
		convertedServer := convertMCPServer(*mcpServer, cred, "", "")
		pmcp.Configured = convertedServer.Configured
		pmcp.NeedsURL = convertedServer.NeedsURL
		pmcp.NeedsUpdate = convertedServer.NeedsUpdate
	}

	return pmcp
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

	var (
		mcpServers = make(map[string]v1.MCPServer)
		credCtxs   = make([]string, 0, len(servers.Items))
	)
	for _, server := range servers.Items {
		mcpServer, err := getMCPServerForProjectServer(req.Context(), req.Storage, server)
		if err != nil {
			return err
		}

		if mcpServer != nil {
			mcpServers[server.Name] = *mcpServer

			if mcpServer.Spec.MCPCatalogID == "" {
				credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", mcpServer.Spec.UserID, mcpServer.Name))
			}
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

	var items = make([]types.ProjectMCPServer, 0, len(servers.Items))

	for _, server := range servers.Items {
		mcpServer, ok := mcpServers[server.Name]
		if !ok {
			continue
		}
		cred := credMap[mcpServer.Name]

		items = append(items, convertProjectMCPServer(&server, &mcpServer, cred))
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
		var (
			hasAccess bool
			err       error
		)
		if mcpServer.Spec.MCPCatalogID != "" {
			hasAccess, err = p.acrHelper.UserHasAccessToMCPServerInCatalog(req.User, mcpServer.Name, mcpServer.Spec.MCPCatalogID)
		} else if mcpServer.Spec.PowerUserWorkspaceID != "" {
			hasAccess, err = p.acrHelper.UserHasAccessToMCPServerInWorkspace(req.User, mcpServer.Name, mcpServer.Spec.PowerUserWorkspaceID)
		}

		if err != nil {
			return err
		}
		if !hasAccess {
			return types.NewErrNotFound("MCP server %s is not found", mcpServer.Name)
		}
	}

	var cred map[string]string
	if mcpServer.Spec.MCPCatalogID == "" {
		gptscriptCred, err := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", mcpServer.Spec.UserID, mcpServer.Name)}, mcpServer.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}

		cred = gptscriptCred.Env
	}

	if err = req.Create(&projectServer); err != nil {
		return err
	}

	return req.WriteCreated(convertProjectMCPServer(&projectServer, mcpServer, cred))
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

	var cred map[string]string
	if mcpServer.Spec.MCPCatalogID == "" {
		gptscriptCred, err := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", mcpServer.Spec.UserID, mcpServer.Name)}, mcpServer.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}

		cred = gptscriptCred.Env
	}

	return req.Write(convertProjectMCPServer(&projectServer, mcpServer, cred))
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

	var cred map[string]string
	if mcpServer.Spec.MCPCatalogID == "" {
		gptscriptCred, err := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", mcpServer.Spec.UserID, mcpServer.Name)}, mcpServer.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}

		cred = gptscriptCred.Env
	}

	if err = req.Delete(&projectServer); err != nil {
		return err
	}

	if err := kickThread(req.Context(), req.Storage, req.Namespace(), projectServer.Spec.ThreadName); err != nil {
		log.Warnf("failed to kick thread %s after project MCP server %s was deleted: %v", projectServer.Spec.ThreadName, projectServer.Name, err)
	}

	return req.Write(convertProjectMCPServer(&projectServer, mcpServer, cred))
}

func (p *ProjectMCPHandler) LaunchServer(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	err := req.Get(&projectServer, req.PathValue("project_mcp_server_id"))
	if err != nil {
		return err
	}

	_, server, serverConfig, err := ServerForActionWithConnectID(req, projectServer.Spec.Manifest.MCPID)
	if err != nil {
		return err
	}

	if server.Spec.Manifest.Runtime != types.RuntimeRemote {
		if _, err = p.mcpSessionManager.ListTools(req.Context(), req.User.GetUID(), server, serverConfig); err != nil {
			if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
				return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
			}
			if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
				return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
			}
			if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
				return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
			}
			return fmt.Errorf("failed to launch MCP server: %w", err)
		}
	}

	return nil
}

func (p *ProjectMCPHandler) CheckOAuth(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	err := req.Get(&projectServer, req.PathValue("project_mcp_server_id"))
	if err != nil {
		return err
	}

	_, server, serverConfig, err := ServerForActionWithConnectID(req, projectServer.Spec.Manifest.MCPID)
	if err != nil {
		return err
	}

	if server.Spec.Manifest.Runtime == types.RuntimeRemote {
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

	_, server, serverConfig, err := ServerForActionWithConnectID(req, projectServer.Spec.Manifest.MCPID)
	if err != nil {
		return err
	}

	u, err := p.mcpOAuthChecker.CheckForMCPAuth(req.Context(), server, serverConfig, req.User.GetUID(), server.Name, "")
	if err != nil {
		return fmt.Errorf("failed to get OAuth URL: %w", err)
	}

	return req.Write(map[string]string{"oauthURL": u})
}

func (p *ProjectMCPHandler) GetTools(req api.Context) error {
	var projectServer v1.ProjectMCPServer
	err := req.Get(&projectServer, req.PathValue("project_mcp_server_id"))
	if err != nil {
		return err
	}

	if strings.Replace(projectServer.Spec.ThreadName, system.ThreadPrefix, system.ProjectPrefix, 1) != req.PathValue("project_id") {
		return types.NewErrNotFound("project %s not found", req.PathValue("project_id"))
	}

	mcpServerName := projectServer.Spec.Manifest.MCPID
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		var mcpServerInstance v1.MCPServerInstance
		if err = req.Get(&mcpServerInstance, projectServer.Spec.Manifest.MCPID); err != nil {
			return err
		}

		mcpServerName = mcpServerInstance.Spec.MCPServerName
	}

	var server v1.MCPServer
	if err = req.Get(&server, mcpServerName); err != nil {
		return err
	}

	serverConfig, err := mcp.ProjectServerToConfig(p.tokenService, projectServer, p.serverURL, req.User.GetUID())
	if err != nil {
		return fmt.Errorf("failed to get project server config: %w", err)
	}

	caps, err := p.mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
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
	if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
	}
	if err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		return fmt.Errorf("failed to list tools: %w", err)
	}

	return req.Write(tools)
}

func (p *ProjectMCPHandler) SetTools(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var projectServer v1.ProjectMCPServer
	if err = req.Get(&projectServer, req.PathValue("project_mcp_server_id")); err != nil {
		return err
	}

	if strings.Replace(projectServer.Spec.ThreadName, system.ThreadPrefix, system.ProjectPrefix, 1) != req.PathValue("project_id") {
		return types.NewErrNotFound("project %s not found", req.PathValue("project_id"))
	}

	mcpServerName := projectServer.Spec.Manifest.MCPID
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		var mcpServerInstance v1.MCPServerInstance
		if err = req.Get(&mcpServerInstance, projectServer.Spec.Manifest.MCPID); err != nil {
			return err
		}

		mcpServerName = mcpServerInstance.Spec.MCPServerName
	}

	var server v1.MCPServer
	if err = req.Get(&server, mcpServerName); err != nil {
		return err
	}

	serverConfig, err := mcp.ProjectServerToConfig(p.tokenService, projectServer, p.serverURL, req.User.GetUID())
	if err != nil {
		return fmt.Errorf("failed to get project server config: %w", err)
	}

	caps, err := p.mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		return err
	}

	if caps.Tools == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support tools")
	}

	var tools []string
	if err = req.Read(&tools); err != nil {
		return err
	}

	mcpTools, err := toolsForServer(req.Context(), p.mcpSessionManager, req.User.GetUID(), server, serverConfig, tools)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		return fmt.Errorf("failed to render tools: %w", err)
	}

	if thread.Spec.Manifest.AllowedMCPTools == nil {
		thread.Spec.Manifest.AllowedMCPTools = make(map[string][]string)
	}

	if slices.Contains(tools, "*") {
		thread.Spec.Manifest.AllowedMCPTools[projectServer.Name] = []string{"*"}
	} else {
		for _, t := range tools {
			if !slices.ContainsFunc(mcpTools, func(tool types.MCPServerTool) bool {
				return tool.ID == t
			}) {
				return types.NewErrBadRequest("tool %q is not a recognized tool for MCP server %q", t, projectServer.Name)
			}
		}

		thread.Spec.Manifest.AllowedMCPTools[projectServer.Name] = tools
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

	if strings.Replace(projectServer.Spec.ThreadName, system.ThreadPrefix, system.ProjectPrefix, 1) != req.PathValue("project_id") {
		return types.NewErrNotFound("project %s not found", req.PathValue("project_id"))
	}

	mcpServerName := projectServer.Spec.Manifest.MCPID
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		var mcpServerInstance v1.MCPServerInstance
		if err = req.Get(&mcpServerInstance, projectServer.Spec.Manifest.MCPID); err != nil {
			return err
		}

		mcpServerName = mcpServerInstance.Spec.MCPServerName
	}

	var server v1.MCPServer
	if err = req.Get(&server, mcpServerName); err != nil {
		return err
	}

	serverConfig, err := mcp.ProjectServerToConfig(p.tokenService, projectServer, p.serverURL, req.User.GetUID())
	if err != nil {
		return fmt.Errorf("failed to get project server config: %w", err)
	}

	caps, err := p.mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		return err
	}

	if caps.Resources == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
	}

	resources, err := p.mcpSessionManager.ListResources(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}

		var are nmcp.AuthRequiredErr
		if errors.As(err, &are) {
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

	if strings.Replace(projectServer.Spec.ThreadName, system.ThreadPrefix, system.ProjectPrefix, 1) != req.PathValue("project_id") {
		return types.NewErrNotFound("project %s not found", req.PathValue("project_id"))
	}

	mcpServerName := projectServer.Spec.Manifest.MCPID
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		var mcpServerInstance v1.MCPServerInstance
		if err = req.Get(&mcpServerInstance, projectServer.Spec.Manifest.MCPID); err != nil {
			return err
		}

		mcpServerName = mcpServerInstance.Spec.MCPServerName
	}

	var server v1.MCPServer
	if err = req.Get(&server, mcpServerName); err != nil {
		return err
	}

	serverConfig, err := mcp.ProjectServerToConfig(p.tokenService, projectServer, p.serverURL, req.User.GetUID())
	if err != nil {
		return fmt.Errorf("failed to get project server config: %w", err)
	}

	caps, err := p.mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		return err
	}

	if caps.Resources == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
	}

	contents, err := p.mcpSessionManager.ReadResource(req.Context(), req.User.GetUID(), server, serverConfig, req.PathValue("resource_uri"))
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
		}

		var are nmcp.AuthRequiredErr
		if errors.As(err, &are) {
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

	if strings.Replace(projectServer.Spec.ThreadName, system.ThreadPrefix, system.ProjectPrefix, 1) != req.PathValue("project_id") {
		return types.NewErrNotFound("project %s not found", req.PathValue("project_id"))
	}

	mcpServerName := projectServer.Spec.Manifest.MCPID
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		var mcpServerInstance v1.MCPServerInstance
		if err = req.Get(&mcpServerInstance, projectServer.Spec.Manifest.MCPID); err != nil {
			return err
		}

		mcpServerName = mcpServerInstance.Spec.MCPServerName
	}

	var server v1.MCPServer
	if err = req.Get(&server, mcpServerName); err != nil {
		return err
	}

	serverConfig, err := mcp.ProjectServerToConfig(p.tokenService, projectServer, p.serverURL, req.User.GetUID())
	if err != nil {
		return fmt.Errorf("failed to get project server config: %w", err)
	}

	caps, err := p.mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		return err
	}

	if caps.Prompts == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
	}

	prompts, err := p.mcpSessionManager.ListPrompts(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
		}

		var are nmcp.AuthRequiredErr
		if errors.As(err, &are) {
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

	if strings.Replace(projectServer.Spec.ThreadName, system.ThreadPrefix, system.ProjectPrefix, 1) != req.PathValue("project_id") {
		return types.NewErrNotFound("project %s not found", req.PathValue("project_id"))
	}

	mcpServerName := projectServer.Spec.Manifest.MCPID
	if system.IsMCPServerInstanceID(projectServer.Spec.Manifest.MCPID) {
		var mcpServerInstance v1.MCPServerInstance
		if err = req.Get(&mcpServerInstance, projectServer.Spec.Manifest.MCPID); err != nil {
			return err
		}

		mcpServerName = mcpServerInstance.Spec.MCPServerName
	}

	var server v1.MCPServer
	if err = req.Get(&server, mcpServerName); err != nil {
		return err
	}

	serverConfig, err := mcp.ProjectServerToConfig(p.tokenService, projectServer, p.serverURL, req.User.GetUID())
	if err != nil {
		return fmt.Errorf("failed to get project server config: %w", err)
	}

	caps, err := p.mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
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
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP server is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
		}

		var are nmcp.AuthRequiredErr
		if errors.As(err, &are) {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to get prompt: %w", err)
	}

	return req.Write(map[string]any{
		"messages":    messages,
		"description": description,
	})
}
