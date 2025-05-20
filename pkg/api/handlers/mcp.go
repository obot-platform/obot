package handlers

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gptscript-ai/go-gptscript"
	gtypes "github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/projects"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type MCPHandler struct {
	gptscript         *gptscript.GPTScript
	mcpSessionManager *mcp.SessionManager
}

func NewMCPHandler(gptscript *gptscript.GPTScript, mcpLoader *mcp.SessionManager) *MCPHandler {
	return &MCPHandler{
		gptscript:         gptscript,
		mcpSessionManager: mcpLoader,
	}
}

func (m *MCPHandler) GetCatalogEntry(req api.Context) error {
	var (
		entry v1.MCPServerCatalogEntry
		id    = req.PathValue("mcp_server_id")
	)

	if err := req.Get(&entry, id); err != nil {
		return err
	}

	return req.Write(convertMCPServerCatalogEntry(entry))
}

func (m *MCPHandler) ListCatalog(req api.Context) error {
	var list v1.MCPServerCatalogEntryList
	if err := req.List(&list); err != nil {
		return err
	}

	items := make([]types.MCPServerCatalogEntry, 0, len(list.Items))
	for _, entry := range list.Items {
		items = append(items, convertMCPServerCatalogEntry(entry))
	}

	return req.Write(types.MCPServerCatalogEntryList{Items: items})
}

func convertMCPServerCatalogEntry(entry v1.MCPServerCatalogEntry) types.MCPServerCatalogEntry {
	return types.MCPServerCatalogEntry{
		Metadata:          MetadataFrom(&entry),
		CommandManifest:   entry.Spec.CommandManifest,
		URLManifest:       entry.Spec.URLManifest,
		ToolReferenceName: entry.Spec.ToolReferenceName,
	}
}

func (m *MCPHandler) ListServer(req api.Context) error {
	withTools := req.URL.Query().Get("tools") == "true"
	t, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	topMost, err := projects.GetRoot(req.Context(), req.Storage, t)
	if err != nil {
		return err
	}

	var servers v1.MCPServerList
	if err := req.List(&servers, kclient.MatchingFields{
		"spec.threadName": topMost.Name,
	}); err != nil {
		return nil
	}

	project, err := getProjectThread(req)
	if err != nil {
		return err
	}

	credCtxs := make([]string, 0, len(servers.Items))
	for _, server := range servers.Items {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", project.Name, server.Name))
		if project.IsSharedProject() {
			// Add default credentials shared by the agent for this MCP server if available.
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s-shared", server.Spec.ThreadName, server.Name))
		}
	}

	creds, err := m.gptscript.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: credCtxs,
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	credMap := make(map[string]map[string]string, len(creds))
	for _, cred := range creds {
		if _, ok := credMap[cred.ToolName]; !ok {
			credMap[cred.ToolName] = cred.Env
		}
	}

	var tools []types.MCPServerTool
	items := make([]types.MCPServer, 0, len(servers.Items))
	for _, server := range servers.Items {
		if withTools {
			credCtxs := []string{fmt.Sprintf("%s-%s", project.Name, server.Name)}
			if project.IsSharedProject() {
				credCtxs = append(credCtxs, fmt.Sprintf("%s-%s-shared", server.Spec.ThreadName, server.Name))
			}

			// If we want to get the tools, then we have to reveal the credentials
			// so we know what the values are and not only which values are set.
			c, err := m.gptscript.RevealCredential(req.Context(), credCtxs, server.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to find credential: %w", err)
			}

			thread, err := getThreadForScope(req)
			if err != nil {
				return err
			}

			tools, err = m.toolsForServer(req.Context(), req.Storage, project.Name, server, thread.Spec.Manifest.AllowedMCPTools[server.Name], c.Env)
			if err != nil {
				if uc := (*render.UnconfiguredMCPError)(nil); errors.As(err, &uc) {
					// Leave out tools for un-configured MCP servers
					continue
				}
				return fmt.Errorf("failed to render tools: %w", err)
			}
		}

		items = append(items, convertMCPServer(server, tools, credMap[server.Name]))
	}

	return req.Write(types.MCPServerList{Items: items})
}

func (m *MCPHandler) GetServer(req api.Context) error {
	return m.getServer(req, req.URL.Query().Get("tools") == "true")
}

func (m *MCPHandler) GetServerWithTools(req api.Context) error {
	return m.getServer(req, true)
}

func (m *MCPHandler) getServer(req api.Context, withTools bool) error {
	var (
		server v1.MCPServer
		id     = req.PathValue("mcp_server_id")
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	project, err := getProjectThread(req)
	if err != nil {
		return err
	}

	credCtxs := []string{
		fmt.Sprintf("%s-%s", project.Name, server.Name),
	}
	if project.IsSharedProject() {
		// Add default credentials shared by the agent for this MCP server if available.
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s-shared", server.Spec.ThreadName, server.Name))
	}

	cred, err := m.gptscript.RevealCredential(req.Context(), credCtxs, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	var tools []types.MCPServerTool
	if withTools {
		thread, err := getThreadForScope(req)
		if err != nil {
			return err
		}

		allowedTools := thread.Spec.Manifest.AllowedMCPTools[server.Name]
		if len(allowedTools) == 0 && !thread.Spec.Project {
			// If there is no information about allowed tools on the thread, then check the project.
			allowedTools = project.Spec.Manifest.AllowedMCPTools[server.Name]
		}

		tools, err = m.toolsForServer(req.Context(), req.Storage, project.Name, server, allowedTools, cred.Env)
		if err != nil {
			return fmt.Errorf("failed to render tools: %w", err)
		}
	}

	return req.Write(convertMCPServer(server, tools, cred.Env))
}

func (m *MCPHandler) DeleteServer(req api.Context) error {
	var (
		server v1.MCPServer
		id     = req.PathValue("mcp_server_id")
	)

	project, err := getProjectThread(req)
	if err != nil {
		return err
	}

	if err := req.Get(&server, id); err != nil {
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

	if err := req.Delete(&server); err != nil {
		return err
	}

	return req.Write(convertMCPServer(server, nil, nil))
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
	if override.URL != "" {
		existing.URL = override.URL
	}
	if len(override.Headers) > 0 {
		existing.Headers = override.Headers
	}

	return existing
}

func (m *MCPHandler) CreateServer(req api.Context) error {
	var input types.MCPServer
	if err := req.Read(&input); err != nil {
		return err
	}

	t, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	server := v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.MCPServerPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.MCPServerSpec{
			Manifest:                  input.MCPServerManifest,
			MCPServerCatalogEntryName: input.CatalogID,
			ThreadName:                t.Name,
			UserID:                    req.User.GetUID(),
		},
	}

	if input.CatalogID != "" {
		var catalogEntry v1.MCPServerCatalogEntry
		if err := req.Get(&catalogEntry, input.CatalogID); err != nil {
			return err
		}

		if catalogEntry.Spec.CommandManifest.Server.URL != "" {
			server.Spec.Manifest = catalogEntry.Spec.URLManifest.Server
		} else {
			server.Spec.Manifest = catalogEntry.Spec.CommandManifest.Server
		}
		server.Spec.ToolReferenceName = catalogEntry.Spec.ToolReferenceName
		server.Spec.DefaultDisabledTools = catalogEntry.Spec.DefaultDisabledTools
		// Override the defaults from the catalog with the values from the request.
		server.Spec.Manifest = mergeMCPServerManifests(server.Spec.Manifest, input.MCPServerManifest)
	}

	if err = req.Create(&server); err != nil {
		return err
	}

	cred, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", server.Spec.ThreadName, server.Name)}, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	return req.WriteCreated(convertMCPServer(server, nil, cred.Env))
}

func (m *MCPHandler) UpdateServer(req api.Context) error {
	var (
		id       = req.PathValue("mcp_server_id")
		updated  types.MCPServerManifest
		existing v1.MCPServer
	)

	if err := req.Get(&existing, id); err != nil {
		return err
	}

	project, err := getProjectThread(req)
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

	if err := req.Read(&updated); err != nil {
		return err
	}

	// Shutdown any server that is using the default credentials.
	cred, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", existing.Spec.ThreadName, existing.Name)}, existing.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	// Shutdown the server, even if there is no credential
	if err = m.removeMCPServer(req.Context(), existing, project.Name, cred.Env); err != nil {
		return err
	}

	// Shutdown the MCP server using any shared credentials.
	cred, err = m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s-shared", existing.Spec.ThreadName, existing.Name)}, existing.Name)
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
		childCred, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", chatBot.Name, existing.Name)}, existing.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		} else if err != nil {
			// Use the shared parent credential if we didn't find the chatbot's credential.
			childCred = cred
		}

		// Shutdown the server, even if there is no credential
		if err = m.removeMCPServer(req.Context(), existing, chatBot.Name, childCred.Env); err != nil {
			return err
		}
	}

	existing.Spec.Manifest = updated
	if err := req.Update(&existing); err != nil {
		return err
	}

	return req.Write(convertMCPServer(existing, nil, cred.Env))
}

func (m *MCPHandler) ConfigureServer(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	project, err := getProjectThread(req)
	if err != nil {
		return err
	}

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	credCtx := fmt.Sprintf("%s-%s", project.Name, mcpServer.Name)
	if err = m.removeMCPServerAndCred(req.Context(), mcpServer, project.Name, []string{credCtx}); err != nil {
		return err
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err := m.gptscript.CreateCredential(req.Context(), gptscript.Credential{
		Context:  credCtx,
		ToolName: mcpServer.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	return req.Write(convertMCPServer(mcpServer, nil, envVars))
}

func (m *MCPHandler) ConfigureSharedServer(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

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
	cred, err := m.gptscript.RevealCredential(req.Context(), []string{credCtx}, mcpServer.Name)
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
	if err = m.removeMCPServerAndCred(req.Context(), mcpServer, project.Name, []string{credCtx}); err != nil {
		return err
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err := m.gptscript.CreateCredential(req.Context(), gptscript.Credential{
		Context:  credCtx,
		ToolName: mcpServer.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	return req.Write(convertMCPServer(mcpServer, nil, envVars))
}

func (m *MCPHandler) DeconfigureServer(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	project, err := getProjectThread(req)
	if err != nil {
		return err
	}

	if err = m.removeMCPServerAndCred(req.Context(), mcpServer, project.Name, []string{fmt.Sprintf("%s-%s", project.Name, mcpServer.Name)}); err != nil {
		return err
	}

	return req.Write(convertMCPServer(mcpServer, nil, nil))
}

func (m *MCPHandler) DeconfigureSharedServer(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

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

	cred, err := m.gptscript.RevealCredential(req.Context(), credCtx, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	for _, chatBot := range chatBots.Items {
		if err = m.removeMCPServer(req.Context(), mcpServer, chatBot.Name, cred.Env); err != nil {
			return err
		}
	}

	// Remove the top-level MCP server if it exists and remove the credential.
	if err = m.removeMCPServerAndCred(req.Context(), mcpServer, project.Name, credCtx); err != nil {
		return err
	}

	return req.Write(convertMCPServer(mcpServer, nil, nil))
}

func (m *MCPHandler) Reveal(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	project, err := getProjectThread(req)
	if err != nil {
		return err
	}

	cred, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", project.Name, mcpServer.Name)}, mcpServer.Name)
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

	cred, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s-shared", mcpServer.Spec.ThreadName, mcpServer.Name)}, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", mcpServer.Name)
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

	cred, err := m.gptscript.RevealCredential(req.Context(), credCtxs, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	mcpTools, err := m.toolsForServer(req.Context(), req.Storage, project.Name, mcpServer, thread.Spec.Manifest.AllowedMCPTools[mcpServer.Name], cred.Env)
	if err != nil {
		return fmt.Errorf("failed to render tools: %w", err)
	}

	if len(tools) == 0 {
		delete(thread.Spec.Manifest.AllowedMCPTools, mcpServer.Name)
	} else {
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
	}

	if err = req.Update(thread); err != nil {
		return fmt.Errorf("failed to update thread: %w", err)
	}

	return nil
}

func (m *MCPHandler) toolsForServer(ctx context.Context, client kclient.Client, projectThreadName string, server v1.MCPServer, allowedTools []string, credEnv map[string]string) ([]types.MCPServerTool, error) {
	allTools := slices.Contains(allowedTools, "*")
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
					Metadata:    ref.Status.Tool.Metadata,
					Params:      ref.Status.Tool.Params,
					Credentials: ref.Status.Tool.Credentials,
					Enabled:     allTools || len(allowedTools) == 0 || slices.Contains(allowedTools, ref.Name),
				})
			}
		}

		return tools, nil
	}

	tool, err := render.MCPServerToolWithCreds(server, projectThreadName, credEnv)
	if err != nil {
		return nil, err
	}

	gTools, err := m.mcpSessionManager.Load(ctx, gtypes.Tool{
		ToolDef: gtypes.ToolDef{
			Parameters: gtypes.Parameters{
				Name: tool.Name,
			},
			Instructions: tool.Instructions,
		},
	})
	if err != nil {
		return nil, err
	}

	// Exclude the first tool because it is the "bundle" tool, and we aren't concerned with that.
	tools := make([]types.MCPServerTool, 0, len(gTools)-1)
	for _, t := range gTools[1:] {
		mcpTool := types.MCPServerTool{
			ID:          t.Name,
			Name:        t.Name,
			Description: t.Description,
			Metadata:    t.MetaData,
			Enabled:     allTools || (len(allowedTools) == 0 && !slices.Contains(server.Spec.DefaultDisabledTools, t.Name)) || slices.Contains(allowedTools, t.Name),
		}

		if t.Arguments != nil {
			mcpTool.Params = make(map[string]string, len(t.Arguments.Properties))
			for name, param := range t.Arguments.Properties {
				if param.Value != nil {
					mcpTool.Params[name] = param.Value.Description
				}
			}
		}

		tools = append(tools, mcpTool)
	}

	return tools, nil
}

func (m *MCPHandler) removeMCPServer(ctx context.Context, mcpServer v1.MCPServer, projectName string, credEnv map[string]string) error {
	serverConfig, _ := mcp.ToServerConfig(mcpServer, projectName, credEnv, nil)
	if err := m.mcpSessionManager.ShutdownServer(ctx, serverConfig); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	return nil
}

func (m *MCPHandler) removeMCPServerAndCred(ctx context.Context, mcpServer v1.MCPServer, projectName string, credCtx []string) error {
	cred, err := m.gptscript.RevealCredential(ctx, credCtx, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	// Shutdown the server, even if there is no credential
	if err := m.removeMCPServer(ctx, mcpServer, projectName, cred.Env); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	// If revealing the credential was successful, remove it.
	if err == nil {
		if err = m.gptscript.DeleteCredential(ctx, cred.Context, mcpServer.Name); err != nil {
			return fmt.Errorf("failed to remove existing credential: %w", err)
		}
	}

	return nil
}

func convertMCPServer(server v1.MCPServer, tools []types.MCPServerTool, credEnv map[string]string) types.MCPServer {
	var missingEnvVars, missingHeaders []string
	for _, env := range server.Spec.Manifest.Env {
		if !env.Required {
			continue
		}

		if _, ok := credEnv[env.Key]; !ok {
			missingEnvVars = append(missingEnvVars, env.Key)
		}
	}
	for _, header := range server.Spec.Manifest.Headers {
		if !header.Required {
			continue
		}

		if _, ok := credEnv[header.Key]; !ok {
			missingHeaders = append(missingHeaders, header.Key)
		}
	}

	return types.MCPServer{
		Metadata:               MetadataFrom(&server),
		MissingRequiredEnvVars: missingEnvVars,
		MissingRequiredHeaders: missingHeaders,
		Configured:             len(missingEnvVars) == 0 && len(missingHeaders) == 0,
		MCPServerManifest:      server.Spec.Manifest,
		CatalogID:              server.Spec.MCPServerCatalogEntryName,
		Tools:                  tools,
	}
}
