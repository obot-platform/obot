package handlers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/projects"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var envVarRegex = regexp.MustCompile(`\${([^}]+)}`)

// MCPOAuthChecker will check the OAuth status for an MCP server. This interface breaks an import cycle.
type MCPOAuthChecker interface {
	CheckForMCPAuth(req api.Context, server v1.MCPServer, config mcp.ServerConfig, userID, mcpID, oauthAppAuthRequestID string) (string, error)
}

type MCPHandler struct {
	mcpSessionManager *mcp.SessionManager
	mcpOAuthChecker   MCPOAuthChecker
	acrHelper         *accesscontrolrule.Helper
	serverURL         string
}

func NewMCPHandler(mcpLoader *mcp.SessionManager, acrHelper *accesscontrolrule.Helper, mcpOAuthChecker MCPOAuthChecker, serverURL string) *MCPHandler {
	return &MCPHandler{
		mcpSessionManager: mcpLoader,
		mcpOAuthChecker:   mcpOAuthChecker,
		acrHelper:         acrHelper,
		serverURL:         serverURL,
	}
}

func (m *MCPHandler) GetEntryFromAllSources(req api.Context) error {
	var (
		entry v1.MCPServerCatalogEntry
		id    = req.PathValue("entry_id")
	)

	if err := req.Get(&entry, id); err != nil {
		return err
	}

	// Check if entry is from default catalog or workspace
	if entry.Spec.MCPCatalogName != system.DefaultCatalog && entry.Spec.PowerUserWorkspaceID == "" {
		return types.NewErrNotFound("MCP catalog entry not found")
	}

	// Authorization check.
	var (
		hasAccess bool
		err       error
	)

	if entry.Spec.MCPCatalogName != "" {
		hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(req.User, entry.Name, entry.Spec.MCPCatalogName)
	} else if entry.Spec.PowerUserWorkspaceID != "" {
		hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), req.User, entry.Name, entry.Spec.PowerUserWorkspaceID)
	}
	if err != nil {
		return err
	}
	if !hasAccess {
		return types.NewErrForbidden("user is not authorized to access this catalog entry")
	}

	return req.Write(convertMCPServerCatalogEntry(entry))
}

func (m *MCPHandler) ListEntriesFromAllSources(req api.Context) error {
	var list v1.MCPServerCatalogEntryList
	if err := req.List(&list); err != nil {
		return err
	}

	// Allow admins/auditors to bypass ACR filtering with ?all=true
	if (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true" {
		entries := make([]types.MCPServerCatalogEntry, 0, len(list.Items))
		for _, entry := range list.Items {
			entries = append(entries, convertMCPServerCatalogEntry(entry))
		}
		return req.Write(types.MCPServerCatalogEntryList{Items: entries})
	}

	// Apply ACR filtering for regular users and for admins without ?all=true
	var entries []types.MCPServerCatalogEntry
	for _, entry := range list.Items {
		var (
			err       error
			hasAccess bool
		)

		if entry.Spec.MCPCatalogName != "" {
			hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(req.User, entry.Name, entry.Spec.MCPCatalogName)
		} else if entry.Spec.PowerUserWorkspaceID != "" {
			hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), req.User, entry.Name, entry.Spec.PowerUserWorkspaceID)
		}
		if err != nil {
			return err
		}

		if hasAccess {
			entries = append(entries, convertMCPServerCatalogEntry(entry))
		}
	}

	return req.Write(types.MCPServerCatalogEntryList{Items: entries})
}

func convertMCPServerCatalogEntry(entry v1.MCPServerCatalogEntry) types.MCPServerCatalogEntry {
	return convertMCPServerCatalogEntryWithWorkspace(entry, "", "")
}

func convertMCPServerCatalogEntryWithWorkspace(entry v1.MCPServerCatalogEntry, powerUserWorkspaceID, powerUserID string) types.MCPServerCatalogEntry {
	// Add extracted env vars directly to the entry
	addExtractedEnvVarsToCatalogEntry(&entry)

	return types.MCPServerCatalogEntry{
		Metadata:                  MetadataFrom(&entry),
		Manifest:                  entry.Spec.Manifest,
		Editable:                  entry.Spec.Editable,
		CatalogName:               entry.Spec.MCPCatalogName,
		SourceURL:                 entry.Spec.SourceURL,
		UserCount:                 entry.Status.UserCount,
		LastUpdated:               v1.NewTime(entry.Status.LastUpdated),
		ToolPreviewsLastGenerated: v1.NewTime(entry.Status.ToolPreviewsLastGenerated),
		PowerUserWorkspaceID:      powerUserWorkspaceID,
		PowerUserID:               powerUserID,
	}
}

func (m *MCPHandler) ListServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	var fieldSelector kclient.MatchingFields
	if catalogID != "" {
		fieldSelector = kclient.MatchingFields{
			"spec.mcpCatalogID": catalogID,
		}
	} else if workspaceID != "" {
		fieldSelector = kclient.MatchingFields{
			"spec.powerUserWorkspaceID": workspaceID,
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
	} else if workspaceID != "" {
		for _, server := range servers.Items {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", workspaceID, server.Name))
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

	// Allow admins/auditors to bypass ACR filtering with ?all=true
	bypassACRCheck := (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true"

	for _, server := range servers.Items {
		if server.Spec.Template || server.Spec.CompositeName != "" {
			continue
		}

		var (
			hasAccess bool
			err       error
		)

		if bypassACRCheck {
			// Admins/auditors with ?all=true can see all servers
			hasAccess = true
		} else if server.Spec.UserID == req.User.GetUID() {
			// If the server is owned by the current user, they have access to it
			hasAccess = true
		} else {
			// Apply ACR filtering for regular users and for admins without ?all=true
			if server.Spec.MCPCatalogID != "" {
				hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(req.User, server.Name, server.Spec.MCPCatalogID)
				if err != nil {
					return fmt.Errorf("failed to check access: %w", err)
				}
			} else if server.Spec.PowerUserWorkspaceID != "" {
				hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), req.User, server.Name, server.Spec.PowerUserWorkspaceID)
				if err != nil {
					return fmt.Errorf("failed to check access: %w", err)
				}
			}
		}

		if !hasAccess {
			continue
		}

		// Add extracted env vars to the server definition
		addExtractedEnvVars(&server)

		slug, err := slugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), catalogID, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to determine slug: %w", err)
		}

		items = append(items, convertMCPServer(server, credMap[server.Name], m.serverURL, slug))
	}

	return req.Write(types.MCPServerList{Items: items})
}

func (m *MCPHandler) GetServer(req api.Context) error {
	var (
		server      v1.MCPServer
		id          = req.PathValue("mcp_server_id")
		catalogID   = req.PathValue("catalog_id")
		workspaceID = req.PathValue("workspace_id")
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&server)

	var credCtxs []string
	if catalogID != "" {
		credCtxs = []string{fmt.Sprintf("%s-%s", catalogID, server.Name)}
	} else if workspaceID != "" {
		credCtxs = []string{fmt.Sprintf("%s-%s", workspaceID, server.Name)}
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

	slug, err := slugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(convertMCPServer(server, cred.Env, m.serverURL, slug))
}

func (m *MCPHandler) DeleteServer(req api.Context) error {
	var (
		server      v1.MCPServer
		id          = req.PathValue("mcp_server_id")
		catalogID   = req.PathValue("catalog_id")
		workspaceID = req.PathValue("workspace_id")
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&server)

	slug, err := slugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	if err := req.Delete(&server); err != nil {
		return err
	}

	return req.Write(convertMCPServer(server, nil, m.serverURL, slug))
}

func (m *MCPHandler) LaunchServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	if server.Spec.Manifest.Runtime == types.RuntimeComposite {
		var componentServers v1.MCPServerList
		if err := req.List(&componentServers,
			kclient.InNamespace(server.Namespace),
			kclient.MatchingFields{
				"spec.compositeName": server.Name,
			},
		); err != nil {
			return fmt.Errorf("failed to list child servers: %w", err)
		}

		for _, component := range componentServers.Items {
			if component.Spec.Manifest.Runtime == types.RuntimeRemote {
				continue
			}

			config, err := ServerConfigForAction(req, component)
			if err != nil {
				return fmt.Errorf("failed to get config for component server %s: %w", component.Name, err)
			}

			if _, err = m.mcpSessionManager.ListTools(req.Context(), req.User.GetUID(), component, config); err != nil {
				if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
					return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("Component MCP server %s is not healthy, check configuration for errors", component.Name))
				}
				if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
					return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("No response from component MCP server %s, check configuration for errors", component.Name))
				}
				if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
					return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
				}

				return fmt.Errorf("failed to launch component MCP server %s: %w", component.Name, err)
			}
		}

		return nil
	}

	if server.Spec.Manifest.Runtime != types.RuntimeRemote {
		if _, err = m.mcpSessionManager.ListTools(req.Context(), req.User.GetUID(), server, serverConfig); err != nil {
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

func (m *MCPHandler) CheckOAuth(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	if server.Spec.Manifest.Runtime == types.RuntimeRemote {
		var are nmcp.AuthRequiredErr
		if _, err = m.mcpSessionManager.PingServer(req.Context(), req.User.GetUID(), server, serverConfig); err != nil {
			if !errors.As(err, &are) {
				return fmt.Errorf("failed to ping MCP server: %w", err)
			}
			req.WriteHeader(http.StatusPreconditionFailed)
		}
	}

	return nil
}

func (m *MCPHandler) GetOAuthURL(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	u, err := m.mcpOAuthChecker.CheckForMCPAuth(req, server, serverConfig, req.User.GetUID(), server.Name, "")
	if err != nil {
		return fmt.Errorf("failed to get OAuth URL: %w", err)
	}

	return req.Write(map[string]string{"oauthURL": u})
}

func (m *MCPHandler) GetTools(req api.Context) error {
	server, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager)
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

	tools, err := toolsForServer(req.Context(), m.mcpSessionManager, req.User.GetUID(), server, serverConfig, allowedTools)
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
	serverConfig, missingRequiredNames, err := mcp.ServerToServerConfig(mcpServer, project.Name, cred.Env, tools...)
	if err != nil {
		return fmt.Errorf("failed to get server config: %w", err)
	}

	if len(missingRequiredNames) > 0 {
		return types.NewErrBadRequest("MCP server %s is missing required parameters: %s", mcpServer.Name, strings.Join(missingRequiredNames, ", "))
	}

	mcpTools, err := toolsForServer(req.Context(), m.mcpSessionManager, req.User.GetUID(), mcpServer, serverConfig, tools)
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
	mcpServer, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager)
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

	resources, err := m.mcpSessionManager.ListResources(req.Context(), req.User.GetUID(), mcpServer, serverConfig)
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

func (m *MCPHandler) ReadResource(req api.Context) error {
	mcpServer, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager)
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

	contents, err := m.mcpSessionManager.ReadResource(req.Context(), req.User.GetUID(), mcpServer, serverConfig, req.PathValue("resource_uri"))
	if err != nil {
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

	return req.Write(contents)
}

func (m *MCPHandler) GetPrompts(req api.Context) error {
	mcpServer, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager)
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

	if caps.Prompts == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
	}

	prompts, err := m.mcpSessionManager.ListPrompts(req.Context(), req.User.GetUID(), mcpServer, serverConfig)
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
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}

		var are nmcp.AuthRequiredErr
		if errors.As(err, &are) {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to list prompts: %w", err)
	}

	return req.Write(prompts)
}

func (m *MCPHandler) GetPrompt(req api.Context) error {
	mcpServer, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager)
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

	if caps.Prompts == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
	}

	var args map[string]string
	if err = req.Read(&args); err != nil {
		return fmt.Errorf("failed to read args: %w", err)
	}

	messages, description, err := m.mcpSessionManager.GetPrompt(req.Context(), req.User.GetUID(), mcpServer, serverConfig, req.PathValue("prompt_name"), args)
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
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
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

func mcpServerOrInstanceFromConnectURL(req api.Context, id string) (v1.MCPServer, v1.MCPServerInstance, error) {
	switch {
	case system.IsMCPServerInstanceID(id):
		var instance v1.MCPServerInstance
		return v1.MCPServer{}, instance, req.Get(&instance, id)
	case system.IsMCPServerID(id):
		var server v1.MCPServer
		if err := req.Get(&server, id); err != nil {
			return v1.MCPServer{}, v1.MCPServerInstance{}, err
		}

		if server.Spec.MCPCatalogID != "" || server.Spec.PowerUserWorkspaceID != "" {
			// This is a multi-user MCP server, and user is trying to connect to it.
			// List the MCP server instances, sort by creation time, and take the first one.
			var instances v1.MCPServerInstanceList
			if err := req.List(&instances, &kclient.ListOptions{
				FieldSelector: fields.SelectorFromSet(map[string]string{
					"spec.mcpServerName": id,
					"spec.userID":        req.User.GetUID(),
					"spec.template":      "false",
				}),
			}); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, err
			}
			if len(instances.Items) == 0 {
				// If none exist, then create one for the user.
				instance := v1.MCPServerInstance{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: system.MCPServerInstancePrefix,
						Namespace:    server.Namespace,
					},
					Spec: v1.MCPServerInstanceSpec{
						MCPServerName:             id,
						MCPCatalogName:            server.Spec.MCPCatalogID,
						MCPServerCatalogEntryName: server.Spec.MCPServerCatalogEntryName,
						PowerUserWorkspaceID:      server.Spec.PowerUserWorkspaceID,
						UserID:                    req.User.GetUID(),
					},
				}
				if err := req.Create(&instance); err != nil {
					return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("user has not configured an instance of MCP server %s", id)
				}

				instances.Items = append(instances.Items, instance)
			}

			slices.SortFunc(instances.Items, func(a, b v1.MCPServerInstance) int {
				return a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
			})

			return v1.MCPServer{}, instances.Items[0], nil
		}

		return server, v1.MCPServerInstance{}, nil
	default:
		// In this case, id refers to a catalog entry.
		// Get the catalog entry to make sure it's valid
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&v1.MCPServerCatalogEntry{}, id); err != nil {
			return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("catalog entry %s not found", id)
		}

		// List the MCP servers for the user and take the first one.
		var servers v1.MCPServerList
		if err := req.List(&servers, &kclient.ListOptions{
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.mcpServerCatalogEntryName": id,
				"spec.userID":                    req.User.GetUID(),
				"spec.template":                  "false",
			}),
		}); err != nil {
			return v1.MCPServer{}, v1.MCPServerInstance{}, err
		}
		if len(servers.Items) == 0 {
			// If the user has not configured an MCP server for the catalog entry, and the catalog entry does not have any required configuration, then create an server for the user.
			if entry.Spec.Manifest.Runtime == types.RuntimeComposite {
				// For now launching composite servers by connecting to a catalog entry ID is not supported.
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("user has not configured an MCP server for composite catalog entry %s", id)
			}

			for _, env := range entry.Spec.Manifest.Env {
				if env.Required {
					return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("user has not configured an MCP server for catalog entry %s", id)
				}
			}

			if entry.Spec.Manifest.Runtime == types.RuntimeRemote {
				if entry.Spec.Manifest.RemoteConfig.FixedURL == "" {
					return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("user has not configured an MCP server for catalog entry %s", id)
				}

				for _, h := range entry.Spec.Manifest.RemoteConfig.Headers {
					if h.Required {
						return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("user has not configured an MCP server for catalog entry %s", id)
					}
				}
			}

			// Convert the catalog entry manifest to a server manifest. Treat the user as non-admin always.
			manifest, err := serverManifestFromCatalogEntryManifest(false, entry.Spec.Manifest, types.MCPServerManifest{})
			if err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("user has not configured an MCP server for catalog entry %s", id)
			}

			// Create a new MCP server for the user.
			server := v1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: system.MCPServerPrefix,
					Namespace:    req.Namespace(),
				},
				Spec: v1.MCPServerSpec{
					Manifest:                  manifest,
					UnsupportedTools:          entry.Spec.UnsupportedTools,
					MCPServerCatalogEntryName: id,
					UserID:                    req.User.GetUID(),
				},
			}
			if err := req.Create(&server); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("user has not configured an MCP server for catalog entry %s", id)
			}

			servers.Items = append(servers.Items, server)
		}

		slices.SortFunc(servers.Items, func(a, b v1.MCPServer) int {
			return a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
		})

		return servers.Items[0], v1.MCPServerInstance{}, nil
	}
}

func MCPIDFromConnectURL(req api.Context, id string) (string, error) {
	server, instance, err := mcpServerOrInstanceFromConnectURL(req, id)
	if err != nil {
		return "", err
	}

	switch {
	case instance.Name != "":
		return instance.Name, nil
	case server.Name != "":
		return server.Name, nil
	default:
		return "", fmt.Errorf("unknown MCP server ID %s", id)
	}
}

func ServerForActionWithConnectID(req api.Context, id string) (string, v1.MCPServer, mcp.ServerConfig, error) {
	server, instance, err := mcpServerOrInstanceFromConnectURL(req, id)
	if err != nil {
		return "", v1.MCPServer{}, mcp.ServerConfig{}, err
	}

	switch {
	case instance.Name != "":
		server, config, err := serverFromMCPServerInstance(req, instance)
		return instance.Name, server, config, err
	case server.Name != "":
		config, err := ServerConfigForAction(req, server)
		return server.Name, server, config, err
	default:
		return "", v1.MCPServer{}, mcp.ServerConfig{}, fmt.Errorf("unknown MCP server ID %s", id)
	}
}

func serverFromMCPServerInstance(req api.Context, instance v1.MCPServerInstance) (v1.MCPServer, mcp.ServerConfig, error) {
	var server v1.MCPServer
	if err := req.Get(&server, instance.Spec.MCPServerName); err != nil {
		return server, mcp.ServerConfig{}, err
	}

	if server.Spec.NeedsURL {
		return server, mcp.ServerConfig{}, fmt.Errorf("mcp server %s needs to update its URL", server.Name)
	}

	addExtractedEnvVars(&server)

	var credCtx, scope string
	if server.Spec.MCPCatalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name)
		scope = server.Spec.MCPCatalogID
	} else if server.Spec.PowerUserWorkspaceID != "" {
		credCtx = fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name)
		scope = server.Spec.PowerUserWorkspaceID
	} else {
		credCtx = fmt.Sprintf("%s-%s", instance.Spec.UserID, server.Name)
		scope = instance.Spec.UserID
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return server, mcp.ServerConfig{}, fmt.Errorf("failed to find credential: %w", err)
	}

	serverConfig, missingConfig, err := mcp.ServerToServerConfig(server, scope, cred.Env)
	if err != nil {
		return server, mcp.ServerConfig{}, err
	}

	if len(missingConfig) > 0 {
		return server, mcp.ServerConfig{}, types.NewErrBadRequest("missing required config: %s", strings.Join(missingConfig, ", "))
	}

	return server, serverConfig, nil
}

func ServerForAction(req api.Context, id string) (v1.MCPServer, mcp.ServerConfig, error) {
	var server v1.MCPServer
	if err := req.Get(&server, id); err != nil {
		return server, mcp.ServerConfig{}, err
	}

	serverConfig, err := ServerConfigForAction(req, server)
	return server, serverConfig, err
}

func ServerConfigForAction(req api.Context, server v1.MCPServer) (mcp.ServerConfig, error) {
	if server.Spec.NeedsURL {
		return mcp.ServerConfig{}, types.NewErrBadRequest("mcp server %s needs to update its URL", server.Name)
	}

	var (
		credCtxs []string
		scope    string
	)
	if server.Spec.MCPCatalogID != "" {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name))
		scope = server.Spec.MCPCatalogID
	} else if server.Spec.PowerUserWorkspaceID != "" {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name))
		scope = server.Spec.PowerUserWorkspaceID
	} else if server.Spec.ThreadName != "" {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.ThreadName, server.Name))

		if req.PathValue("project_id") != "" {
			project, err := getProjectThread(req)
			if err != nil {
				return mcp.ServerConfig{}, err
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

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&server)

	cred, err := req.GPTClient.RevealCredential(req.Context(), credCtxs, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return mcp.ServerConfig{}, fmt.Errorf("failed to find credential: %w", err)
	}

	serverConfig, missingConfig, err := mcp.ServerToServerConfig(server, scope, cred.Env)
	if err != nil {
		return mcp.ServerConfig{}, err
	}

	if len(missingConfig) > 0 {
		return mcp.ServerConfig{}, types.NewErrBadRequest("missing required config: %s", strings.Join(missingConfig, ", "))
	}

	return serverConfig, nil
}

func serverForAction(req api.Context) (v1.MCPServer, mcp.ServerConfig, error) {
	return ServerForAction(req, req.PathValue("mcp_server_id"))
}

func serverForActionWithCapabilities(req api.Context, mcpSessionManager *mcp.SessionManager) (v1.MCPServer, mcp.ServerConfig, nmcp.ServerCapabilities, error) {
	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return server, serverConfig, nmcp.ServerCapabilities{}, err
	}

	caps, err := mcpSessionManager.ServerCapabilities(req.Context(), req.User.GetUID(), server, serverConfig)
	return server, serverConfig, caps, err
}

func serverManifestFromCatalogEntryManifest(isAdmin bool, entry types.MCPServerCatalogEntryManifest, input types.MCPServerManifest) (types.MCPServerManifest, error) {
	if entry.Runtime == types.RuntimeComposite && entry.CompositeConfig != nil {
		var result types.MCPServerManifest
		result.Name = entry.Name
		result.Icon = entry.Icon
		result.Description = entry.Description
		result.Metadata = entry.Metadata
		result.Runtime = types.RuntimeComposite
		result.CompositeConfig = &types.CompositeRuntimeConfig{ComponentServers: make([]types.ComponentServer, 0, len(entry.CompositeConfig.ComponentServers))}

		// Build quick lookup from input by CatalogEntryID -> URL
		userURLByID := map[string]string{}
		if input.CompositeConfig != nil {
			for _, cs := range input.CompositeConfig.ComponentServers {
				if cs.CatalogEntryID != "" && cs.Manifest.RemoteConfig != nil && cs.Manifest.RemoteConfig.URL != "" {
					userURLByID[cs.CatalogEntryID] = cs.Manifest.RemoteConfig.URL
				}
			}
		}

		for i, c := range entry.CompositeConfig.ComponentServers {
			userURL := userURLByID[c.CatalogEntryID]
			mapped, err := types.MapCatalogEntryToServer(c.Manifest, userURL)
			if err != nil {
				return types.MCPServerManifest{}, types.RuntimeValidationError{
					Runtime: types.RuntimeComposite,
					Field:   fmt.Sprintf("compositeConfig.componentServers[%d]", i),
					Message: fmt.Sprintf("failed to convert component manifest: %v", err),
				}
			}
			result.CompositeConfig.ComponentServers = append(result.CompositeConfig.ComponentServers, types.ComponentServer{
				CatalogEntryID: c.CatalogEntryID,
				Manifest:       mapped,
				ToolOverrides:  c.ToolOverrides,
				Enabled:        c.Enabled,
			})
		}

		if isAdmin {
			result = mergeMCPServerManifests(result, input)
		}
		return result, nil
	}

	// Non-composite: use the mapping function from types package to convert catalog entry to server manifest
	var userURL string
	if entry.Runtime == types.RuntimeRemote && entry.RemoteConfig != nil && entry.RemoteConfig.Hostname != "" && input.RemoteConfig != nil {
		userURL = input.RemoteConfig.URL
	}

	result, err := types.MapCatalogEntryToServer(entry, userURL)
	if err != nil {
		return types.MCPServerManifest{}, err
	}

	// If the user is an admin, they can override anything from the catalog entry.
	if isAdmin {
		result = mergeMCPServerManifests(result, input)
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
	if override.Runtime != "" {
		existing.Runtime = override.Runtime
	}

	// Merge runtime-specific configurations
	if override.UVXConfig != nil {
		existing.UVXConfig = override.UVXConfig
	}
	if override.NPXConfig != nil {
		existing.NPXConfig = override.NPXConfig
	}
	if override.ContainerizedConfig != nil {
		existing.ContainerizedConfig = override.ContainerizedConfig
	}
	if override.RemoteConfig != nil {
		if existing.RemoteConfig == nil {
			existing.RemoteConfig = override.RemoteConfig
		} else {
			if override.RemoteConfig.URL != "" {
				existing.RemoteConfig.URL = override.RemoteConfig.URL
			}

			if len(override.RemoteConfig.Headers) > 0 {
				existing.RemoteConfig.Headers = override.RemoteConfig.Headers
			}
		}
	}

	return existing
}

func (m *MCPHandler) CreateServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	var input types.MCPServer
	if err := req.Read(&input); err != nil {
		return err
	}

	if input.MCPServerManifest.RemoteConfig != nil && !strings.HasPrefix(input.MCPServerManifest.RemoteConfig.URL, "http") {
		input.MCPServerManifest.RemoteConfig.URL = "https://" + input.MCPServerManifest.RemoteConfig.URL
	}

	// Normalize component URLs for composite input
	if input.MCPServerManifest.CompositeConfig != nil {
		for i := range input.MCPServerManifest.CompositeConfig.ComponentServers {
			rc := input.MCPServerManifest.CompositeConfig.ComponentServers[i].Manifest.RemoteConfig
			if rc != nil && rc.URL != "" && !strings.HasPrefix(rc.URL, "http") {
				rc.URL = "https://" + rc.URL
			}
		}
	}

	server := v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.MCPServerPrefix,
			Namespace:    req.Namespace(),
			Finalizers:   []string{v1.MCPServerFinalizer},
		},
		Spec: v1.MCPServerSpec{
			Alias:                     input.Alias,
			MCPServerCatalogEntryName: input.CatalogEntryID,
			UserID:                    req.User.GetUID(),
		},
	}

	if catalogID != "" {
		var catalog v1.MCPCatalog
		if err := req.Get(&catalog, catalogID); err != nil {
			return err
		}

		server.Spec.MCPCatalogID = catalogID
	} else if workspaceID != "" {
		var workspace v1.PowerUserWorkspace
		if err := req.Get(&workspace, workspaceID); err != nil {
			return err
		}

		server.Spec.PowerUserWorkspaceID = workspaceID
	}

	if input.CatalogEntryID != "" {
		var catalogEntry v1.MCPServerCatalogEntry
		if err := req.Get(&catalogEntry, input.CatalogEntryID); err != nil {
			return err
		}

		var (
			err       error
			hasAccess bool
		)

		if catalogEntry.Spec.MCPCatalogName != "" {
			hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(req.User, catalogEntry.Name, catalogEntry.Spec.MCPCatalogName)
		} else if catalogEntry.Spec.PowerUserWorkspaceID != "" {
			hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), req.User, catalogEntry.Name, catalogEntry.Spec.PowerUserWorkspaceID)
		}
		if err != nil {
			return err
		}

		if !hasAccess {
			return types.NewErrForbidden("user does not have access to MCP server catalog entry")
		}

		// Composite servers are handled by a dedicated helper to create parent + components
		if catalogEntry.Spec.Manifest.Runtime == types.RuntimeComposite {
			// Helper writes the response and returns; finish early
			return m.createCompositeServer(req, server, catalogEntry, input)
		} else {
			manifest, err := serverManifestFromCatalogEntryManifest(req.UserIsAdmin(), catalogEntry.Spec.Manifest, input.MCPServerManifest)
			if err != nil {
				return err
			}
			server.Spec.Manifest = manifest
			server.Spec.UnsupportedTools = catalogEntry.Spec.UnsupportedTools
		}
	} else if req.UserIsAdmin() || workspaceID != "" {
		// If the user is an admin, or if this server is being created in a workspace by a PowerUserPlus,
		// they can create a server with a manifest that is not in the catalog.
		server.Spec.Manifest = input.MCPServerManifest
	} else {
		return types.NewErrBadRequest("catalogEntryID is required")
	}

	if err := validation.ValidateServerManifest(server.Spec.Manifest); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
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
	} else if workspaceID != "" {
		cred, err = req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", workspaceID, server.Name)}, server.Name)
	} else {
		cred, err = req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", req.User.GetUID(), server.Name)}, server.Name)
	}
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	slug, err := slugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.WriteCreated(convertMCPServer(server, cred.Env, m.serverURL, slug))
}

// createCompositeServer creates the parent composite server and all component servers.
// It maps per-component user URLs via serverManifestFromCatalogEntryManifest before creation.
func (m *MCPHandler) createCompositeServer(
	req api.Context,
	compositeServer v1.MCPServer,
	catalogEntry v1.MCPServerCatalogEntry,
	input types.MCPServer,
) error {
	// Build the full composite manifest, propagating user-provided component URLs
	manifest, err := serverManifestFromCatalogEntryManifest(
		req.UserIsAdmin(),
		catalogEntry.Spec.Manifest,
		input.MCPServerManifest,
	)
	if err != nil {
		return err
	}

	compositeServer.Spec.Manifest = manifest
	compositeServer.Spec.UnsupportedTools = catalogEntry.Spec.UnsupportedTools

	// Validate parent composite manifest
	if err := validation.ValidateServerManifest(compositeServer.Spec.Manifest); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	// Extract env vars and create parent
	addExtractedEnvVars(&compositeServer)
	if err := req.Create(&compositeServer); err != nil {
		return err
	}

	// Create component servers
	for i, component := range compositeServer.Spec.Manifest.CompositeConfig.ComponentServers {
		componentServer := v1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.MCPServerPrefix,
				Namespace:    compositeServer.Namespace,
				Finalizers:   []string{v1.MCPServerFinalizer},
			},
			Spec: v1.MCPServerSpec{
				CompositeName:             compositeServer.Name,
				MCPServerCatalogEntryName: component.CatalogEntryID,
				UserID:                    compositeServer.Spec.UserID,
				Manifest:                  component.Manifest,
			},
		}

		if err := validation.ValidateServerManifest(componentServer.Spec.Manifest); err != nil {
			return types.NewErrBadRequest("validation failed for component %d: %v", i, err)
		}

		addExtractedEnvVars(&componentServer)

		if err := req.Create(&componentServer); err != nil {
			return fmt.Errorf("failed to create component server %d: %w", i, err)
		}
	}

	// After creating parent + components, finish the response here
	cred, credErr := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", req.User.GetUID(), compositeServer.Name)}, compositeServer.Name)
	if credErr != nil && !errors.As(credErr, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", credErr)
	}

	slug, err := slugForMCPServer(req.Context(), req.Storage, compositeServer, req.User.GetUID(), "", "")
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.WriteCreated(convertMCPServer(compositeServer, cred.Env, m.serverURL, slug))
}

// UpdateServer updates the manifest of an MCPServer.
// This can only be used by the admin (for things in the default catalog) and PowerUserPlusses, for things in their workspaces.
func (m *MCPHandler) UpdateServer(req api.Context) error {
	var (
		id          = req.PathValue("mcp_server_id")
		catalogID   = req.PathValue("catalog_id")
		workspaceID = req.PathValue("workspace_id")
		err         error
		updated     types.MCPServerManifest
		existing    v1.MCPServer
	)

	if err := req.Get(&existing, id); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if existing.Spec.MCPCatalogID != catalogID || existing.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	if err = req.Read(&updated); err != nil {
		return err
	}

	if updated.RemoteConfig != nil && !strings.HasPrefix(updated.RemoteConfig.URL, "http") {
		updated.RemoteConfig.URL = "https://" + updated.RemoteConfig.URL
	}

	// Shutdown any server that is using the default credentials.
	var cred gptscript.Credential
	if catalogID != "" {
		cred, err = req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", catalogID, existing.Name)}, existing.Name)
	} else if workspaceID != "" {
		cred, err = req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", workspaceID, existing.Name)}, existing.Name)
	} else {
		cred, err = req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", req.User.GetUID(), existing.Name)}, existing.Name)
	}
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	// Shutdown the server, even if there is no credential
	if catalogID != "" {
		err = m.removeMCPServer(req.Context(), existing, catalogID, cred.Env)
	} else if workspaceID != "" {
		err = m.removeMCPServer(req.Context(), existing, workspaceID, cred.Env)
	} else {
		err = m.removeMCPServer(req.Context(), existing, req.User.GetUID(), cred.Env)
	}
	if err != nil {
		return err
	}

	if err := validation.ValidateServerManifest(updated); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	existing.Spec.Manifest = updated

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&existing)

	if err = req.Update(&existing); err != nil {
		return err
	}

	slug, err := slugForMCPServer(req.Context(), req.Storage, existing, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(convertMCPServer(existing, cred.Env, m.serverURL, slug))
}

func (m *MCPHandler) UpdateServerAlias(req api.Context) error {
	var (
		id     = req.PathValue("mcp_server_id")
		server v1.MCPServer
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	if server.Spec.MCPCatalogID != "" {
		return types.NewErrBadRequest("cannot update alias for a multi-user MCP server")
	}

	var input struct {
		Alias string `json:"alias,omitempty"`
	}
	if err := req.Read(&input); err != nil {
		return err
	}

	if input.Alias == server.Spec.Alias {
		// If the alias is the same, skip update.
		return nil
	}
	server.Spec.Alias = input.Alias

	if err := req.Update(&server); err != nil {
		return err
	}

	return nil
}

func (m *MCPHandler) ConfigureServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if mcpServer.Spec.MCPCatalogID != catalogID || mcpServer.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	// Handle composite server configuration differently
	if mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		// Composite servers have nested env vars.
		// The keys are the catalog entry IDs and the values are the env vars for that component server.
		return m.configureCompositeServer(req, mcpServer)
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&mcpServer)

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	// Check if this server is from a catalog and has a URL template that needs to be processed
	if mcpServer.Spec.MCPServerCatalogEntryName != "" {
		var catalogEntry v1.MCPServerCatalogEntry
		if err := req.Get(&catalogEntry, mcpServer.Spec.MCPServerCatalogEntryName); err != nil {
			return fmt.Errorf("failed to get catalog entry %s: %w", mcpServer.Spec.MCPServerCatalogEntryName, err)
		}

		// Check if the catalog entry has a URL template for remote runtime
		// Templates use ${VARIABLE_NAME} syntax for variable substitution
		// Example: "https://${DATABRICKS_WORKSPACE_URL}/api/2.0/mcp/genie/${DATABRICKS_GENIE_SPACE_ID}"
		if catalogEntry.Spec.Manifest.Runtime == types.RuntimeRemote &&
			catalogEntry.Spec.Manifest.RemoteConfig != nil &&
			catalogEntry.Spec.Manifest.RemoteConfig.URLTemplate != "" {
			// Apply the URL template with environment variables
			finalURL, err := applyURLTemplate(catalogEntry.Spec.Manifest.RemoteConfig.URLTemplate, envVars)
			if err != nil {
				return fmt.Errorf("failed to apply URL template: %w", err)
			}

			// Update the server's remote config URL with the processed template
			if mcpServer.Spec.Manifest.RemoteConfig == nil {
				mcpServer.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{}
			}
			mcpServer.Spec.Manifest.RemoteConfig.URL = finalURL

			if err := validation.ValidateServerManifest(mcpServer.Spec.Manifest); err != nil {
				return types.NewErrBadRequest("validation failed: %v", err)
			}

			// Save the updated server
			if err := req.Update(&mcpServer); err != nil {
				return fmt.Errorf("failed to update server with processed URL: %w", err)
			}
		}
	}

	var credCtx, scope string
	if catalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", catalogID, mcpServer.Name)
		scope = catalogID
	} else if workspaceID != "" {
		credCtx = fmt.Sprintf("%s-%s", workspaceID, mcpServer.Name)
		scope = workspaceID
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

	slug, err := slugForMCPServer(req.Context(), req.Storage, mcpServer, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(convertMCPServer(mcpServer, envVars, m.serverURL, slug))
}

func (m *MCPHandler) configureCompositeServer(req api.Context, compositeServer v1.MCPServer) error {
	// Read configuration from request body
	var configRequest struct {
		ComponentConfigs map[string]struct {
			Config  map[string]string `json:"config"`
			URL     string            `json:"url"`
			Enabled *bool             `json:"enabled"`
		} `json:"componentConfigs"`
	}
	if err := req.Read(&configRequest); err != nil {
		return types.NewErrBadRequest("failed to read configuration: %v", err)
	}

	// Query component servers
	var componentServers v1.MCPServerList
	if err := req.List(&componentServers,
		kclient.InNamespace(compositeServer.Namespace),
		kclient.MatchingFields{"spec.compositeName": compositeServer.Name},
	); err != nil {
		return fmt.Errorf("failed to list component servers: %w", err)
	}

	// Load composite catalog entry once to map component manifests
	var compositeEntry v1.MCPServerCatalogEntry
	if err := req.Get(&compositeEntry, compositeServer.Spec.MCPServerCatalogEntryName); err != nil {
		return fmt.Errorf("failed to get composite catalog entry %s: %w", compositeServer.Spec.MCPServerCatalogEntryName, err)
	}

	componentManifestByID := map[string]types.MCPServerCatalogEntryManifest{}
	if compositeEntry.Spec.Manifest.CompositeConfig != nil {
		for _, cs := range compositeEntry.Spec.Manifest.CompositeConfig.ComponentServers {
			componentManifestByID[cs.CatalogEntryID] = cs.Manifest
		}
	}

	// Configure each component with its env vars and persist enabled flags
	// Build quick map for parent composite components by catalog entry ID
	parentComps := map[string]int{}
	if compositeServer.Spec.Manifest.CompositeConfig != nil {
		for i, comp := range compositeServer.Spec.Manifest.CompositeConfig.ComponentServers {
			parentComps[comp.CatalogEntryID] = i
		}
	}

	for _, component := range componentServers.Items {
		addExtractedEnvVars(&component)

		// Calculate credential context for this component (after manifest update)
		var (
			credCtx = fmt.Sprintf("%s-%s", req.User.GetUID(), component.Name)
			scope   = req.User.GetUID()
		)

		// Remove existing credential and shutdown server to allow update
		if err := m.removeMCPServerAndCred(req.Context(), req.GPTClient, component, scope, []string{credCtx}); err != nil {
			return err
		}

		componentConfig, hasConfig := configRequest.ComponentConfigs[component.Spec.MCPServerCatalogEntryName]
		if !hasConfig {
			continue
		}

		// Determine enabled (default true)
		enabled := true
		if componentConfig.Enabled != nil {
			enabled = *componentConfig.Enabled
		}

		// Persist enabled state to parent composite manifest if present
		if idx, ok := parentComps[component.Spec.MCPServerCatalogEntryName]; ok && compositeServer.Spec.Manifest.CompositeConfig != nil {
			compositeServer.Spec.Manifest.CompositeConfig.ComponentServers[idx].Enabled = enabled
		}

		if !enabled {
			// Skip configuring this component
			continue
		}

		// Key by catalog entry ID since the UI doesn't know the generated server names
		manifest, hasManifest := componentManifestByID[component.Spec.MCPServerCatalogEntryName]
		if !hasManifest {
			continue
		}

		// Remote runtime URL handling mirrors non-composite ConfigureServer
		if manifest.Runtime == types.RuntimeRemote &&
			manifest.RemoteConfig != nil &&
			manifest.RemoteConfig.URLTemplate != "" {
			finalURL, err := applyURLTemplate(manifest.RemoteConfig.URLTemplate, componentConfig.Config)
			if err != nil {
				return fmt.Errorf("failed to apply URL template: %w", err)
			}
			if component.Spec.Manifest.RemoteConfig == nil {
				component.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{}
			}
			component.Spec.Manifest.RemoteConfig.URL = finalURL
			if err := validation.ValidateServerManifest(component.Spec.Manifest); err != nil {
				return types.NewErrBadRequest("validation failed: %v", err)
			}
			if err := req.Update(&component); err != nil {
				return fmt.Errorf("failed to update component server %s with processed URL: %w", component.Name, err)
			}
		}

		// After processing all components, persist parent composite with updated enabled flags
		if compositeServer.Spec.Manifest.CompositeConfig != nil {
			if err := req.Update(&compositeServer); err != nil {
				return fmt.Errorf("failed to update composite server enabled flags: %w", err)
			}
		}

		// Remove empty values
		for key, val := range componentConfig.Config {
			if val == "" {
				delete(componentConfig.Config, key)
			}
		}

		// Create new credential
		if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
			Context:  credCtx,
			ToolName: component.Name,
			Type:     gptscript.CredentialTypeTool,
			Env:      componentConfig.Config,
		}); err != nil {
			return fmt.Errorf("failed to create credential for component %s: %w", component.Name, err)
		}
	}

	slug, err := slugForMCPServer(req.Context(), req.Storage, compositeServer, req.User.GetUID(), "", "")
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(convertMCPServer(compositeServer, nil, m.serverURL, slug))
}

// applyURLTemplate applies a URL template with environment variables
// The template uses ${VARIABLE_NAME} syntax for variable substitution
func applyURLTemplate(templateStr string, envVars map[string]string) (string, error) {
	result := templateStr

	// Replace all ${VARIABLE_NAME} patterns with actual values
	for key, value := range envVars {
		placeholder := fmt.Sprintf("${%s}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result, nil
}

func (m *MCPHandler) DeconfigureServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if mcpServer.Spec.MCPCatalogID != catalogID || mcpServer.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	if mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		return m.deconfigureCompositeServer(req, mcpServer)
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&mcpServer)

	var credCtx, scope string
	if catalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", catalogID, mcpServer.Name)
		scope = catalogID
	} else if workspaceID != "" {
		credCtx = fmt.Sprintf("%s-%s", workspaceID, mcpServer.Name)
		scope = workspaceID
	} else {
		credCtx = fmt.Sprintf("%s-%s", req.User.GetUID(), mcpServer.Name)
		scope = req.User.GetUID()
	}

	if err := m.removeMCPServerAndCred(req.Context(), req.GPTClient, mcpServer, scope, []string{credCtx}); err != nil {
		return err
	}

	slug, err := slugForMCPServer(req.Context(), req.Storage, mcpServer, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(convertMCPServer(mcpServer, nil, m.serverURL, slug))
}

func (m *MCPHandler) deconfigureCompositeServer(req api.Context, compositeServer v1.MCPServer) error {
	var componentServers v1.MCPServerList
	if err := req.List(&componentServers,
		kclient.InNamespace(compositeServer.Namespace),
		kclient.MatchingFields{"spec.compositeName": compositeServer.Name},
	); err != nil {
		return fmt.Errorf("failed to list component servers: %w", err)
	}

	for _, component := range componentServers.Items {
		addExtractedEnvVars(&component)
		var (
			credCtx = fmt.Sprintf("%s-%s", req.User.GetUID(), component.Name)
			scope   = req.User.GetUID()
		)
		if err := m.removeMCPServerAndCred(req.Context(), req.GPTClient, component, scope, []string{credCtx}); err != nil {
			return err
		}
	}

	addExtractedEnvVars(&compositeServer)

	var (
		credCtx = fmt.Sprintf("%s-%s", req.User.GetUID(), compositeServer.Name)
		scope   = req.User.GetUID()
	)
	if err := m.removeMCPServerAndCred(req.Context(), req.GPTClient, compositeServer, scope, []string{credCtx}); err != nil {
		return err
	}

	slug, err := slugForMCPServer(req.Context(), req.Storage, compositeServer, scope, "", "")
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(convertMCPServer(compositeServer, nil, m.serverURL, slug))
}

func (m *MCPHandler) Reveal(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if mcpServer.Spec.MCPCatalogID != catalogID || mcpServer.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	// If this is a composite, return per-component configs
	if mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		return m.revealCompositeServer(req, mcpServer)
	}

	var credCtx string
	if catalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", catalogID, mcpServer.Name)
	} else if workspaceID != "" {
		credCtx = fmt.Sprintf("%s-%s", workspaceID, mcpServer.Name)
	} else {
		credCtx = fmt.Sprintf("%s-%s", req.User.GetUID(), mcpServer.Name)
	}

	// Non-composite: return flat env
	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", mcpServer.Name)
}

// revealCompositeServer returns the per-component configuration values (env and URL) for a composite server
func (m *MCPHandler) revealCompositeServer(req api.Context, compositeServer v1.MCPServer) error {
	// List component servers for this composite
	var componentServers v1.MCPServerList
	if err := req.List(&componentServers,
		kclient.InNamespace(compositeServer.Namespace),
		kclient.MatchingFields{"spec.compositeName": compositeServer.Name},
	); err != nil {
		return fmt.Errorf("failed to list component servers: %w", err)
	}

	result := map[string]struct {
		Config map[string]string `json:"config"`
		URL    string            `json:"url"`
		Skip   bool              `json:"skip"`
	}{}

	// For each component, reveal its credential context and URL
	for _, component := range componentServers.Items {
		compCredCtx := fmt.Sprintf("%s-%s", req.User.GetUID(), component.Name)
		compCred, err := req.GPTClient.RevealCredential(req.Context(), []string{compCredCtx}, component.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential for component %s: %w", component.Name, err)
		}

		cfg := map[string]string{}
		if err == nil {
			for k, v := range compCred.Env {
				if v != "" {
					cfg[k] = v
				}
			}
		}

		url := ""
		if component.Spec.Manifest.RemoteConfig != nil {
			url = component.Spec.Manifest.RemoteConfig.URL
		}

		result[component.Spec.MCPServerCatalogEntryName] = struct {
			Config map[string]string `json:"config"`
			URL    string            `json:"url"`
			Skip   bool              `json:"skip"`
		}{Config: cfg, URL: url, Skip: false}
	}

	return req.Write(map[string]any{"componentConfigs": result})
}

func toolsForServer(ctx context.Context, mcpSessionManager *mcp.SessionManager, userID string, server v1.MCPServer, serverConfig mcp.ServerConfig, allowedTools []string) ([]types.MCPServerTool, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	gTools, err := mcpSessionManager.ListTools(ctx, userID, server, serverConfig)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, nil
		}
		var are nmcp.AuthRequiredErr
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return nil, types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support tools")
		} else if errors.As(err, &are) {
			return nil, types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return nil, err
	}

	return mcp.ConvertTools(gTools, allowedTools, server.Spec.UnsupportedTools)
}

func (m *MCPHandler) removeMCPServer(ctx context.Context, mcpServer v1.MCPServer, scope string, credEnv map[string]string) error {
	serverConfig, _, err := mcp.ServerToServerConfig(mcpServer, scope, credEnv)
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

	// Extract variables based on runtime type
	var toExtract []string
	switch server.Spec.Manifest.Runtime {
	case types.RuntimeUVX:
		if server.Spec.Manifest.UVXConfig != nil {
			toExtract = []string{server.Spec.Manifest.UVXConfig.Command}
			if len(server.Spec.Manifest.UVXConfig.Args) > 0 {
				toExtract = append(toExtract, server.Spec.Manifest.UVXConfig.Args...)
			}
		}
	case types.RuntimeNPX:
		if server.Spec.Manifest.NPXConfig != nil && len(server.Spec.Manifest.NPXConfig.Args) > 0 {
			toExtract = append(toExtract, server.Spec.Manifest.NPXConfig.Args...)
		}
	case types.RuntimeContainerized:
		if server.Spec.Manifest.ContainerizedConfig != nil {
			toExtract = []string{server.Spec.Manifest.ContainerizedConfig.Command}
			if len(server.Spec.Manifest.ContainerizedConfig.Args) > 0 {
				toExtract = append(toExtract, server.Spec.Manifest.ContainerizedConfig.Args...)
			}
		}
	case types.RuntimeRemote:
		if server.Spec.Manifest.RemoteConfig != nil {
			toExtract = []string{server.Spec.Manifest.RemoteConfig.URL}
		}
	}

	for _, v := range toExtract {
		for _, env := range extractEnvVars(v) {
			if _, exists := existing[env]; !exists {
				server.Spec.Manifest.Env = append(server.Spec.Manifest.Env, types.MCPEnv{
					MCPHeader: types.MCPHeader{
						Name:        env,
						Key:         env,
						Description: "Automatically detected variable",
						Sensitive:   true,
						Required:    true,
					},
				})
			}
		}
	}
}

// addExtractedEnvVarsToCatalogEntry extracts and adds environment variables to the catalog entry manifest
func addExtractedEnvVarsToCatalogEntry(entry *v1.MCPServerCatalogEntry) {
	// Keep track of existing env vars in the manifest to avoid duplicates
	existing := make(map[string]struct{})
	for _, env := range entry.Spec.Manifest.Env {
		existing[env.Key] = struct{}{}
	}

	// Extract variables based on runtime type
	var toExtract []string

	switch entry.Spec.Manifest.Runtime {
	case types.RuntimeUVX:
		if entry.Spec.Manifest.UVXConfig != nil {
			toExtract = append(toExtract, entry.Spec.Manifest.UVXConfig.Command)
			if len(entry.Spec.Manifest.UVXConfig.Args) > 0 {
				toExtract = append(toExtract, entry.Spec.Manifest.UVXConfig.Args...)
			}
		}
	case types.RuntimeNPX:
		if entry.Spec.Manifest.NPXConfig != nil && len(entry.Spec.Manifest.NPXConfig.Args) > 0 {
			toExtract = append(toExtract, entry.Spec.Manifest.NPXConfig.Args...)
		}
	case types.RuntimeContainerized:
		if entry.Spec.Manifest.ContainerizedConfig != nil {
			toExtract = append(toExtract, entry.Spec.Manifest.ContainerizedConfig.Command)
			if len(entry.Spec.Manifest.ContainerizedConfig.Args) > 0 {
				toExtract = append(toExtract, entry.Spec.Manifest.ContainerizedConfig.Args...)
			}
		}
	case types.RuntimeRemote:
		if entry.Spec.Manifest.RemoteConfig != nil {
			// Add the existing headers to the existing map.
			for _, header := range entry.Spec.Manifest.RemoteConfig.Headers {
				existing[header.Key] = struct{}{}
			}

			toExtract = append(toExtract, entry.Spec.Manifest.RemoteConfig.URLTemplate)
		}
	}

	for _, v := range toExtract {
		for _, env := range extractEnvVars(v) {
			if _, exists := existing[env]; !exists {
				if entry.Spec.Manifest.Runtime != types.RuntimeRemote {
					entry.Spec.Manifest.Env = append(entry.Spec.Manifest.Env, types.MCPEnv{
						MCPHeader: types.MCPHeader{
							Name:        env,
							Key:         env,
							Description: "Automatically detected variable",
							Sensitive:   true,
							Required:    true,
						},
					})
				} else if entry.Spec.Manifest.RemoteConfig != nil {
					entry.Spec.Manifest.RemoteConfig.Headers = append(entry.Spec.Manifest.RemoteConfig.Headers, types.MCPHeader{
						Name:        env,
						Key:         env,
						Description: "Automatically detected variable",
						Sensitive:   false,
						Required:    true,
					})
				}
			}
		}
	}
}

func convertMCPServer(server v1.MCPServer, credEnv map[string]string, serverURL, slug string) types.MCPServer {
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

	// Check for missing required headers (only for remote runtime)
	if server.Spec.Manifest.Runtime == types.RuntimeRemote && server.Spec.Manifest.RemoteConfig != nil {
		for _, header := range server.Spec.Manifest.RemoteConfig.Headers {
			if !header.Required {
				continue
			}

			if _, ok := credEnv[header.Key]; !ok {
				missingHeaders = append(missingHeaders, header.Key)
			}
		}
	}

	var connectURL string
	// Only single-user servers get a connect URL.
	// Multi-user servers have connect URLs on the MCPServerInstances instead.
	if server.Spec.MCPCatalogID == "" {
		connectURL = fmt.Sprintf("%s/mcp-connect/%s", serverURL, slug)
	}

	conditions := make([]types.DeploymentCondition, 0, len(server.Status.DeploymentConditions))
	for _, cond := range server.Status.DeploymentConditions {
		conditions = append(conditions, types.DeploymentCondition{
			Type:               string(cond.Type),
			Status:             string(cond.Status),
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastTransitionTime: *types.NewTime(cond.LastTransitionTime.Time),
			LastUpdateTime:     *types.NewTime(cond.LastUpdateTime.Time),
		})
	}

	return types.MCPServer{
		Metadata:                    MetadataFrom(&server),
		Alias:                       server.Spec.Alias,
		MissingRequiredEnvVars:      missingEnvVars,
		MissingRequiredHeaders:      missingHeaders,
		UserID:                      server.Spec.UserID,
		Configured:                  len(missingEnvVars) == 0 && len(missingHeaders) == 0 && !server.Spec.NeedsURL,
		MCPServerManifest:           server.Spec.Manifest,
		CatalogEntryID:              server.Spec.MCPServerCatalogEntryName,
		PowerUserWorkspaceID:        server.Spec.PowerUserWorkspaceID,
		MCPCatalogID:                server.Spec.MCPCatalogID,
		ConnectURL:                  connectURL,
		NeedsUpdate:                 server.Status.NeedsUpdate,
		NeedsURL:                    server.Spec.NeedsURL,
		PreviousURL:                 server.Spec.PreviousURL,
		MCPServerInstanceUserCount:  server.Status.MCPServerInstanceUserCount,
		DeploymentStatus:            server.Status.DeploymentStatus,
		DeploymentAvailableReplicas: server.Status.DeploymentAvailableReplicas,
		DeploymentReadyReplicas:     server.Status.DeploymentReadyReplicas,
		DeploymentReplicas:          server.Status.DeploymentReplicas,
		DeploymentConditions:        conditions,
		Template:                    server.Spec.Template,
	}
}

func slugForMCPServer(ctx context.Context, client kclient.Client, server v1.MCPServer, userID, catalogID, workspaceID string) (string, error) {
	var shouldHaveUnique bool
	if workspaceID == "" && catalogID == "" && server.Spec.MCPServerCatalogEntryName != "" {
		var serversWithEntryName v1.MCPServerList
		if err := client.List(ctx, &serversWithEntryName, &kclient.ListOptions{
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.mcpServerCatalogEntryName": server.Spec.MCPServerCatalogEntryName,
				"spec.userID":                    userID,
				"spec.template":                  "false",
			}),
		}); err != nil {
			return "", fmt.Errorf("failed to find MCP server catalog entry for server: %w", err)
		}

		slices.SortFunc(serversWithEntryName.Items, func(a, b v1.MCPServer) int {
			return a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
		})

		shouldHaveUnique = len(serversWithEntryName.Items) != 0 && serversWithEntryName.Items[0].Name != server.Name
	}

	slug := server.Spec.MCPServerCatalogEntryName
	if shouldHaveUnique || server.Spec.MCPServerCatalogEntryName == "" || server.Spec.Manifest.Runtime == types.RuntimeComposite {
		slug = server.Name
	}

	return slug, nil
}

func (m *MCPHandler) ListServersFromAllSources(req api.Context) error {
	var list v1.MCPServerList
	if err := req.List(&list, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}

	var allowedServers []v1.MCPServer

	// Allow admins/auditors to bypass ACR filtering with ?all=true
	if (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true" {
		allowedServers = list.Items
	} else {
		// Apply ACR filtering for regular users and for admins without ?all=true
		for _, server := range list.Items {
			var (
				err       error
				hasAccess bool
			)

			if server.Spec.MCPCatalogID != "" {
				// Check default catalog servers
				hasAccess, err = m.acrHelper.UserHasAccessToMCPServerInCatalog(req.User, server.Name, server.Spec.MCPCatalogID)
			} else if server.Spec.PowerUserWorkspaceID != "" {
				// Check workspace-scoped servers
				hasAccess, err = m.acrHelper.UserHasAccessToMCPServerInWorkspace(req.User, server.Name, server.Spec.PowerUserWorkspaceID, server.Spec.UserID)
			}
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
		if server.Spec.MCPCatalogID != "" {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name))
		} else if server.Spec.PowerUserWorkspaceID != "" {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name))
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

	mcpServers := make([]types.MCPServer, 0, len(allowedServers))

	var slug string
	for _, server := range allowedServers {
		addExtractedEnvVars(&server)
		// Enrich with tool preview data if catalog entry exists
		if server.Spec.MCPServerCatalogEntryName != "" {
			entry := catalogEntryMap[server.Spec.MCPServerCatalogEntryName]
			// Add tool preview from catalog entry to server manifest
			server.Spec.Manifest.ToolPreview = entry.Spec.Manifest.ToolPreview
		}

		slug, err = slugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), system.DefaultCatalog, server.Spec.PowerUserWorkspaceID)
		if err != nil {
			return fmt.Errorf("failed to generate slug: %w", err)
		}

		mcpServers = append(mcpServers, convertMCPServer(server, credMap[server.Name], m.serverURL, slug))
	}

	return req.Write(types.MCPServerList{Items: mcpServers})
}

func (m *MCPHandler) GetServerFromAllSources(req api.Context) error {
	var (
		server v1.MCPServer
		id     = req.PathValue("mcp_server_id")
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	// Check if server is from default catalog or workspace
	if server.Spec.MCPCatalogID != system.DefaultCatalog && server.Spec.PowerUserWorkspaceID == "" {
		return types.NewErrNotFound("MCP server not found")
	}

	// Authorization check.
	if !req.UserIsAdmin() {
		var (
			hasAccess bool
			err       error
		)

		if server.Spec.MCPCatalogID != "" {
			hasAccess, err = m.acrHelper.UserHasAccessToMCPServerInCatalog(req.User, server.Name, server.Spec.MCPCatalogID)
		} else if server.Spec.PowerUserWorkspaceID != "" {
			hasAccess, err = m.acrHelper.UserHasAccessToMCPServerInWorkspace(req.User, server.Name, server.Spec.PowerUserWorkspaceID, server.Spec.UserID)
		}
		if err != nil {
			return err
		}
		if !hasAccess {
			return types.NewErrForbidden("user is not authorized to access this MCP server")
		}
	}

	// Get credential context based on server scoping
	var credCtxs []string
	if server.Spec.MCPCatalogID != "" {
		credCtxs = []string{fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name)}
	} else if server.Spec.PowerUserWorkspaceID != "" {
		credCtxs = []string{fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name)}
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), credCtxs, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	addExtractedEnvVars(&server)

	// Enrich with tool preview data if catalog entry exists
	if server.Spec.MCPServerCatalogEntryName != "" {
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err == nil {
			// Add tool preview from catalog entry to server manifest
			if entry.Spec.Manifest.ToolPreview != nil {
				server.Spec.Manifest.ToolPreview = entry.Spec.Manifest.ToolPreview
			}
		}
		// Don't fail if catalog entry is missing, just continue without preview
	}

	slug, err := slugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), system.DefaultCatalog, server.Spec.PowerUserWorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(convertMCPServer(server, cred.Env, m.serverURL, slug))
}

func (m *MCPHandler) ClearOAuthCredentials(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	var server v1.MCPServer
	if err := req.Get(&server, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}
	if err := req.GatewayClient.DeleteMCPOAuthToken(req.Context(), req.User.GetUID(), server.Name); err != nil {
		return fmt.Errorf("failed to delete OAuth credentials: %v", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func (m *MCPHandler) GetServerDetails(req api.Context) error {
	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return err
	}

	if server.Spec.Template {
		return types.NewErrNotFound("MCP server not found")
	}

	if serverConfig.Runtime == types.RuntimeRemote {
		return types.NewErrBadRequest("cannot get details for remote MCP server")
	}

	if !req.UserIsAdmin() && !req.UserIsAuditor() {
		workspaceID := req.PathValue("workspace_id")
		if workspaceID == "" {
			return types.NewErrNotFound("MCP server %s not found", server.Name)
		} else if server.Spec.PowerUserWorkspaceID != "" && workspaceID != server.Spec.PowerUserWorkspaceID {
			return types.NewErrNotFound("MCP server %s not found", server.Name)
		} else if server.Spec.PowerUserWorkspaceID == "" {
			if server.Spec.MCPServerCatalogEntryName == "" {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			}

			// In this case, the server should correspond to a workspace catalog entry.
			var entry v1.MCPServerCatalogEntry
			if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
				return fmt.Errorf("failed to get MCP server catalog entry: %v", err)
			}

			if entry.Spec.PowerUserWorkspaceID != workspaceID {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			}
		}
	}

	mcpServerDisplayName := server.Spec.Manifest.Name
	if mcpServerDisplayName == "" {
		mcpServerDisplayName = server.Name
	}

	details, err := m.mcpSessionManager.GetServerDetails(req.Context(), server.Spec.UserID, mcpServerDisplayName, server.Name, serverConfig)
	if err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrNotFound(nse.Error())
		}
		return err
	}

	return req.Write(details)
}

func (m *MCPHandler) RestartServerDeployment(req api.Context) error {
	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return err
	}

	if serverConfig.Runtime == types.RuntimeRemote {
		return types.NewErrBadRequest("cannot restart deployment for remote MCP server")
	}

	if !req.UserIsAdmin() {
		workspaceID := req.PathValue("workspace_id")
		if workspaceID == "" {
			return types.NewErrNotFound("MCP server %s not found", server.Name)
		} else if server.Spec.PowerUserWorkspaceID != "" && workspaceID != server.Spec.PowerUserWorkspaceID {
			return types.NewErrNotFound("MCP server %s not found", server.Name)
		} else if server.Spec.PowerUserWorkspaceID == "" {
			if server.Spec.MCPServerCatalogEntryName == "" {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			}

			// In this case, the server should correspond to a workspace catalog entry.
			var entry v1.MCPServerCatalogEntry
			if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
				return fmt.Errorf("failed to get MCP server catalog entry: %v", err)
			}

			if entry.Spec.PowerUserWorkspaceID != workspaceID {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			}
		}
	}

	if err := m.mcpSessionManager.RestartServerDeployment(req.Context(), serverConfig); err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrNotFound(nse.Error())
		}
		return err
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

// CheckK8sSettingsStatus checks if a server needs redeployment with new K8s settings
func (m *MCPHandler) CheckK8sSettingsStatus(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryID := req.PathValue("entry_id")

	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return err
	}

	// Validate catalog/workspace membership
	// If entry_id is in the path, validate the server was created from that entry
	if entryID != "" {
		if server.Spec.MCPServerCatalogEntryName != entryID {
			return types.NewErrNotFound("MCP server not found")
		}

		// Get the entry and validate it's in the correct catalog/workspace
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, entryID); err != nil {
			return types.NewErrNotFound("MCP server not found")
		}

		// Validate the entry is in the correct catalog or workspace
		if entry.Spec.MCPCatalogName != catalogID || entry.Spec.PowerUserWorkspaceID != workspaceID {
			return types.NewErrNotFound("MCP server not found")
		}
	} else if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		// Multi-user server was not in the specified catalog or workspace
		return types.NewErrNotFound("MCP server not found")
	}

	// Remote servers don't have deployments
	if serverConfig.Runtime == types.RuntimeRemote {
		return types.NewErrBadRequest("K8s settings check is not supported for remote servers")
	}

	// Check status
	needsUpdate, deployedHash, err := m.mcpSessionManager.CheckK8sSettingsStatus(req.Context(), serverConfig)
	if err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrBadRequest("K8s settings check is only supported for Kubernetes runtime")
		}
		return err
	}

	// Get current K8s settings
	var k8sSettings v1.K8sSettings
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.K8sSettingsName,
	}, &k8sSettings); err != nil {
		return err
	}

	currentSettings, err := convertK8sSettings(k8sSettings)
	if err != nil {
		return err
	}

	status := types.K8sSettingsStatus{
		NeedsK8sUpdate:       needsUpdate,
		CurrentSettings:      &currentSettings,
		DeployedSettingsHash: deployedHash,
	}

	return req.Write(status)
}

// RedeployWithK8sSettings redeploys a server with the current K8s settings
func (m *MCPHandler) RedeployWithK8sSettings(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryID := req.PathValue("entry_id")

	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return err
	}

	// Validate catalog/workspace membership
	// If entry_id is in the path, validate the server was created from that entry
	if entryID != "" {
		if server.Spec.MCPServerCatalogEntryName != entryID {
			return types.NewErrNotFound("MCP server not found")
		}

		// Get the entry and validate it's in the correct catalog/workspace
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, entryID); err != nil {
			return types.NewErrNotFound("MCP server not found")
		}

		// Validate the entry is in the correct catalog or workspace
		if entry.Spec.MCPCatalogName != catalogID || entry.Spec.PowerUserWorkspaceID != workspaceID {
			return types.NewErrNotFound("MCP server not found")
		}
	} else if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		// Multi-user server was not in the specified catalog or workspace
		return types.NewErrNotFound("MCP server not found")
	}

	// Remote servers don't have deployments
	if serverConfig.Runtime == types.RuntimeRemote {
		return types.NewErrBadRequest("Redeployment is not supported for remote servers")
	}

	// Check if server actually needs update
	needsUpdate, _, err := m.mcpSessionManager.CheckK8sSettingsStatus(req.Context(), serverConfig)
	if err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrBadRequest("Redeployment is only supported for Kubernetes runtime")
		}
		return err
	}

	if !needsUpdate {
		return types.NewErrBadRequest("Server is already using the current K8s settings")
	}

	// Trigger restart to force redeployment with new settings
	if err := m.mcpSessionManager.RestartServerDeployment(req.Context(), serverConfig); err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrBadRequest("Restart is not supported by the current backend")
		}
		return fmt.Errorf("failed to redeploy server: %w", err)
	}

	// Get credential for server
	var credCtxs []string
	if server.Spec.MCPCatalogID != "" {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name))
	} else if server.Spec.PowerUserWorkspaceID != "" {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name))
	} else {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name))
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), credCtxs, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	slug, err := slugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	// Return updated server
	converted := convertMCPServer(server, cred.Env, m.serverURL, slug)
	return req.Write(converted)
}

func (m *MCPHandler) StreamServerLogs(req api.Context) error {
	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return err
	}

	if serverConfig.Runtime == types.RuntimeRemote {
		return types.NewErrBadRequest("cannot stream logs for remote MCP server")
	}

	// If this is a single-user MCP server that belongs to the user, then let them access the logs.
	if server.Spec.UserID != req.User.GetUID() || server.Spec.PowerUserWorkspaceID != "" || server.Spec.MCPCatalogID != "" {
		// If the user doesn't own the server and is not an admin or auditor, check if they have access to the workspace.
		if !req.UserIsAdmin() && !req.UserIsAuditor() {
			workspaceID := req.PathValue("workspace_id")
			if workspaceID == "" {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			} else if server.Spec.PowerUserWorkspaceID != "" && workspaceID != server.Spec.PowerUserWorkspaceID {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			} else if server.Spec.PowerUserWorkspaceID == "" {
				if server.Spec.MCPServerCatalogEntryName == "" {
					return types.NewErrNotFound("MCP server %s not found", server.Name)
				}

				// In this case, the server should correspond to a workspace catalog entry.
				var entry v1.MCPServerCatalogEntry
				if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
					return fmt.Errorf("failed to get MCP server catalog entry: %v", err)
				}

				if entry.Spec.PowerUserWorkspaceID != workspaceID {
					return types.NewErrNotFound("MCP server %s not found", server.Name)
				}
			}
		}
	}

	mcpServerDisplayName := server.Spec.Manifest.Name
	if mcpServerDisplayName == "" {
		mcpServerDisplayName = server.Name
	}

	logs, err := m.mcpSessionManager.StreamServerLogs(req.Context(), server.Spec.UserID, mcpServerDisplayName, server.Name, serverConfig)
	if err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrNotFound(nse.Error())
		}
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
			if line[0] == '\x01' || line[0] == '\x02' {
				// Docker appends a header to each line of logs so that it knows where to send the log (stdout/stderr)
				// and how long the log is. We don't need this information and it doesn't produce good output.
				// See https://github.com/moby/moby/issues/7375#issuecomment-51462963
				line = line[min(8, len(line)):]
			}
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

	if mcpServer.Spec.MCPCatalogID != "" {
		return types.NewErrBadRequest("cannot update the URL for a multi-user MCP server; use the UpdateServer endpoint instead")
	}

	if mcpServer.Spec.MCPServerCatalogEntryName == "" {
		return types.NewErrBadRequest("this server does not have a catalog entry")
	}

	if mcpServer.Spec.Manifest.Runtime != types.RuntimeRemote || mcpServer.Spec.Manifest.RemoteConfig == nil {
		return types.NewErrBadRequest("cannot update the URL for a non-remote MCP server")
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, mcpServer.Spec.MCPServerCatalogEntryName); err != nil {
		return fmt.Errorf("failed to get catalog entry: %w", err)
	}

	if entry.Spec.Manifest.RemoteConfig == nil {
		return types.NewErrBadRequest("the catalog entry for this server does not have remote configuration")
	}

	if entry.Spec.Manifest.RemoteConfig.FixedURL != "" {
		return types.NewErrBadRequest("this server already has a fixed URL that cannot be updated")
	}

	if entry.Spec.Manifest.RemoteConfig.Hostname == "" {
		return types.NewErrBadRequest("the catalog entry for this server does not have a hostname")
	}

	var input struct {
		URL string `json:"url"`
	}
	if err := req.Read(&input); err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	if !strings.HasPrefix(input.URL, "http") {
		input.URL = "https://" + input.URL
	}

	if err := types.ValidateURLHostname(input.URL, entry.Spec.Manifest.RemoteConfig.Hostname); err != nil {
		return types.NewErrBadRequest("the hostname in the URL does not match the hostname in the catalog entry: %v", err)
	}

	parsedURL, err := url.Parse(input.URL)
	if err != nil {
		return types.NewErrBadRequest("failed to parse input URL: %v", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return types.NewErrBadRequest("the URL must be HTTP or HTTPS")
	}

	mcpServer.Spec.Manifest.RemoteConfig.URL = input.URL
	mcpServer.Spec.NeedsURL = false
	mcpServer.Spec.PreviousURL = ""

	if err := validation.ValidateServerManifest(mcpServer.Spec.Manifest); err != nil {
		return err
	}

	if err := req.Update(&mcpServer); err != nil {
		return fmt.Errorf("failed to update server: %w", err)
	}

	slug, err := slugForMCPServer(req.Context(), req.Storage, mcpServer, req.User.GetUID(), "", "")
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(convertMCPServer(mcpServer, nil, m.serverURL, slug))
}

func (m *MCPHandler) TriggerUpdate(req api.Context) error {
	var server v1.MCPServer
	if err := req.Get(&server, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	if server.Spec.MCPCatalogID != "" || server.Spec.PowerUserWorkspaceID != "" {
		return types.NewErrBadRequest("cannot trigger update for a multi-user MCP server; use the UpdateServer endpoint instead")
	}

	if server.Spec.MCPServerCatalogEntryName == "" || !server.Status.NeedsUpdate {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
		return err
	}

	if !req.UserIsAdmin() {
		workspaceID := req.PathValue("workspace_id")
		if workspaceID == "" {
			return types.NewErrNotFound("MCP server %s not found", server.Name)
		}

		if entry.Spec.PowerUserWorkspaceID != workspaceID {
			return types.NewErrNotFound("MCP server %s not found", server.Name)
		}
	}

	oldServer := server.DeepCopy()

	// Update the server manifest with the latest from the catalog entry
	server.Spec.Manifest.Metadata = entry.Spec.Manifest.Metadata
	server.Spec.Manifest.Name = entry.Spec.Manifest.Name
	server.Spec.Manifest.Description = entry.Spec.Manifest.Description
	server.Spec.Manifest.Icon = entry.Spec.Manifest.Icon
	server.Spec.Manifest.Env = entry.Spec.Manifest.Env
	server.Spec.Manifest.Runtime = entry.Spec.Manifest.Runtime
	server.Spec.Manifest.UVXConfig = entry.Spec.Manifest.UVXConfig
	server.Spec.Manifest.NPXConfig = entry.Spec.Manifest.NPXConfig
	server.Spec.Manifest.ContainerizedConfig = entry.Spec.Manifest.ContainerizedConfig

	// Handle remote runtime URL updates
	if entry.Spec.Manifest.Runtime == types.RuntimeRemote && entry.Spec.Manifest.RemoteConfig != nil {
		if entry.Spec.Manifest.RemoteConfig.FixedURL != "" {
			// Use the fixed URL from catalog entry
			server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{
				URL:     entry.Spec.Manifest.RemoteConfig.FixedURL,
				Headers: entry.Spec.Manifest.RemoteConfig.Headers,
			}
		} else if entry.Spec.Manifest.RemoteConfig.Hostname != "" {
			// Check if the server's current URL matches the new hostname requirement
			if server.Spec.Manifest.RemoteConfig != nil && server.Spec.Manifest.RemoteConfig.URL != "" {
				hostnameMismatchErr := types.ValidateURLHostname(server.Spec.Manifest.RemoteConfig.URL, entry.Spec.Manifest.RemoteConfig.Hostname)

				server.Spec.NeedsURL = hostnameMismatchErr != nil
				if server.Spec.NeedsURL {
					server.Spec.PreviousURL = server.Spec.Manifest.RemoteConfig.URL
					server.Spec.Manifest.RemoteConfig.URL = ""
				}

				server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{
					Headers: entry.Spec.Manifest.RemoteConfig.Headers,
				}
			} else {
				// No current URL, needs one
				server.Spec.NeedsURL = true
				server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{
					Headers: entry.Spec.Manifest.RemoteConfig.Headers,
				}
			}
		}
	} else {
		// For non-remote runtimes, clear the remote config
		server.Spec.Manifest.RemoteConfig = nil
	}

	// Shutdown any server that is using the default credentials.
	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)}, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	// Shutdown the server, even if there is no credential
	if err := m.removeMCPServer(req.Context(), *oldServer, server.Spec.UserID, cred.Env); err != nil {
		return err
	}

	if err := req.Update(&server); err != nil {
		return err
	}

	return nil
}

// ListServerInstances returns all instances for all servers within a specific catalog
func (m *MCPHandler) ListServerInstances(req api.Context) error {
	catalogID := req.PathValue("catalog_id")

	// Verify the catalog exists
	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogID); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	// Get all servers in this catalog
	var serverList v1.MCPServerList
	if err := req.List(&serverList, kclient.MatchingFields{
		"spec.mcpCatalogID": catalogID,
	}); err != nil {
		return fmt.Errorf("failed to list servers in catalog: %w", err)
	}

	// Filter out template servers
	var catalogServers []v1.MCPServer
	for _, server := range serverList.Items {
		if !server.Spec.Template {
			catalogServers = append(catalogServers, server)
		}
	}

	// Get all instances for these catalog servers
	var allInstances v1.MCPServerInstanceList
	if err := req.List(&allInstances); err != nil {
		return fmt.Errorf("failed to list server instances: %w", err)
	}

	// Filter instances that belong to servers in this catalog
	var catalogServerNames = make(map[string]struct{})
	for _, server := range catalogServers {
		catalogServerNames[server.Name] = struct{}{}
	}

	var filteredInstances []v1.MCPServerInstance
	for _, instance := range allInstances.Items {
		if instance.Spec.Template {
			// Hide template instances
			continue
		}
		if _, exists := catalogServerNames[instance.Spec.MCPServerName]; exists {
			filteredInstances = append(filteredInstances, instance)
		}
	}

	// Convert instances to API types
	convertedInstances := make([]types.MCPServerInstance, 0, len(filteredInstances))
	for _, instance := range filteredInstances {
		slug, err := slugForMCPServerInstance(req.Context(), req.Storage, instance)
		if err != nil {
			return fmt.Errorf("failed to determine slug for instance %s: %w", instance.Name, err)
		}

		convertedInstances = append(convertedInstances, convertMCPServerInstance(instance, m.serverURL, slug))
	}

	return req.Write(types.MCPServerInstanceList{
		Items: convertedInstances,
	})
}
