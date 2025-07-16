package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/modelcontextprotocol/go-sdk/jsonschema"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/jwt"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/projects"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var envVarRegex = regexp.MustCompile(`\${([^}]+)}`)

// MCPOAuthChecker will check the OAuth status for an MCP server. This interface breaks an import cycle.
type MCPOAuthChecker interface {
	CheckForMCPAuth(ctx context.Context, server v1.MCPServer, config mcp.ServerConfig, mcpID, oauthAppAuthRequestID string) (string, error)
}

type MCPHandler struct {
	mcpSessionManager *mcp.SessionManager
	mcpOAuthChecker   MCPOAuthChecker
	acrHelper         *accesscontrolrule.Helper
	tokenService      *jwt.TokenService
	serverURL         string
}

func NewMCPHandler(tokenService *jwt.TokenService, mcpLoader *mcp.SessionManager, acrHelper *accesscontrolrule.Helper, mcpOAuthChecker MCPOAuthChecker, serverURL string) *MCPHandler {
	return &MCPHandler{
		tokenService:      tokenService,
		mcpSessionManager: mcpLoader,
		mcpOAuthChecker:   mcpOAuthChecker,
		acrHelper:         acrHelper,
		serverURL:         serverURL,
	}
}

func (m *MCPHandler) GetCatalogEntryFromDefaultCatalog(req api.Context) error {
	var (
		entry v1.MCPServerCatalogEntry
		id    = req.PathValue("entry_id")
	)

	if err := req.Get(&entry, id); err != nil {
		return err
	}

	if entry.Spec.MCPCatalogName != system.DefaultCatalog {
		return types.NewErrNotFound("MCP catalog entry not found")
	}

	// Authorization check.
	if !req.UserIsAdmin() {
		hasAccess, err := m.acrHelper.UserHasAccessToMCPServerCatalogEntry(req.User.GetUID(), entry.Name)
		if err != nil {
			return err
		}
		if !hasAccess {
			return types.NewErrForbidden("user is not authorized to access this catalog entry")
		}
	}

	return req.Write(convertMCPServerCatalogEntry(entry))
}

func (m *MCPHandler) ListEntriesInDefaultCatalog(req api.Context) error {
	var list v1.MCPServerCatalogEntryList
	if err := req.List(&list); err != nil {
		return err
	}

	if req.UserIsAdmin() {
		items := make([]types.MCPServerCatalogEntry, 0, len(list.Items))
		for _, entry := range list.Items {
			if entry.Spec.MCPCatalogName == system.DefaultCatalog {
				items = append(items, convertMCPServerCatalogEntry(entry))
			}
		}

		return req.Write(types.MCPServerCatalogEntryList{Items: items})
	}

	var entries []types.MCPServerCatalogEntry
	for _, entry := range list.Items {
		// For default catalog entries, check AccessControlRule authorization
		if entry.Spec.MCPCatalogName == system.DefaultCatalog {
			hasAccess, err := m.acrHelper.UserHasAccessToMCPServerCatalogEntry(req.User.GetUID(), entry.Name)
			if err != nil {
				return err
			}
			if hasAccess {
				entries = append(entries, convertMCPServerCatalogEntry(entry))
			}
		}
	}

	return req.Write(types.MCPServerCatalogEntryList{Items: entries})
}

func convertMCPServerCatalogEntry(entry v1.MCPServerCatalogEntry) types.MCPServerCatalogEntry {
	// Add extracted env vars directly to the entry
	addExtractedEnvVarsToCatalogEntry(&entry)

	return types.MCPServerCatalogEntry{
		Metadata:          MetadataFrom(&entry),
		CommandManifest:   entry.Spec.CommandManifest,
		URLManifest:       entry.Spec.URLManifest,
		ToolReferenceName: entry.Spec.ToolReferenceName,
		Editable:          entry.Spec.Editable,
		CatalogName:       entry.Spec.MCPCatalogName,
		SourceURL:         entry.Spec.SourceURL,
	}
}

func (m *MCPHandler) ListServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")

	var fieldSelector kclient.MatchingFields
	if catalogID != "" {
		fieldSelector = kclient.MatchingFields{
			"spec.sharedWithinMCPCatalogName": catalogID,
		}
	} else if req.PathValue("project_id") != "" {
		t, err := getThreadForScope(req)
		if err != nil {
			return err
		}

		topMost, err := projects.GetRoot(req.Context(), req.Storage, t)
		if err != nil {
			return err
		}

		fieldSelector = kclient.MatchingFields{
			"spec.threadName": topMost.Name,
		}
	} else {
		// List servers scoped to the user.
		fieldSelector = kclient.MatchingFields{
			"spec.userID":     req.User.GetUID(),
			"spec.threadName": "",
		}
	}

	var servers v1.MCPServerList
	if err := req.List(&servers, fieldSelector); err != nil {
		return nil
	}

	credCtxs := make([]string, 0, len(servers.Items))
	if catalogID != "" {
		for _, server := range servers.Items {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", catalogID, server.Name))
		}
	} else if req.PathValue("project_id") != "" {
		project, err := getProjectThread(req)
		if err != nil {
			return err
		}

		for _, server := range servers.Items {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", project.Name, server.Name))
			if project.IsSharedProject() {
				// Add default credentials shared by the agent for this MCP server if available.
				credCtxs = append(credCtxs, fmt.Sprintf("%s-%s-shared", server.Spec.ThreadName, server.Name))
			}
		}
	} else {
		for _, server := range servers.Items {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", req.User.GetUID(), server.Name))
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

	items := make([]types.MCPServer, 0, len(servers.Items))
	for _, server := range servers.Items {
		// Add extracted env vars to the server definition
		addExtractedEnvVars(&server)

		items = append(items, convertMCPServer(server, credMap[server.Name], m.serverURL))
	}

	return req.Write(types.MCPServerList{Items: items})
}

func (m *MCPHandler) GetServer(req api.Context) error {
	var (
		server    v1.MCPServer
		id        = req.PathValue("mcp_server_id")
		catalogID = req.PathValue("catalog_id")
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are not in catalogs, this checks to make sure that no catalogID was provided. (Both are empty strings.)
	if server.Spec.SharedWithinMCPCatalogName != catalogID {
		return types.NewErrNotFound("MCP server not found")
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&server)

	var credCtxs []string
	if catalogID != "" {
		credCtxs = []string{fmt.Sprintf("%s-%s", catalogID, server.Name)}
	} else if req.PathValue("project_id") != "" {
		project, err := getProjectThread(req)
		if err != nil {
			return err
		}

		credCtxs = []string{
			fmt.Sprintf("%s-%s", project.Name, server.Name),
		}
		if project.IsSharedProject() {
			// Add default credentials shared by the agent for this MCP server if available.
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s-shared", server.Spec.ThreadName, server.Name))
		}
	} else {
		credCtxs = []string{fmt.Sprintf("%s-%s", req.User.GetUID(), server.Name)}
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), credCtxs, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	return req.Write(convertMCPServer(server, cred.Env, m.serverURL))
}

func (m *MCPHandler) DeleteServer(req api.Context) error {
	var (
		server    v1.MCPServer
		id        = req.PathValue("mcp_server_id")
		catalogID = req.PathValue("catalog_id")
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are not in catalogs, this checks to make sure that no catalogID was provided. (Both are empty strings.)
	if server.Spec.SharedWithinMCPCatalogName != catalogID {
		return types.NewErrNotFound("MCP server not found")
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&server)

	if req.PathValue("project_id") != "" {
		project, err := getProjectThread(req)
		if err != nil {
			return err
		}

		// Ensure that the MCP server is in the same project as the request before deleting it.
		// This prevents chatbot users from deleting MCP servers from the agent.
		// This is necessary because in order to enable MCP servers to be shared across projects,
		// the standard authz middleware allows access to all MCP server endpoints from any "child" project
		// of the one the MCP server belongs to.
		if project.Name != server.Spec.ThreadName {
			return types.NewErrForbidden("cannot delete MCP server from this project")
		}
	}

	if err := req.Delete(&server); err != nil {
		return err
	}

	return req.Write(convertMCPServer(server, nil, m.serverURL))
}

func (m *MCPHandler) CheckOAuth(req api.Context) error {
	catalogID := req.PathValue("catalog_id")

	server, serverConfig, err := serverForAction(req, m.tokenService)
	if err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are not in catalogs, this checks to make sure that no catalogID was provided. (Both are empty strings.)
	if server.Spec.SharedWithinMCPCatalogName != catalogID {
		return types.NewErrNotFound("MCP server not found")
	}

	if server.Spec.ToolReferenceName != "" || server.Spec.Manifest.Command != "" {
		return nil
	}

	var are nmcp.AuthRequiredErr
	if _, err = m.mcpSessionManager.PingServer(req.Context(), server, serverConfig); err != nil {
		if !errors.As(err, &are) {
			return fmt.Errorf("failed to ping MCP server: %w", err)
		}
		req.WriteHeader(http.StatusPreconditionFailed)
	}

	return nil
}

func (m *MCPHandler) GetOAuthURL(req api.Context) error {
	catalogID := req.PathValue("catalog_id")

	server, serverConfig, err := serverForAction(req, m.tokenService)
	if err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are not in catalogs, this checks to make sure that no catalogID was provided. (Both are empty strings.)
	if server.Spec.SharedWithinMCPCatalogName != catalogID {
		return types.NewErrNotFound("MCP server not found")
	}

	var u string
	if server.Spec.ToolReferenceName == "" && server.Spec.Manifest.URL != "" {
		u, err = m.mcpOAuthChecker.CheckForMCPAuth(req.Context(), server, serverConfig, server.Name, "")
		if err != nil {
			return fmt.Errorf("failed to get OAuth URL: %w", err)
		}
	}

	return req.Write(map[string]string{"oauthURL": u})
}

func (m *MCPHandler) GetTools(req api.Context) error {
	server, serverConfig, caps, err := serverForActionWithCapabilities(req, m.tokenService, m.mcpSessionManager)
	if err != nil {
		return err
	}

	if caps.Tools == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support tools")
	}

	var allowedTools []string
	if server.Spec.ThreadName != "" {
		thread, err := getThreadForScope(req)
		if err != nil {
			return err
		}

		thread, err = projects.GetFirst(req.Context(), req.Storage, thread, func(project *v1.Thread) (bool, error) {
			return project.Spec.Manifest.AllowedMCPTools[server.Name] != nil, nil
		})
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}

		allowedTools = thread.Spec.Manifest.AllowedMCPTools[server.Name]
	}

	tools, err := m.toolsForServer(req.Context(), req.Storage, server, serverConfig, allowedTools)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	return req.Write(tools)
}

func (m *MCPHandler) SetTools(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var mcpServer v1.MCPServer
	if err = req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
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
		fmt.Sprintf("%s-%s", project.Name, mcpServer.Name),
	}
	if project.IsSharedProject() {
		// Add default credentials shared by the agent for this MCP server if available.
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s-shared", mcpServer.Spec.ThreadName, mcpServer.Name))
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), credCtxs, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}
	serverConfig, missingRequiredNames, err := mcp.ToServerConfig(m.tokenService, mcpServer, m.serverURL, project.Name, cred.Env, tools...)
	if err != nil {
		return fmt.Errorf("failed to get server config: %w", err)
	}

	if len(missingRequiredNames) > 0 {
		return types.NewErrBadRequest("MCP server %s is missing required parameters: %s", mcpServer.Name, strings.Join(missingRequiredNames, ", "))
	}

	mcpTools, err := m.toolsForServer(req.Context(), req.Storage, mcpServer, serverConfig, tools)
	if err != nil {
		return fmt.Errorf("failed to render tools: %w", err)
	}

	if thread.Spec.Manifest.AllowedMCPTools == nil {
		thread.Spec.Manifest.AllowedMCPTools = make(map[string][]string)
	}

	if slices.Contains(tools, "*") {
		thread.Spec.Manifest.AllowedMCPTools[mcpServer.Name] = []string{"*"}
	} else {
		for _, t := range tools {
			if !slices.ContainsFunc(mcpTools, func(tool types.MCPServerTool) bool {
				return tool.ID == t
			}) {
				return types.NewErrBadRequest("tool %q is not a recognized tool for MCP server %q", t, mcpServer.Name)
			}
		}

		thread.Spec.Manifest.AllowedMCPTools[mcpServer.Name] = tools
	}

	if err = req.Update(thread); err != nil {
		return fmt.Errorf("failed to update thread: %w", err)
	}

	return nil
}

func (m *MCPHandler) GetResources(req api.Context) error {
	mcpServer, serverConfig, caps, err := serverForActionWithCapabilities(req, m.tokenService, m.mcpSessionManager)
	if err != nil {
		return err
	}

	if caps.Resources == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
	}

	resources, err := m.mcpSessionManager.ListResources(req.Context(), mcpServer, serverConfig)
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

func (m *MCPHandler) ReadResource(req api.Context) error {
	mcpServer, serverConfig, caps, err := serverForActionWithCapabilities(req, m.tokenService, m.mcpSessionManager)
	if err != nil {
		return err
	}

	if caps.Resources == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
	}

	contents, err := m.mcpSessionManager.ReadResource(req.Context(), mcpServer, serverConfig, req.PathValue("resource_uri"))
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

func (m *MCPHandler) GetPrompts(req api.Context) error {
	mcpServer, serverConfig, caps, err := serverForActionWithCapabilities(req, m.tokenService, m.mcpSessionManager)
	if err != nil {
		return err
	}

	if caps.Prompts == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
	}

	prompts, err := m.mcpSessionManager.ListPrompts(req.Context(), mcpServer, serverConfig)
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

func (m *MCPHandler) GetPrompt(req api.Context) error {
	mcpServer, serverConfig, caps, err := serverForActionWithCapabilities(req, m.tokenService, m.mcpSessionManager)
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

	messages, description, err := m.mcpSessionManager.GetPrompt(req.Context(), mcpServer, serverConfig, req.PathValue("prompt_name"), args)
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

func ServerFromMCPServerInstance(req api.Context, tokenService *jwt.TokenService, instanceID string) (v1.MCPServer, mcp.ServerConfig, error) {
	var (
		server   v1.MCPServer
		instance v1.MCPServerInstance
	)
	if err := req.Get(&instance, instanceID); err != nil {
		return server, mcp.ServerConfig{}, err
	}

	if err := req.Get(&server, instance.Spec.MCPServerName); err != nil {
		return server, mcp.ServerConfig{}, err
	}

	if server.Spec.ToolReferenceName != "" && server.Spec.Manifest.Command == "" && server.Spec.Manifest.URL == "" {
		// Legacy tool bundle. Nothing else to do.
		return server, mcp.ServerConfig{}, nil
	}

	if server.Spec.NeedsURL {
		return server, mcp.ServerConfig{}, fmt.Errorf("mcp server %s needs to update its URL", server.Name)
	}

	addExtractedEnvVars(&server)

	var credCtx, scope string
	if server.Spec.SharedWithinMCPCatalogName != "" {
		credCtx = fmt.Sprintf("%s-%s", server.Spec.SharedWithinMCPCatalogName, server.Name)
		scope = server.Spec.SharedWithinMCPCatalogName
	} else {
		credCtx = fmt.Sprintf("%s-%s", instance.Spec.UserID, server.Name)
		scope = instance.Spec.UserID
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return server, mcp.ServerConfig{}, fmt.Errorf("failed to find credential: %w", err)
	}

	serverConfig, missingConfig, err := mcp.ToServerConfig(tokenService, server, strings.TrimSuffix(req.APIBaseURL, "/api"), scope, cred.Env)
	if err != nil {
		return server, mcp.ServerConfig{}, err
	}

	if len(missingConfig) > 0 {
		return server, mcp.ServerConfig{}, types.NewErrBadRequest("missing required config: %s", strings.Join(missingConfig, ", "))
	}

	return server, serverConfig, nil
}

func ServerForActionWithID(req api.Context, tokenService *jwt.TokenService, id string) (v1.MCPServer, mcp.ServerConfig, error) {
	var server v1.MCPServer
	if err := req.Get(&server, id); err != nil {
		return server, mcp.ServerConfig{}, err
	}

	if server.Spec.NeedsURL {
		return server, mcp.ServerConfig{}, types.NewErrBadRequest("mcp server %s needs to update its URL", server.Name)
	}

	var (
		credCtxs []string
		scope    string
	)
	if server.Spec.SharedWithinMCPCatalogName != "" {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.SharedWithinMCPCatalogName, server.Name))
		scope = server.Spec.SharedWithinMCPCatalogName
	} else if server.Spec.ThreadName != "" {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.ThreadName, server.Name))

		if req.PathValue("project_id") != "" {
			project, err := getProjectThread(req)
			if err != nil {
				return server, mcp.ServerConfig{}, err
			}

			if project.IsSharedProject() {
				credCtxs = append(credCtxs, fmt.Sprintf("%s-%s-shared", server.Spec.ThreadName, server.Name))
			}
		}

		scope = server.Spec.ThreadName
	} else {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name))
		scope = server.Spec.UserID
	}

	if server.Spec.ToolReferenceName != "" && server.Spec.Manifest.Command == "" && server.Spec.Manifest.URL == "" {
		// Legacy tool bundle. Nothing else to do.
		return server, mcp.ServerConfig{}, nil
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&server)

	cred, err := req.GPTClient.RevealCredential(req.Context(), credCtxs, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return server, mcp.ServerConfig{}, fmt.Errorf("failed to find credential: %w", err)
	}

	serverConfig, missingConfig, err := mcp.ToServerConfig(tokenService, server, strings.TrimSuffix(req.APIBaseURL, "/api"), scope, cred.Env)
	if err != nil {
		return server, mcp.ServerConfig{}, err
	}

	if len(missingConfig) > 0 {
		return server, mcp.ServerConfig{}, types.NewErrBadRequest("missing required config: %s", strings.Join(missingConfig, ", "))
	}

	return server, serverConfig, nil
}

func serverForAction(req api.Context, tokenService *jwt.TokenService) (v1.MCPServer, mcp.ServerConfig, error) {
	return ServerForActionWithID(req, tokenService, req.PathValue("mcp_server_id"))
}

func serverForActionWithCapabilities(req api.Context, tokenService *jwt.TokenService, mcpSessionManager *mcp.SessionManager) (v1.MCPServer, mcp.ServerConfig, nmcp.ServerCapabilities, error) {
	server, serverConfig, err := serverForAction(req, tokenService)
	if err != nil {
		return server, serverConfig, nmcp.ServerCapabilities{}, err
	}

	caps, err := mcpSessionManager.ServerCapabilities(req.Context(), server, serverConfig)
	return server, serverConfig, caps, err
}

func serverManifestFromCatalogEntryManifest(isAdmin bool, entry types.MCPServerCatalogEntryManifest, input types.MCPServerManifest) (types.MCPServerManifest, error) {
	result := types.MCPServerManifest{
		Name:        entry.Name,
		Description: entry.Description,
		Icon:        entry.Icon,
		Metadata:    maps.Clone(entry.Metadata),
		Env:         entry.Env,
		Command:     entry.Command,
		Args:        entry.Args,
		Headers:     entry.Headers,
	}

	// If the user is an admin, they can override anything from the catalog entry.
	if isAdmin {
		result = mergeMCPServerManifests(result, input)
	}

	if entry.FixedURL != "" {
		result.URL = entry.FixedURL
	} else if entry.Hostname != "" {
		if input.URL == "" {
			return types.MCPServerManifest{}, types.NewErrBadRequest("the server must use a specific URL that matches the hostname %q", entry.Hostname)
		}

		u, err := url.Parse(input.URL)
		if err != nil {
			return types.MCPServerManifest{}, fmt.Errorf("failed to parse URL %q: %w", input.URL, err)
		}

		if u.Hostname() != entry.Hostname {
			return types.MCPServerManifest{}, types.NewErrBadRequest("the server must use a specific URL that matches the hostname %q", entry.Hostname)
		}

		result.URL = input.URL
	}

	return result, nil
}

func mergeMCPServerManifests(existing, override types.MCPServerManifest) types.MCPServerManifest {
	if override.Name != "" {
		existing.Name = override.Name
	}
	if override.Description != "" {
		existing.Description = override.Description
	}
	if override.Icon != "" {
		existing.Icon = override.Icon
	}
	if len(override.Env) > 0 {
		existing.Env = override.Env
	}
	if override.Command != "" {
		existing.Command = override.Command
	}
	if len(override.Args) > 0 {
		existing.Args = override.Args
	}
	if len(override.Headers) > 0 {
		existing.Headers = override.Headers
	}

	return existing
}

func (m *MCPHandler) CreateServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	projectID := req.PathValue("project_id")

	var input types.MCPServer
	if err := req.Read(&input); err != nil {
		return err
	}

	server := v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.MCPServerPrefix,
			Namespace:    req.Namespace(),
			Finalizers:   []string{v1.MCPServerFinalizer},
		},
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: input.CatalogEntryID,
			UserID:                    req.User.GetUID(),
		},
	}

	if catalogID != "" {
		var catalog v1.MCPCatalog
		if err := req.Get(&catalog, catalogID); err != nil {
			return err
		}

		server.Spec.SharedWithinMCPCatalogName = catalogID
	} else if projectID != "" {
		t, err := getThreadForScope(req)
		if err != nil {
			return err
		}

		server.Spec.ThreadName = t.Name
	}

	if input.CatalogEntryID != "" {
		if !req.UserIsAdmin() {
			hasAccess, err := m.acrHelper.UserHasAccessToMCPServerCatalogEntry(req.User.GetUID(), input.CatalogEntryID)
			if err != nil {
				return err
			}

			if !hasAccess {
				return types.NewErrForbidden("user does not have access to MCP server catalog entry")
			}
		}

		var catalogEntry v1.MCPServerCatalogEntry
		if err := req.Get(&catalogEntry, input.CatalogEntryID); err != nil {
			return err
		}

		var (
			manifest types.MCPServerManifest
			err      error
		)
		if catalogEntry.Spec.CommandManifest.Command != "" {
			manifest, err = serverManifestFromCatalogEntryManifest(req.UserIsAdmin(), catalogEntry.Spec.CommandManifest, input.MCPServerManifest)
		} else {
			manifest, err = serverManifestFromCatalogEntryManifest(req.UserIsAdmin(), catalogEntry.Spec.URLManifest, input.MCPServerManifest)
		}
		if err != nil {
			return err
		}

		server.Spec.Manifest = manifest
		server.Spec.ToolReferenceName = catalogEntry.Spec.ToolReferenceName
		server.Spec.UnsupportedTools = catalogEntry.Spec.UnsupportedTools
	} else if req.UserIsAdmin() || projectID != "" {
		// If the user is an admin, they can create a server with a manifest that is not in the catalog.
		// Additionally, creating a server in a project for a user is OK, too.
		server.Spec.Manifest = input.MCPServerManifest
	} else {
		return types.NewErrBadRequest("catalogEntryID is required")
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&server)

	if err := req.Create(&server); err != nil {
		return err
	}

	var (
		cred gptscript.Credential
		err  error
	)
	if catalogID != "" {
		cred, err = req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", catalogID, server.Name)}, server.Name)
	} else if projectID != "" {
		cred, err = req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", server.Spec.ThreadName, server.Name)}, server.Name)
	} else {
		cred, err = req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", req.User.GetUID(), server.Name)}, server.Name)
	}
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	return req.WriteCreated(convertMCPServer(server, cred.Env, m.serverURL))
}

func (m *MCPHandler) UpdateServer(req api.Context) error {
	var (
		id        = req.PathValue("mcp_server_id")
		catalogID = req.PathValue("catalog_id")
		projectID = req.PathValue("project_id")
		err       error
		project   *v1.Thread
		updated   types.MCPServerManifest
		existing  v1.MCPServer
	)

	if err := req.Get(&existing, id); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are not in catalogs, this checks to make sure that no catalogID was provided. (Both are empty strings.)
	if existing.Spec.SharedWithinMCPCatalogName != catalogID {
		return types.NewErrNotFound("MCP server not found")
	}

	if projectID != "" {
		project, err = getProjectThread(req)
		if err != nil {
			return err
		}

		// Ensure that the MCP server being updated is in the project referenced by the request.
		// This prevents chatbot users from editing MCP servers in the agent.
		// This is necessary because in order to enable MCP servers to be shared across projects,
		// the standard authz middleware allows access to all MCP server endpoints from any "child" project
		// of the one the MCP server belongs to.
		if project.Name != existing.Spec.ThreadName {
			return types.NewErrForbidden("cannot edit MCP server from this project")
		}
	}

	if err := req.Read(&updated); err != nil {
		return err
	}

	// Shutdown any server that is using the default credentials.
	var cred gptscript.Credential
	if catalogID != "" {
		cred, err = req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", catalogID, existing.Name)}, existing.Name)
	} else if projectID != "" {
		cred, err = req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", existing.Spec.ThreadName, existing.Name)}, existing.Name)
	} else {
		cred, err = req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", req.User.GetUID(), existing.Name)}, existing.Name)
	}
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	// Shutdown the server, even if there is no credential
	if catalogID != "" {
		err = m.removeMCPServer(req.Context(), existing, catalogID, cred.Env)
	} else if projectID != "" {
		err = m.removeMCPServer(req.Context(), existing, project.Name, cred.Env)
	} else {
		err = m.removeMCPServer(req.Context(), existing, req.User.GetUID(), cred.Env)
	}
	if err != nil {
		return err
	}

	// Shutdown the MCP server using any shared credentials.
	if projectID != "" {
		sharedCred, err := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s-shared", existing.Spec.ThreadName, existing.Name)}, existing.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}

		var chatBots v1.ThreadList
		if err = req.List(&chatBots, &kclient.ListOptions{
			Namespace: project.Namespace,
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.parentThreadName": project.Name,
				"spec.project":          "true",
			}),
		}); err != nil {
			return fmt.Errorf("failed to list child projects: %w", err)
		}

		// Shutdown all chatbot MCP servers.
		for _, chatBot := range chatBots.Items {
			childCred, err := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", chatBot.Name, existing.Name)}, existing.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to find credential: %w", err)
			} else if err != nil {
				// Use the shared parent credential if we didn't find the chatbot's credential.
				childCred = sharedCred
			}

			// Shutdown the server, even if there is no credential
			if err = m.removeMCPServer(req.Context(), existing, chatBot.Name, childCred.Env); err != nil {
				return err
			}
		}
	}

	existing.Spec.Manifest = updated

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&existing)

	if err = req.Update(&existing); err != nil {
		return err
	}

	return req.Write(convertMCPServer(existing, cred.Env, m.serverURL))
}

func (m *MCPHandler) ConfigureServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	projectID := req.PathValue("project_id")

	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are not in catalogs, this checks to make sure that no catalogID was provided. (Both are empty strings.)
	if mcpServer.Spec.SharedWithinMCPCatalogName != catalogID {
		return types.NewErrNotFound("MCP server not found")
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&mcpServer)

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	var credCtx, scope string
	if catalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", catalogID, mcpServer.Name)
		scope = catalogID
	} else if projectID != "" {
		project, err := getProjectThread(req)
		if err != nil {
			return err
		}

		credCtx = fmt.Sprintf("%s-%s", project.Name, mcpServer.Name)
		scope = project.Name
	} else {
		credCtx = fmt.Sprintf("%s-%s", req.User.GetUID(), mcpServer.Name)
		scope = req.User.GetUID()
	}

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	if err := m.removeMCPServerAndCred(req.Context(), req.GPTClient, mcpServer, scope, []string{credCtx}); err != nil {
		return err
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
		Context:  credCtx,
		ToolName: mcpServer.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	return req.Write(convertMCPServer(mcpServer, envVars, m.serverURL))
}

func (m *MCPHandler) ConfigureSharedServer(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&mcpServer)

	project, err := getProjectThread(req)
	if err != nil {
		return err
	}

	if project.Name != mcpServer.Spec.ThreadName {
		return types.NewErrForbidden("cannot edit shared MCP server from this project")
	}

	var envVars map[string]string
	if err = req.Read(&envVars); err != nil {
		return err
	}

	var chatBots v1.ThreadList
	if err = req.List(&chatBots, &kclient.ListOptions{
		Namespace: project.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.parentThreadName": project.Name,
			"spec.project":          "true",
		}),
	}); err != nil {
		return fmt.Errorf("failed to list child projects: %w", err)
	}

	credCtx := fmt.Sprintf("%s-%s-shared", mcpServer.Spec.ThreadName, mcpServer.Name)
	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	// Remove the MCP server for all chatbots using this credential.
	for _, chatBot := range chatBots.Items {
		if err = m.removeMCPServer(req.Context(), mcpServer, chatBot.Name, cred.Env); err != nil {
			return err
		}
	}

	// Remove the top-level MCP server if it exists and remove the credential.
	if err = m.removeMCPServerAndCred(req.Context(), req.GPTClient, mcpServer, project.Name, []string{credCtx}); err != nil {
		return err
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err = req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
		Context:  credCtx,
		ToolName: mcpServer.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	return req.Write(convertMCPServer(mcpServer, envVars, m.serverURL))
}

func (m *MCPHandler) DeconfigureServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	projectID := req.PathValue("project_id")

	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are not in catalogs, this checks to make sure that no catalogID was provided. (Both are empty strings.)
	if mcpServer.Spec.SharedWithinMCPCatalogName != catalogID {
		return types.NewErrNotFound("MCP server not found")
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&mcpServer)

	var credCtx, scope string
	if catalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", catalogID, mcpServer.Name)
		scope = catalogID
	} else if projectID != "" {
		project, err := getProjectThread(req)
		if err != nil {
			return err
		}

		credCtx = fmt.Sprintf("%s-%s", project.Name, mcpServer.Name)
		scope = project.Name
	} else {
		credCtx = fmt.Sprintf("%s-%s", req.User.GetUID(), mcpServer.Name)
		scope = req.User.GetUID()
	}

	if err := m.removeMCPServerAndCred(req.Context(), req.GPTClient, mcpServer, scope, []string{credCtx}); err != nil {
		return err
	}

	return req.Write(convertMCPServer(mcpServer, nil, m.serverURL))
}

func (m *MCPHandler) DeconfigureSharedServer(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&mcpServer)

	project, err := getProjectThread(req)
	if err != nil {
		return err
	}

	if project.Name != mcpServer.Spec.ThreadName {
		return types.NewErrForbidden("cannot edit shared MCP server from this project")
	}

	var chatBots v1.ThreadList
	if err = req.List(&chatBots, &kclient.ListOptions{
		Namespace: project.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.parentThreadName": project.Name,
			"spec.project":          "true",
		}),
	}); err != nil {
		return fmt.Errorf("failed to list child projects: %w", err)
	}

	credCtx := []string{fmt.Sprintf("%s-%s-shared", mcpServer.Spec.ThreadName, mcpServer.Name)}

	cred, err := req.GPTClient.RevealCredential(req.Context(), credCtx, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	for _, chatBot := range chatBots.Items {
		if err = m.removeMCPServer(req.Context(), mcpServer, chatBot.Name, cred.Env); err != nil {
			return err
		}
	}

	// Remove the top-level MCP server if it exists and remove the credential.
	if err = m.removeMCPServerAndCred(req.Context(), req.GPTClient, mcpServer, project.Name, credCtx); err != nil {
		return err
	}

	return req.Write(convertMCPServer(mcpServer, nil, m.serverURL))
}

func (m *MCPHandler) Reveal(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	projectID := req.PathValue("project_id")

	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are not in catalogs, this checks to make sure that no catalogID was provided. (Both are empty strings.)
	if mcpServer.Spec.SharedWithinMCPCatalogName != catalogID {
		return types.NewErrNotFound("MCP server not found")
	}

	var credCtx string
	if catalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", catalogID, mcpServer.Name)
	} else if projectID != "" {
		project, err := getProjectThread(req)
		if err != nil {
			return err
		}

		credCtx = fmt.Sprintf("%s-%s", project.Name, mcpServer.Name)
	} else {
		credCtx = fmt.Sprintf("%s-%s", req.User.GetUID(), mcpServer.Name)
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", mcpServer.Name)
}

func (m *MCPHandler) RevealSharedServer(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s-shared", mcpServer.Spec.ThreadName, mcpServer.Name)}, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", mcpServer.Name)
}

func (m *MCPHandler) toolsForServer(ctx context.Context, client kclient.Client, server v1.MCPServer, serverConfig mcp.ServerConfig, allowedTools []string) ([]types.MCPServerTool, error) {
	allTools := allowedTools == nil || slices.Contains(allowedTools, "*")
	if server.Spec.ToolReferenceName != "" {
		var toolReferences v1.ToolReferenceList
		if err := client.List(ctx, &toolReferences, kclient.MatchingFields{
			"spec.bundleToolName": server.Spec.ToolReferenceName,
		}); err != nil {
			return nil, err
		}

		tools := make([]types.MCPServerTool, 0, len(toolReferences.Items))
		for _, ref := range toolReferences.Items {
			if ref.Status.Tool != nil {
				tools = append(tools, types.MCPServerTool{
					ID:          ref.Name,
					Name:        ref.Status.Tool.Name,
					Description: ref.Status.Tool.Description,
					Params:      ref.Status.Tool.Params,
					Credentials: ref.Status.Tool.Credentials,
					Enabled:     allTools || slices.Contains(allowedTools, ref.Name),
				})
			}
		}

		return tools, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	gTools, err := m.mcpSessionManager.ListTools(ctx, server, serverConfig)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, nil
		}
		var are nmcp.AuthRequiredErr
		if strings.HasSuffix(err.Error(), "Method not found") {
			return nil, types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support tools")
		} else if errors.As(err, &are) {
			return nil, types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return nil, err
	}

	tools := make([]types.MCPServerTool, 0, len(gTools))
	for _, t := range gTools {
		mcpTool := types.MCPServerTool{
			ID:          t.Name,
			Name:        t.Name,
			Description: t.Description,
			Enabled:     allTools && !slices.Contains(server.Spec.UnsupportedTools, t.Name) || slices.Contains(allowedTools, t.Name),
			Unsupported: slices.Contains(server.Spec.UnsupportedTools, t.Name),
		}

		if len(t.InputSchema) > 0 {
			var schema jsonschema.Schema

			schemaData, err := json.Marshal(t.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal input schema for tool %s: %w", t.Name, err)
			}

			if err := json.Unmarshal(schemaData, &schema); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tool input schema: %w", err)
			}

			mcpTool.Params = make(map[string]string, len(schema.Properties))
			for name, param := range schema.Properties {
				if param != nil {
					mcpTool.Params[name] = param.Description
				}
			}
		}

		tools = append(tools, mcpTool)
	}

	return tools, nil
}

func (m *MCPHandler) removeMCPServer(ctx context.Context, mcpServer v1.MCPServer, scope string, credEnv map[string]string) error {
	serverConfig, _, err := mcp.ToServerConfig(m.tokenService, mcpServer, m.serverURL, scope, credEnv)
	if err != nil {
		return err
	}

	if err = m.mcpSessionManager.ShutdownServer(ctx, serverConfig); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	return nil
}

func (m *MCPHandler) removeMCPServerAndCred(ctx context.Context, gptClient *gptscript.GPTScript, mcpServer v1.MCPServer, scope string, credCtx []string) error {
	cred, err := gptClient.RevealCredential(ctx, credCtx, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	// Shutdown the server, even if there is no credential
	if err := m.removeMCPServer(ctx, mcpServer, scope, cred.Env); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	// If revealing the credential was successful, remove it.
	if err == nil {
		if err = gptClient.DeleteCredential(ctx, cred.Context, mcpServer.Name); err != nil {
			return fmt.Errorf("failed to remove existing credential: %w", err)
		}
	}

	return nil
}

func extractEnvVars(text string) []string {
	if text == "" {
		return nil
	}

	matches := envVarRegex.FindAllStringSubmatch(text, -1)

	vars := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			vars = append(vars, match[1])
		}
	}

	return vars
}

// addExtractedEnvVars extracts and adds environment variables to the server definition
func addExtractedEnvVars(server *v1.MCPServer) {
	// Keep track of existing env vars in the spec to avoid duplicates
	existing := make(map[string]struct{})
	for _, env := range server.Spec.Manifest.Env {
		existing[env.Key] = struct{}{}
	}

	// Extract variables from command
	extracted := make(map[string]struct{})
	for _, v := range extractEnvVars(server.Spec.Manifest.Command) {
		extracted[v] = struct{}{}
	}

	// Extract variables from args
	for _, arg := range server.Spec.Manifest.Args {
		for _, v := range extractEnvVars(arg) {
			extracted[v] = struct{}{}
		}
	}

	// Extract variables from URL
	for _, v := range extractEnvVars(server.Spec.Manifest.URL) {
		extracted[v] = struct{}{}
	}

	// Add any new vars to the server's Env list
	for v := range extracted {
		if _, exists := existing[v]; !exists {
			server.Spec.Manifest.Env = append(server.Spec.Manifest.Env, types.MCPEnv{
				MCPHeader: types.MCPHeader{
					Name:        v,
					Key:         v,
					Description: "Automatically detected variable",
					Sensitive:   true,
					Required:    true,
				},
			})
		}
	}
}

// addExtractedEnvVarsToCatalogEntry extracts and adds environment variables to both manifests in the catalog entry
func addExtractedEnvVarsToCatalogEntry(entry *v1.MCPServerCatalogEntry) {
	// Extract and add env vars to Command Manifest
	if entry.Spec.CommandManifest.Command != "" {
		// Keep track of existing env vars in the command manifest to avoid duplicates
		existingCmd := make(map[string]struct{})
		for _, env := range entry.Spec.CommandManifest.Env {
			existingCmd[env.Key] = struct{}{}
		}

		// Extract variables from command
		extractedCmd := make(map[string]struct{})
		for _, v := range extractEnvVars(entry.Spec.CommandManifest.Command) {
			extractedCmd[v] = struct{}{}
		}

		// Extract variables from args
		for _, arg := range entry.Spec.CommandManifest.Args {
			for _, v := range extractEnvVars(arg) {
				extractedCmd[v] = struct{}{}
			}
		}

		// Add any new vars to the Command Manifest's Env list
		for v := range extractedCmd {
			if _, exists := existingCmd[v]; !exists {
				entry.Spec.CommandManifest.Env = append(entry.Spec.CommandManifest.Env, types.MCPEnv{
					MCPHeader: types.MCPHeader{
						Name:        v,
						Key:         v,
						Description: "Automatically detected variable",
						Sensitive:   true,
						Required:    true,
					},
				})
			}
		}
	}

	// Extract and add env vars to URL Manifest
	if entry.Spec.URLManifest.FixedURL != "" {
		// Keep track of existing env vars in the URL manifest to avoid duplicates
		existingURL := make(map[string]struct{})
		for _, env := range entry.Spec.URLManifest.Env {
			existingURL[env.Key] = struct{}{}
		}

		// Extract variables from URL
		extractedURL := make(map[string]struct{})
		for _, v := range extractEnvVars(entry.Spec.URLManifest.FixedURL) {
			extractedURL[v] = struct{}{}
		}

		// Add any new vars to the URL Manifest's Env list
		for v := range extractedURL {
			if _, exists := existingURL[v]; !exists {
				entry.Spec.URLManifest.Env = append(entry.Spec.URLManifest.Env, types.MCPEnv{
					MCPHeader: types.MCPHeader{
						Name:        v,
						Key:         v,
						Description: "Automatically detected variable",
						Sensitive:   true,
						Required:    true,
					},
				})
			}
		}
	}
}

func convertMCPServer(server v1.MCPServer, credEnv map[string]string, serverURL string) types.MCPServer {
	var missingEnvVars, missingHeaders []string

	// Check for missing required env vars
	for _, env := range server.Spec.Manifest.Env {
		if !env.Required {
			continue
		}

		if _, ok := credEnv[env.Key]; !ok {
			missingEnvVars = append(missingEnvVars, env.Key)
		}
	}

	// Check for missing required headers
	for _, header := range server.Spec.Manifest.Headers {
		if !header.Required {
			continue
		}

		if _, ok := credEnv[header.Key]; !ok {
			missingHeaders = append(missingHeaders, header.Key)
		}
	}

	var connectURL string
	// Only non-shared servers get a connect URL.
	// Shared servers have connect URLs on the MCPServerInstances instead.
	if server.Spec.SharedWithinMCPCatalogName == "" {
		connectURL = fmt.Sprintf("%s/mcp-connect/%s", serverURL, server.Name)
	}

	return types.MCPServer{
		Metadata:                MetadataFrom(&server),
		MissingRequiredEnvVars:  missingEnvVars,
		MissingRequiredHeaders:  missingHeaders,
		Configured:              len(missingEnvVars) == 0 && len(missingHeaders) == 0,
		MCPServerManifest:       server.Spec.Manifest,
		CatalogEntryID:          server.Spec.MCPServerCatalogEntryName,
		SharedWithinCatalogName: server.Spec.SharedWithinMCPCatalogName,
		ConnectURL:              connectURL,
		NeedsUpdate:             server.Status.NeedsUpdate,
		NeedsURL:                server.Spec.NeedsURL,
		PreviousURL:             server.Spec.PreviousURL,
	}
}

func (m *MCPHandler) ListServersInDefaultCatalog(req api.Context) error {
	var list v1.MCPServerList
	if err := req.List(&list, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
		"spec.sharedWithinMCPCatalogName": system.DefaultCatalog,
	}); err != nil {
		return err
	}

	var allowedServers []v1.MCPServer
	if req.UserIsAdmin() {
		allowedServers = list.Items
	} else {
		for _, server := range list.Items {
			hasAccess, err := m.acrHelper.UserHasAccessToMCPServer(req.User.GetUID(), server.Name)
			if err != nil {
				return err
			}
			if hasAccess {
				allowedServers = append(allowedServers, server)
			}
		}
	}

	var credCtxs []string
	for _, server := range allowedServers {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.SharedWithinMCPCatalogName, server.Name))
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

	// Load catalog entries to enrich servers with tool previews
	var catalogEntries v1.MCPServerCatalogEntryList
	if err := req.List(&catalogEntries); err != nil {
		// Don't fail if we can't load catalog entries, just continue without previews
		log.Errorf("failed to load catalog entries: %v", err)
	}

	catalogEntryMap := make(map[string]v1.MCPServerCatalogEntry, len(catalogEntries.Items))
	for _, entry := range catalogEntries.Items {
		catalogEntryMap[entry.Name] = entry
	}

	var mcpServers []types.MCPServer
	for _, server := range allowedServers {
		addExtractedEnvVars(&server)
		// Enrich with tool preview data if catalog entry exists
		if server.Spec.MCPServerCatalogEntryName != "" {
			if entry, exists := catalogEntryMap[server.Spec.MCPServerCatalogEntryName]; exists {
				// Add tool preview from catalog entry to server manifest
				if entry.Spec.CommandManifest.ToolPreview != nil {
					server.Spec.Manifest.ToolPreview = entry.Spec.CommandManifest.ToolPreview
				} else if entry.Spec.URLManifest.ToolPreview != nil {
					server.Spec.Manifest.ToolPreview = entry.Spec.URLManifest.ToolPreview
				}
			}
		}

		mcpServers = append(mcpServers, convertMCPServer(server, credMap[server.Name], m.serverURL))
	}

	return req.Write(types.MCPServerList{Items: mcpServers})
}

func (m *MCPHandler) GetServerFromDefaultCatalog(req api.Context) error {
	var (
		server v1.MCPServer
		id     = req.PathValue("mcp_server_id")
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	if server.Spec.SharedWithinMCPCatalogName != system.DefaultCatalog {
		return types.NewErrNotFound("MCP server not found")
	}

	// Authorization check.
	if !req.UserIsAdmin() {
		hasAccess, err := m.acrHelper.UserHasAccessToMCPServer(req.User.GetUID(), server.Name)
		if err != nil {
			return err
		}
		if !hasAccess {
			return types.NewErrForbidden("user is not authorized to access this MCP server")
		}
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", server.Spec.SharedWithinMCPCatalogName, server.Name)}, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	addExtractedEnvVars(&server)

	// Enrich with tool preview data if catalog entry exists
	if server.Spec.MCPServerCatalogEntryName != "" {
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err == nil {
			// Add tool preview from catalog entry to server manifest
			if entry.Spec.CommandManifest.ToolPreview != nil {
				server.Spec.Manifest.ToolPreview = entry.Spec.CommandManifest.ToolPreview
			} else if entry.Spec.URLManifest.ToolPreview != nil {
				server.Spec.Manifest.ToolPreview = entry.Spec.URLManifest.ToolPreview
			}
		}
		// Don't fail if catalog entry is missing, just continue without preview
	}

	return req.Write(convertMCPServer(server, cred.Env, m.serverURL))
}

func (m *MCPHandler) ClearOAuthCredentials(req api.Context) error {
	catalogID := req.PathValue("catalog_id")

	var server v1.MCPServer
	if err := req.Get(&server, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are not in catalogs, this checks to make sure that no catalogID was provided. (Both are empty strings.)
	if server.Spec.SharedWithinMCPCatalogName != catalogID {
		return types.NewErrNotFound("MCP server not found")
	}
	if err := req.GatewayClient.DeleteMCPOAuthToken(req.Context(), server.Name); err != nil {
		return fmt.Errorf("failed to delete OAuth credentials: %v", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func (m *MCPHandler) GetServerDetails(req api.Context) error {
	if !m.mcpSessionManager.KubernetesEnabled() {
		return types.NewErrNotFound("Kubernetes is not enabled")
	}

	_, serverConfig, err := serverForAction(req, m.tokenService)
	if err != nil {
		return err
	}

	details, err := m.mcpSessionManager.GetServerDetails(req.Context(), serverConfig)
	if err != nil {
		return err
	}

	return req.Write(details)
}

func (m *MCPHandler) StreamServerLogs(req api.Context) error {
	if !m.mcpSessionManager.KubernetesEnabled() {
		return types.NewErrNotFound("Kubernetes is not enabled")
	}

	_, serverConfig, err := serverForAction(req, m.tokenService)
	if err != nil {
		return err
	}

	logs, err := m.mcpSessionManager.StreamServerLogs(req.Context(), serverConfig)
	if err != nil {
		return err
	}

	defer logs.Close()

	// Set up Server-Sent Events headers
	req.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	req.ResponseWriter.Header().Set("Cache-Control", "no-cache")
	req.ResponseWriter.Header().Set("Connection", "keep-alive")

	flusher, shouldFlush := req.ResponseWriter.(http.Flusher)

	// Send initial connection event
	fmt.Fprintf(req.ResponseWriter, "event: connected\ndata: Log stream started\n\n")
	if shouldFlush {
		flusher.Flush()
	}

	// Channel to coordinate between goroutines
	logChan := make(chan string, 100) // Buffered to prevent blocking

	// Start a goroutine to read logs
	go func() {
		defer close(logChan)

		scanner := bufio.NewScanner(logs)
		for scanner.Scan() {
			line := scanner.Text()
			select {
			case <-req.Context().Done():
				return
			case logChan <- line:
			}
		}
		if err := scanner.Err(); err != nil {
			// Send error event
			select {
			case logChan <- fmt.Sprintf("ERROR retrieving logs: %v", err):
			case <-req.Context().Done():
			}
			return
		}
	}()

	// Send log events as they come in
	ticker := time.NewTicker(30 * time.Second) // Keep-alive ping
	defer ticker.Stop()

	for {
		select {
		case <-req.Context().Done():
			fmt.Fprintf(req.ResponseWriter, "event: disconnected\ndata: Client disconnected\n\n")
			if shouldFlush {
				flusher.Flush()
			}
			return nil
		case <-ticker.C:
			// Send keep-alive ping
			fmt.Fprintf(req.ResponseWriter, "event: ping\ndata: keep-alive\n\n")
			if shouldFlush {
				flusher.Flush()
			}
		case logLine, ok := <-logChan:
			if !ok {
				fmt.Fprintf(req.ResponseWriter, "event: ended\ndata: Log stream ended\n\n")
				if shouldFlush {
					flusher.Flush()
				}
				return nil
			}
			fmt.Fprintf(req.ResponseWriter, "event: log\ndata: %s\n\n", logLine)
			if shouldFlush {
				flusher.Flush()
			}
		}
	}
}

func (m *MCPHandler) UpdateURL(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return fmt.Errorf("failed to get server: %w", err)
	}

	if mcpServer.Spec.SharedWithinMCPCatalogName != "" {
		return types.NewErrBadRequest("cannot update the URL for a multi-user MCP server; use the UpdateServer endpoint instead")
	}

	if mcpServer.Spec.MCPServerCatalogEntryName == "" {
		return types.NewErrBadRequest("this server does not have a catalog entry")
	}

	if mcpServer.Spec.Manifest.Command != "" {
		return types.NewErrBadRequest("cannot update the URL for a non-remote MCP server")
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, mcpServer.Spec.MCPServerCatalogEntryName); err != nil {
		return fmt.Errorf("failed to get catalog entry: %w", err)
	}

	if entry.Spec.URLManifest.FixedURL != "" {
		return types.NewErrBadRequest("this server already has a fixed URL that cannot be updated")
	}

	if entry.Spec.URLManifest.Hostname == "" {
		return types.NewErrBadRequest("the catalog entry for this server does not have a hostname")
	}

	var input struct {
		URL string `json:"url"`
	}
	if err := req.Read(&input); err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	parsedURL, err := url.Parse(input.URL)
	if err != nil {
		return types.NewErrBadRequest("failed to parse input URL: %v", err)
	}

	if parsedURL.Hostname() != entry.Spec.URLManifest.Hostname {
		return types.NewErrBadRequest("the hostname in the URL does not match the hostname in the catalog entry")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return types.NewErrBadRequest("the URL must be HTTP or HTTPS")
	}

	mcpServer.Spec.Manifest.URL = input.URL
	mcpServer.Spec.NeedsURL = false
	mcpServer.Spec.PreviousURL = ""

	if err := req.Update(&mcpServer); err != nil {
		return fmt.Errorf("failed to update server: %w", err)
	}

	return req.Write(convertMCPServer(mcpServer, nil, m.serverURL))
}

func (m *MCPHandler) TriggerUpdate(req api.Context) error {
	var server v1.MCPServer
	if err := req.Get(&server, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	if server.Spec.SharedWithinMCPCatalogName != "" {
		return types.NewErrBadRequest("cannot trigger update for a multi-user MCP server; use the UpdateServer endpoint instead")
	}

	if server.Spec.MCPServerCatalogEntryName == "" || !server.Status.NeedsUpdate {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
		return err
	}

	if entry.Spec.CommandManifest.Name != "" {
		server.Spec.Manifest.Metadata = entry.Spec.CommandManifest.Metadata
		server.Spec.Manifest.Name = entry.Spec.CommandManifest.Name
		server.Spec.Manifest.Description = entry.Spec.CommandManifest.Description
		server.Spec.Manifest.Icon = entry.Spec.CommandManifest.Icon
		server.Spec.Manifest.Env = entry.Spec.CommandManifest.Env
		server.Spec.Manifest.Command = entry.Spec.CommandManifest.Command
		server.Spec.Manifest.Args = entry.Spec.CommandManifest.Args

		server.Spec.Manifest.Headers = nil
		server.Spec.Manifest.URL = ""
	} else {
		server.Spec.Manifest.Metadata = entry.Spec.URLManifest.Metadata
		server.Spec.Manifest.Name = entry.Spec.URLManifest.Name
		server.Spec.Manifest.Description = entry.Spec.URLManifest.Description
		server.Spec.Manifest.Icon = entry.Spec.URLManifest.Icon
		server.Spec.Manifest.Headers = entry.Spec.URLManifest.Headers
		server.Spec.Manifest.Env = entry.Spec.URLManifest.Env

		server.Spec.Manifest.Command = ""
		server.Spec.Manifest.Args = nil

		if entry.Spec.URLManifest.FixedURL != "" {
			server.Spec.Manifest.URL = entry.Spec.URLManifest.FixedURL
		} else {
			// Check to see if the server's URL no longer matches the catalog entry's hostname.
			currentURL, err := url.Parse(server.Spec.Manifest.URL)
			if err != nil {
				return err
			}

			server.Spec.NeedsURL = currentURL.Hostname() != entry.Spec.URLManifest.Hostname
			server.Spec.PreviousURL = server.Spec.Manifest.URL
			server.Spec.Manifest.URL = ""
		}
	}

	// Shutdown any server that is using the default credentials.
	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)}, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	// Shutdown the server, even if there is no credential
	if err := m.removeMCPServer(req.Context(), server, server.Spec.UserID, cred.Env); err != nil {
		return err
	}

	if err := req.Update(&server); err != nil {
		return err
	}

	return nil
}
