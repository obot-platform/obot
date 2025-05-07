package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/loader"
	gtypes "github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type MCPHandler struct {
	gptscript *gptscript.GPTScript
	mcpLoader loader.MCPLoader
}

func NewMCPHandler(gptscript *gptscript.GPTScript, mcpLoader loader.MCPLoader) *MCPHandler {
	return &MCPHandler{
		gptscript: gptscript,
		mcpLoader: mcpLoader,
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
		Metadata:        MetadataFrom(&entry),
		CommandManifest: entry.Spec.CommandManifest,
		URLManifest:     entry.Spec.URLManifest,
	}
}

func (m *MCPHandler) ListServer(req api.Context) error {
	withTools := req.URL.Query().Get("tools") == "true"
	t, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var servers v1.MCPServerList
	if err := req.List(&servers, kclient.MatchingFields{
		"spec.threadName": t.Name,
	}); err != nil {
		return nil
	}

	credCtxs := make([]string, 0, len(servers.Items))
	for _, server := range servers.Items {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.ThreadName, server.Name))
	}

	creds, err := m.gptscript.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: credCtxs,
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	credMap := make(map[string]map[string]string, len(creds))
	for _, cred := range creds {
		credMap[cred.ToolName] = cred.Env
	}

	var tools []gtypes.Tool
	items := make([]types.MCPServer, 0, len(servers.Items))
	for _, server := range servers.Items {
		if withTools {
			// If we want to get the tools, then we have to reveal the credentials
			// so we know what the values are and not only which values are set.
			c, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", server.Spec.ThreadName, server.Name)}, server.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to find credential: %w", err)
			}

			tools, err = m.toolsForServer(req.Context(), server, c.Env)
			if err != nil {
				return fmt.Errorf("failed to render tools: %w", err)
			}
		}

		items = append(items, convertMCPServer(server, tools, credMap[server.Name]))
	}

	return req.Write(types.MCPServerList{Items: items})
}

func (m *MCPHandler) GetServer(req api.Context) error {
	var (
		server    v1.MCPServer
		id        = req.PathValue("mcp_server_id")
		withTools = req.URL.Query().Get("tools") == "true"
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	cred, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", server.Spec.ThreadName, server.Name)}, server.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	}

	var tools []gtypes.Tool
	if withTools {
		tools, err = m.toolsForServer(req.Context(), server, cred.Env)
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

	if err := req.Get(&server, id); err != nil {
		return err
	}

	if err := m.gptscript.DeleteCredential(req.Context(), fmt.Sprintf("%s-%s", server.Spec.ThreadName, server.Name), server.Name); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	if err := req.Delete(&server); err != nil {
		return err
	}

	return req.Write(convertMCPServer(server, nil, nil))
}

func (m *MCPHandler) CreateServer(req api.Context) error {
	var input types.MCPServer

	t, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err = req.Read(&input); err != nil {
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
	}

	if input.URL != "" {
		server.Spec.Manifest.URL = input.URL
	}

	if err = req.Create(&server); err != nil {
		return err
	}

	cred, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", server.Spec.ThreadName, server.Name)}, server.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
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

	if err := req.Read(&updated); err != nil {
		return err
	}

	existing.Spec.Manifest = updated
	if err := req.Update(&existing); err != nil {
		return err
	}

	cred, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", existing.Spec.ThreadName, existing.Name)}, existing.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	}

	return req.Write(convertMCPServer(existing, nil, cred.Env))
}

func (m *MCPHandler) ConfigureServer(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	cred, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", mcpServer.Spec.ThreadName, mcpServer.Name)}, mcpServer.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = m.gptscript.DeleteCredential(req.Context(), cred.Context, mcpServer.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err := m.gptscript.CreateCredential(req.Context(), gptscript.Credential{
		Context:  fmt.Sprintf("%s-%s", mcpServer.Spec.ThreadName, mcpServer.Name),
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

	cred, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", mcpServer.Spec.ThreadName, mcpServer.Name)}, mcpServer.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = m.gptscript.DeleteCredential(req.Context(), cred.Context, mcpServer.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	return req.Write(convertMCPServer(mcpServer, nil, nil))
}

func (m *MCPHandler) Reveal(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	cred, err := m.gptscript.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", mcpServer.Spec.ThreadName, mcpServer.Name)}, mcpServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", mcpServer.Name)
}

func (m *MCPHandler) toolsForServer(ctx context.Context, server v1.MCPServer, credEnv map[string]string) ([]gtypes.Tool, error) {
	tool, err := render.MCPServerToolWithCreds(server, credEnv)
	if err != nil {
		return nil, err
	}

	// Instead of converting the whole tool type from the Go SDK type to the GPTScript type,
	// only convert the parts we need: Name and Instructions.
	return m.mcpLoader.Load(ctx, gtypes.Tool{
		ToolDef: gtypes.ToolDef{
			Parameters: gtypes.Parameters{
				Name: tool.Name,
			},
			Instructions: tool.Instructions,
		},
	})
}

func convertMCPServer(server v1.MCPServer, tools []gtypes.Tool, credEnv map[string]string) types.MCPServer {
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

	var mcpTools []types.MCPServerTool
	if len(tools) > 0 {
		mcpTools = make([]types.MCPServerTool, 0, len(tools))
		for _, t := range tools {
			tool := types.MCPServerTool{
				Name:        t.Name,
				Description: t.Description,
				Metadata:    t.MetaData,
			}

			if t.Arguments != nil {
				tool.Params = make(map[string]string, len(t.Arguments.Properties))
				for name, param := range t.Arguments.Properties {
					if param.Value != nil {
						tool.Params[name] = param.Value.Description
					}
				}
			}
			mcpTools = append(mcpTools, tool)
		}
	}

	return types.MCPServer{
		Metadata:               MetadataFrom(&server),
		MissingRequiredEnvVars: missingEnvVars,
		MissingRequiredHeaders: missingHeaders,
		Configured:             len(missingEnvVars) == 0 && len(missingHeaders) == 0,
		MCPServerManifest:      server.Spec.Manifest,
		CatalogID:              server.Spec.MCPServerCatalogEntryName,
		Tools:                  mcpTools,
	}
}
