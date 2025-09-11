package handlers

import (
	"errors"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type PowerUserWorkspaceHandler struct {
	serverURL string
}

func NewPowerUserWorkspaceHandler(serverURL string) *PowerUserWorkspaceHandler {
	return &PowerUserWorkspaceHandler{
		serverURL: serverURL,
	}
}

// List returns power user workspaces. Admins see all, non-admins see only their own.
func (*PowerUserWorkspaceHandler) List(req api.Context) error {
	var list v1.PowerUserWorkspaceList
	if req.UserIsAdmin() {
		// Admins can see all PowerUserWorkspaces
		if err := req.List(&list); err != nil {
			return fmt.Errorf("failed to list power user workspaces: %w", err)
		}
	} else {
		// Non-admins can only see their own workspace
		userID := req.User.GetUID()
		if err := req.List(&list, &kclient.ListOptions{
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.userID": userID,
			}),
		}); err != nil {
			return fmt.Errorf("failed to list power user workspaces: %w", err)
		}
	}

	items := make([]types.PowerUserWorkspace, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertPowerUserWorkspace(item))
	}

	return req.Write(types.PowerUserWorkspaceList{
		Items: items,
	})
}

// Get returns a specific power user workspace by ID.
func (*PowerUserWorkspaceHandler) Get(req api.Context) error {
	var workspace v1.PowerUserWorkspace
	if err := req.Get(&workspace, req.PathValue("workspace_id")); err != nil {
		return fmt.Errorf("failed to get power user workspace: %w", err)
	}

	return req.Write(convertPowerUserWorkspace(workspace))
}

func (*PowerUserWorkspaceHandler) ListAllEntries(req api.Context) error {
	var list v1.PowerUserWorkspaceList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list power user workspaces: %w", err)
	}

	catalogEntries := make([]types.MCPServerCatalogEntry, 0)
	for _, item := range list.Items {
		fieldSelector := client.MatchingFields{"spec.powerUserWorkspaceID": item.Name}
		var list2 v1.MCPServerCatalogEntryList
		if err := req.List(&list2, fieldSelector); err != nil {
			return fmt.Errorf("failed to list entries: %w", err)
		}
		for _, entry := range list2.Items {
			catalogEntries = append(catalogEntries, convertMCPServerCatalogEntry(entry))
		}
	}

	return req.Write(types.MCPServerCatalogEntryList{
		Items: catalogEntries,
	})
}

func (p *PowerUserWorkspaceHandler) ListAllServers(req api.Context) error {
	var list v1.PowerUserWorkspaceList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list power user workspaces: %w", err)
	}

	servers := make([]types.MCPServer, 0)
	for _, item := range list.Items {
		fieldSelector := client.MatchingFields{"spec.powerUserWorkspaceID": item.Name}
		var serverList v1.MCPServerList
		if err := req.List(&serverList, fieldSelector); err != nil {
			return fmt.Errorf("failed to list servers: %w", err)
		}

		credCtxs := make([]string, 0, len(serverList.Items))
		for _, server := range serverList.Items {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", item.Name, server.Name))
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

		for _, server := range serverList.Items {
			// Add extracted env vars to the server definition
			addExtractedEnvVars(&server)

			slug, err := slugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), "", item.Name)
			if err != nil {
				return fmt.Errorf("failed to determine slug: %w", err)
			}

			servers = append(servers, convertMCPServer(server, credMap[server.Name], p.serverURL, slug))
		}
	}

	return req.Write(types.MCPServerList{
		Items: servers,
	})
}

func convertPowerUserWorkspace(workspace v1.PowerUserWorkspace) types.PowerUserWorkspace {
	return types.PowerUserWorkspace{
		Metadata: MetadataFrom(&workspace),
		UserID:   workspace.Spec.UserID,
		Role:     workspace.Spec.Role,
	}
}
