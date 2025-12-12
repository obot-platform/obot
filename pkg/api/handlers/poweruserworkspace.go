package handlers

import (
	"errors"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type PowerUserWorkspaceHandler struct {
	serverURL string
	acrHelper *accesscontrolrule.Helper
}

func NewPowerUserWorkspaceHandler(serverURL string, acrHelper *accesscontrolrule.Helper) *PowerUserWorkspaceHandler {
	return &PowerUserWorkspaceHandler{
		serverURL: serverURL,
		acrHelper: acrHelper,
	}
}

// List returns power user workspaces. All users are allowed to list these.
func (*PowerUserWorkspaceHandler) List(req api.Context) error {
	var list v1.PowerUserWorkspaceList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list power user workspaces: %w", err)
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

func (p *PowerUserWorkspaceHandler) ListAllEntries(req api.Context) error {
	var list v1.PowerUserWorkspaceList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list power user workspaces: %w", err)
	}

	catalogEntries := make([]types.MCPServerCatalogEntry, 0)
	for _, item := range list.Items {
		fieldSelector := kclient.MatchingFields{"spec.powerUserWorkspaceID": item.Name}
		var list2 v1.MCPServerCatalogEntryList
		if err := req.List(&list2, fieldSelector); err != nil {
			return fmt.Errorf("failed to list entries: %w", err)
		}

		for _, entry := range list2.Items {
			catalogEntries = append(catalogEntries, ConvertMCPServerCatalogEntryWithWorkspace(entry, item.Name, item.Spec.UserID))
		}
	}

	return req.Write(types.MCPServerCatalogEntryList{
		Items: catalogEntries,
	})
}

func (p *PowerUserWorkspaceHandler) ListAllServers(req api.Context) error {
	var serverList v1.MCPServerList
	if err := req.List(&serverList); err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	// Filter servers that have a non-empty PowerUserWorkspaceID
	var filteredServers []v1.MCPServer
	for _, server := range serverList.Items {
		if server.Spec.PowerUserWorkspaceID != "" {
			filteredServers = append(filteredServers, server)
		}
	}

	// Build credential contexts for all filtered servers
	credCtxs := make([]string, 0, len(filteredServers))
	for _, server := range filteredServers {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name))
	}

	var credMap map[string]map[string]string
	if len(credCtxs) > 0 {
		creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
			CredentialContexts: credCtxs,
		})
		if err != nil {
			return fmt.Errorf("failed to list credentials: %w", err)
		}

		credMap = make(map[string]map[string]string, len(creds))
		for _, cred := range creds {
			if _, ok := credMap[cred.ToolName]; !ok {
				c, err := req.GPTClient.RevealCredential(req.Context(), []string{cred.Context}, cred.ToolName)
				if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
					return fmt.Errorf("failed to find credential: %w", err)
				}
				credMap[cred.ToolName] = c.Env
			}
		}
	}

	servers := make([]types.MCPServer, 0, len(filteredServers))
	for _, server := range filteredServers {
		// Add extracted env vars to the server definition
		addExtractedEnvVars(&server)

		slug, err := SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), "", server.Spec.PowerUserWorkspaceID)
		if err != nil {
			return fmt.Errorf("failed to determine slug: %w", err)
		}

		servers = append(servers, ConvertMCPServer(server, credMap[server.Name], p.serverURL, slug))
	}

	return req.Write(types.MCPServerList{
		Items: servers,
	})
}

func (p *PowerUserWorkspaceHandler) ListAllServersForAllEntries(req api.Context) error {
	// Get all workspaces
	var workspaceList v1.PowerUserWorkspaceList
	if err := req.List(&workspaceList); err != nil {
		return fmt.Errorf("failed to list power user workspaces: %w", err)
	}

	// Get all entries from all workspaces
	var (
		allEntries []v1.MCPServerCatalogEntry
		entryList  v1.MCPServerCatalogEntryList
	)
	if err := req.List(&entryList); err != nil {
		return fmt.Errorf("failed to list entries: %w", err)
	}

	// Create a map of workspace names for efficient lookup
	workspaceNames := make(map[string]struct{}, len(workspaceList.Items))
	for _, workspace := range workspaceList.Items {
		workspaceNames[workspace.Name] = struct{}{}
	}

	// Filter entries that belong to any workspace
	for _, entry := range entryList.Items {
		if entry.Spec.PowerUserWorkspaceID != "" {
			allEntries = append(allEntries, entry)
		}
	}

	// For each entry, get its servers using the same approach as ListServersForEntry
	var allServers []v1.MCPServer
	for _, entry := range allEntries {
		var serverList v1.MCPServerList
		if err := req.List(&serverList, kclient.MatchingFields{
			"spec.mcpServerCatalogEntryName": entry.Name,
		}); err != nil {
			return fmt.Errorf("failed to list servers for entry %s: %w", entry.Name, err)
		}
		allServers = append(allServers, serverList.Items...)
	}

	// Filter out template servers
	var (
		filteredServers []v1.MCPServer
		seenServers     = make(map[string]bool)
	)
	for _, server := range allServers {
		if _, seen := seenServers[server.Name]; seen || server.Spec.Template {
			continue
		}

		seenServers[server.Name] = true
		filteredServers = append(filteredServers, server)
	}

	// Build credential contexts for all filtered servers
	credCtxs := make([]string, 0, len(filteredServers))
	for _, server := range filteredServers {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name))
	}

	var credMap map[string]map[string]string
	if len(credCtxs) > 0 {
		creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
			CredentialContexts: credCtxs,
		})
		if err != nil {
			return fmt.Errorf("failed to list credentials: %w", err)
		}

		credMap = make(map[string]map[string]string, len(creds))
		for _, cred := range creds {
			if _, ok := credMap[cred.ToolName]; !ok {
				c, err := req.GPTClient.RevealCredential(req.Context(), []string{cred.Context}, cred.ToolName)
				if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
					return fmt.Errorf("failed to find credential: %w", err)
				}
				credMap[cred.ToolName] = c.Env
			}
		}
	}

	servers := make([]types.MCPServer, 0, len(filteredServers))
	for _, server := range filteredServers {
		// Add extracted env vars to the server definition
		addExtractedEnvVars(&server)

		slug, err := SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), "", "")
		if err != nil {
			return fmt.Errorf("failed to determine slug: %w", err)
		}

		servers = append(servers, ConvertMCPServer(server, credMap[server.Name], p.serverURL, slug))
	}

	return req.Write(types.MCPServerList{
		Items: servers,
	})
}

func (p *PowerUserWorkspaceHandler) ListAllAccessControlRules(req api.Context) error {
	var list v1.PowerUserWorkspaceList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list power user workspaces: %w", err)
	}

	accessControlRules := make([]types.AccessControlRule, 0)
	for _, item := range list.Items {
		fieldSelector := kclient.MatchingFields{"spec.powerUserWorkspaceID": item.Name}
		var acrList v1.AccessControlRuleList
		if err := req.List(&acrList, fieldSelector); err != nil {
			return fmt.Errorf("failed to list access control rules: %w", err)
		}

		for _, acr := range acrList.Items {
			accessControlRules = append(accessControlRules, convertAccessControlRuleWithWorkspace(acr, item.Spec.UserID))
		}
	}

	return req.Write(types.AccessControlRuleList{
		Items: accessControlRules,
	})
}

func convertAccessControlRuleWithWorkspace(rule v1.AccessControlRule, powerUserID string) types.AccessControlRule {
	return types.AccessControlRule{
		Metadata:                  MetadataFrom(&rule),
		MCPCatalogID:              rule.Spec.MCPCatalogID,
		PowerUserWorkspaceID:      rule.Spec.PowerUserWorkspaceID,
		PowerUserID:               powerUserID,
		Generated:                 rule.Spec.Generated,
		AccessControlRuleManifest: rule.Spec.Manifest,
	}
}

func (p *PowerUserWorkspaceHandler) ListAllServerInstances(req api.Context) error {
	// Get all multi-user servers (servers with PowerUserWorkspaceID)
	var serverList v1.MCPServerList
	if err := req.List(&serverList); err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	// Filter to only multi-user servers
	var multiUserServers []v1.MCPServer
	for _, server := range serverList.Items {
		if server.Spec.PowerUserWorkspaceID != "" {
			multiUserServers = append(multiUserServers, server)
		}
	}

	// Get all instances for these multi-user servers
	var allInstances v1.MCPServerInstanceList
	if err := req.List(&allInstances); err != nil {
		return fmt.Errorf("failed to list server instances: %w", err)
	}

	// Filter instances that belong to multi-user servers
	var multiUserServerNames = make(map[string]bool)
	for _, server := range multiUserServers {
		multiUserServerNames[server.Name] = true
	}

	var filteredInstances []v1.MCPServerInstance
	for _, instance := range allInstances.Items {
		if instance.Spec.Template {
			// Hide template instances
			continue
		}
		if multiUserServerNames[instance.Spec.MCPServerName] {
			filteredInstances = append(filteredInstances, instance)
		}
	}

	// Convert instances to API types
	convertedInstances := make([]types.MCPServerInstance, 0, len(filteredInstances))
	for _, instance := range filteredInstances {
		slug, err := SlugForMCPServerInstance(req.Context(), req.Storage, instance)
		if err != nil {
			return fmt.Errorf("failed to determine slug for instance %s: %w", instance.Name, err)
		}

		convertedInstances = append(convertedInstances, ConvertMCPServerInstance(instance, p.serverURL, slug))
	}

	return req.Write(types.MCPServerInstanceList{
		Items: convertedInstances,
	})
}

func convertPowerUserWorkspace(workspace v1.PowerUserWorkspace) types.PowerUserWorkspace {
	return types.PowerUserWorkspace{
		Metadata: MetadataFrom(&workspace),
		UserID:   workspace.Spec.UserID,
		Role:     workspace.Spec.Role,
	}
}
