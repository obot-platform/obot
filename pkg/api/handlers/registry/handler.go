package registry

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	acrHelper      *accesscontrolrule.Helper
	serverURL      string
	registryNoAuth bool
}

func NewHandler(acrHelper *accesscontrolrule.Helper, serverURL string, registryNoAuth bool) *Handler {
	return &Handler{
		acrHelper:      acrHelper,
		serverURL:      serverURL,
		registryNoAuth: registryNoAuth,
	}
}

// ListServers handles GET /v0.1/servers
func (h *Handler) ListServers(req api.Context) error {
	// Parse query parameters
	cursor := req.URL.Query().Get("cursor")
	limit := parseLimit(req.URL.Query().Get("limit"))
	search := req.URL.Query().Get("search")

	reverseDNS, err := ReverseDNSFromURL(h.serverURL)
	if err != nil {
		return fmt.Errorf("failed to generate reverse DNS: %w", err)
	}

	// Collect servers based on registry mode
	var servers []types.RegistryServerResponse
	if h.registryNoAuth {
		servers, err = h.collectAccessibleServersNoAuth(req, reverseDNS)
	} else {
		servers, err = h.collectAccessibleServers(req, reverseDNS)
	}
	if err != nil {
		return err
	}

	// Apply search filter if provided
	if search != "" {
		servers = filterServersBySearch(servers, search)
	}

	// Apply pagination
	response := paginateServers(servers, cursor, limit)

	return req.Write(response)
}

func (h *Handler) collectAccessibleServers(req api.Context, reverseDNS string) ([]types.RegistryServerResponse, error) {
	var result []types.RegistryServerResponse
	userID := req.User.GetUID()

	// Track what we've already added for deduplication
	addedCatalogEntries := make(map[string]bool) // catalog entry ID -> true

	// Step 1: List all user's own personal MCPServers (userID matches)
	personalServers, credMap, err := h.listPersonalServers(req, userID)
	if err != nil {
		return nil, err
	}

	for _, server := range personalServers {
		// Get slug for this server
		slug, err := handlers.SlugForMCPServer(req.Context(), req.Storage, server, userID, "", "")
		if err != nil {
			// Skip if we can't get slug
			continue
		}

		converted, err := ConvertMCPServerToRegistry(req.Context(), server, credMap[server.Name], h.serverURL, slug, reverseDNS, userID)
		if err != nil {
			// Skip servers that can't be converted
			continue
		}
		result = append(result, converted)

		// Track catalog entry for deduplication
		if server.Spec.MCPServerCatalogEntryName != "" {
			addedCatalogEntries[server.Spec.MCPServerCatalogEntryName] = true
		}
	}

	// Step 2: List catalog entries in default catalog with access
	catalogEntries, err := h.listCatalogEntriesInCatalog(req, system.DefaultCatalog, addedCatalogEntries)
	if err != nil {
		return nil, err
	}

	for _, entry := range catalogEntries {
		converted, err := ConvertMCPServerCatalogEntryToRegistry(req.Context(), entry, h.serverURL, reverseDNS)
		if err != nil {
			// If conversion fails, just skip the entry
			continue
		}
		result = append(result, converted)
	}

	// Step 3: List servers in default catalog with access
	catalogServers, credMap, err := h.listServersInCatalog(req, system.DefaultCatalog)
	if err != nil {
		return nil, err
	}

	for _, server := range catalogServers {
		// Get slug for catalog server (no userID since it's catalog-scoped)
		slug, err := handlers.SlugForMCPServer(req.Context(), req.Storage, server, "", system.DefaultCatalog, "")
		if err != nil {
			// If we failed to get the slug, just skip the server
			continue
		}

		converted, err := ConvertMCPServerToRegistry(req.Context(), server, credMap[server.Name], h.serverURL, slug, reverseDNS, userID)
		if err != nil {
			// If conversion fails, just skip the server
			continue
		}
		result = append(result, converted)
	}

	// Step 4: List catalog entries in PowerUserWorkspaces with access
	workspaceEntries, err := h.listCatalogEntriesInWorkspaces(req, addedCatalogEntries)
	if err != nil {
		return nil, err
	}

	for _, entry := range workspaceEntries {
		converted, err := ConvertMCPServerCatalogEntryToRegistry(req.Context(), entry, h.serverURL, reverseDNS)
		if err != nil {
			// If conversion fails, just skip the entry
			continue
		}
		result = append(result, converted)
	}

	// Step 5: List servers in PowerUserWorkspaces with access
	workspaceServers, credMap, err := h.listServersInWorkspaces(req)
	if err != nil {
		return nil, err
	}

	for _, server := range workspaceServers {
		// Get slug for workspace server
		slug, err := handlers.SlugForMCPServer(req.Context(), req.Storage, server, "", "", server.Spec.PowerUserWorkspaceID)
		if err != nil {
			// If slug generation fails, just skip the server
			continue
		}

		converted, err := ConvertMCPServerToRegistry(req.Context(), server, credMap[server.Name], h.serverURL, slug, reverseDNS, userID)
		if err != nil {
			// If conversion fails, just skip the server
			continue
		}
		result = append(result, converted)
	}

	return result, nil
}

// collectAccessibleServersNoAuth returns only default catalog items with wildcard ACR access
func (h *Handler) collectAccessibleServersNoAuth(req api.Context, reverseDNS string) ([]types.RegistryServerResponse, error) {
	var result []types.RegistryServerResponse

	// Step 1: List catalog entries in default catalog
	var entryList v1.MCPServerCatalogEntryList
	if err := req.Storage.List(req.Context(), &entryList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.mcpCatalogName": system.DefaultCatalog,
		}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list catalog entries: %w", err)
	}

	// Filter for wildcard ACR access
	for _, entry := range entryList.Items {
		hasWildcardAccess, err := h.acrHelper.HasWildcardAccessToMCPServerCatalogEntryInCatalog(
			entry.Name,
			system.DefaultCatalog,
		)
		if err != nil || !hasWildcardAccess {
			continue
		}

		converted, err := ConvertMCPServerCatalogEntryToRegistry(req.Context(), entry, h.serverURL, reverseDNS)
		if err != nil {
			// If conversion fails, just skip the entry
			continue
		}
		result = append(result, converted)
	}

	// Step 2: List multi-user servers in default catalog
	var serverList v1.MCPServerList
	if err := req.Storage.List(req.Context(), &serverList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.mcpCatalogID": system.DefaultCatalog,
		}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list catalog servers: %w", err)
	}

	// Filter for wildcard ACR access and non-templates
	for _, server := range serverList.Items {
		// Skip templates and components
		if server.Spec.Template || server.Spec.CompositeName != "" {
			continue
		}

		hasWildcardAccess, err := h.acrHelper.HasWildcardAccessToMCPServerInCatalog(
			server.Name,
			system.DefaultCatalog,
		)
		if err != nil || !hasWildcardAccess {
			continue
		}

		// Get slug for catalog server (no userID since it's catalog-scoped)
		slug, err := handlers.SlugForMCPServer(req.Context(), req.Storage, server, "", system.DefaultCatalog, "")
		if err != nil {
			// If slug generation fails, just skip the server
			continue
		}

		// Get credentials
		credEnv, _ := h.getCredentialsForServer(req, server, "", system.DefaultCatalog, "")

		converted, err := ConvertMCPServerToRegistry(req.Context(), server, credEnv, h.serverURL, slug, reverseDNS, "")
		if err != nil {
			// If conversion fails, just skip the server
			continue
		}
		result = append(result, converted)
	}

	return result, nil
}

// Helper methods for each step

func (h *Handler) listPersonalServers(req api.Context, userID string) ([]v1.MCPServer, map[string]map[string]string, error) {
	var serverList v1.MCPServerList

	if err := req.Storage.List(req.Context(), &serverList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID":               userID,
			"spec.mcpCatalogID":         "",
			"spec.powerUserWorkspaceID": "",
		}),
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to list personal servers: %w", err)
	}

	// Filter out template and component servers
	var servers []v1.MCPServer
	for _, server := range serverList.Items {
		if !server.Spec.Template && server.Spec.CompositeName == "" {
			servers = append(servers, server)
		}
	}

	// Get credentials for all servers
	credMap, err := h.getCredentialsForServers(req, servers, userID, "", "")
	if err != nil {
		return nil, nil, err
	}

	return servers, credMap, nil
}

func (h *Handler) listCatalogEntriesInCatalog(
	req api.Context,
	catalogID string,
	exclude map[string]bool,
) ([]v1.MCPServerCatalogEntry, error) {
	var entryList v1.MCPServerCatalogEntryList

	if err := req.Storage.List(req.Context(), &entryList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.mcpCatalogName": catalogID,
		}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list catalog entries: %w", err)
	}

	var result []v1.MCPServerCatalogEntry
	for _, entry := range entryList.Items {
		// Skip if already added via user's personal server
		if exclude[entry.Name] {
			continue
		}

		// Check access
		hasAccess, err := h.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(
			req.User,
			entry.Name,
			catalogID,
		)
		if err != nil || !hasAccess {
			continue
		}

		result = append(result, entry)
	}

	return result, nil
}

func (h *Handler) listServersInCatalog(
	req api.Context,
	catalogID string,
) ([]v1.MCPServer, map[string]map[string]string, error) {
	var serverList v1.MCPServerList

	if err := req.Storage.List(req.Context(), &serverList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.mcpCatalogID": catalogID,
		}),
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to list catalog servers: %w", err)
	}

	var result []v1.MCPServer
	for _, server := range serverList.Items {
		// Skip templates and components
		if server.Spec.Template || server.Spec.CompositeName != "" {
			continue
		}

		// Check access
		hasAccess, err := h.acrHelper.UserHasAccessToMCPServerInCatalog(
			req.User,
			server.Name,
			catalogID,
		)
		if err != nil || !hasAccess {
			continue
		}

		result = append(result, server)
	}

	// Get credentials
	credMap, err := h.getCredentialsForServers(req, result, "", catalogID, "")
	if err != nil {
		return nil, nil, err
	}

	return result, credMap, nil
}

func (h *Handler) listCatalogEntriesInWorkspaces(
	req api.Context,
	exclude map[string]bool,
) ([]v1.MCPServerCatalogEntry, error) {
	// List ALL workspaces (not pre-filtered)
	var workspaceList v1.PowerUserWorkspaceList
	if err := req.Storage.List(req.Context(), &workspaceList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	var result []v1.MCPServerCatalogEntry

	// For each workspace, list catalog entries and filter by ACR access
	for _, workspace := range workspaceList.Items {
		var entryList v1.MCPServerCatalogEntryList

		if err := req.Storage.List(req.Context(), &entryList, &kclient.ListOptions{
			Namespace: system.DefaultNamespace,
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.powerUserWorkspaceID": workspace.Name,
			}),
		}); err != nil {
			continue
		}

		for _, entry := range entryList.Items {
			// Skip if already added
			if exclude[entry.Name] {
				continue
			}

			// Check access for this specific entry
			hasAccess, err := h.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(
				req.Context(),
				req.User,
				entry.Name,
				workspace.Name,
			)
			if err != nil || !hasAccess {
				continue
			}

			result = append(result, entry)
		}
	}

	return result, nil
}

func (h *Handler) listServersInWorkspaces(
	req api.Context,
) ([]v1.MCPServer, map[string]map[string]string, error) {
	// List ALL workspaces (not pre-filtered)
	var workspaceList v1.PowerUserWorkspaceList
	if err := req.Storage.List(req.Context(), &workspaceList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	var result []v1.MCPServer

	// For each workspace, list servers and filter by ACR access
	for _, workspace := range workspaceList.Items {
		var serverList v1.MCPServerList

		if err := req.Storage.List(req.Context(), &serverList, &kclient.ListOptions{
			Namespace: system.DefaultNamespace,
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.powerUserWorkspaceID": workspace.Name,
			}),
		}); err != nil {
			continue
		}

		for _, server := range serverList.Items {
			// Skip templates and components
			if server.Spec.Template || server.Spec.CompositeName != "" {
				continue
			}

			// Check access for this specific server
			hasAccess, err := h.acrHelper.UserHasAccessToMCPServerInWorkspace(
				req.User,
				server.Name,
				workspace.Name,
				server.Spec.UserID,
			)
			if err != nil || !hasAccess {
				continue
			}

			result = append(result, server)
		}
	}

	// Get credentials - for workspace servers, we need to pass workspace context
	credMap := make(map[string]map[string]string)
	for _, server := range result {
		credEnv, err := h.getCredentialsForServer(req, server, "", "", server.Spec.PowerUserWorkspaceID)
		if err != nil {
			// Skip if credentials not found
			credMap[server.Name] = make(map[string]string)
			continue
		}
		credMap[server.Name] = credEnv
	}

	return result, credMap, nil
}

// Credential retrieval helpers

func (h *Handler) getCredentialsForServers(
	req api.Context,
	servers []v1.MCPServer,
	userID, catalogID, workspaceID string,
) (map[string]map[string]string, error) {
	if len(servers) == 0 {
		return make(map[string]map[string]string), nil
	}

	// Build credential contexts
	credCtxs := make([]string, 0, len(servers))
	for _, server := range servers {
		ctx := h.buildCredentialContext(server, userID, catalogID, workspaceID)
		credCtxs = append(credCtxs, ctx)
	}

	// List credentials
	creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: credCtxs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list credentials: %w", err)
	}

	// Reveal and build map
	credMap := make(map[string]map[string]string)
	for _, cred := range creds {
		if _, ok := credMap[cred.ToolName]; !ok {
			revealed, err := req.GPTClient.RevealCredential(req.Context(), []string{cred.Context}, cred.ToolName)
			if err != nil {
				// Skip if credential not found
				continue
			}
			credMap[cred.ToolName] = revealed.Env
		}
	}

	return credMap, nil
}

func (h *Handler) getCredentialsForServer(
	req api.Context,
	server v1.MCPServer,
	userID, catalogID, workspaceID string,
) (map[string]string, error) {
	ctx := h.buildCredentialContext(server, userID, catalogID, workspaceID)

	revealed, err := req.GPTClient.RevealCredential(req.Context(), []string{ctx}, server.Name)
	if err != nil {
		// Return empty map if not found
		return make(map[string]string), nil
	}

	return revealed.Env, nil
}

func (h *Handler) buildCredentialContext(
	server v1.MCPServer,
	userID, catalogID, workspaceID string,
) string {
	// Follow pattern from pkg/api/handlers/mcp.go
	if catalogID != "" {
		return fmt.Sprintf("%s-%s", catalogID, server.Name)
	}
	if workspaceID != "" {
		return fmt.Sprintf("%s-%s", workspaceID, server.Name)
	}
	if userID != "" {
		return fmt.Sprintf("%s-%s", userID, server.Name)
	}
	return fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)
}

// Pagination and filtering helpers

func parseLimit(limitStr string) int {
	if limitStr == "" {
		return 50 // Default limit
	}

	var limit int
	_, _ = fmt.Sscanf(limitStr, "%d", &limit)

	if limit <= 0 || limit > 100 {
		return 50
	}

	return limit
}

func filterServersBySearch(servers []types.RegistryServerResponse, search string) []types.RegistryServerResponse {
	search = strings.ToLower(search)
	var result []types.RegistryServerResponse

	for _, server := range servers {
		// Substring match on name, title, or description
		if strings.Contains(strings.ToLower(server.Server.Name), search) ||
			strings.Contains(strings.ToLower(server.Server.Title), search) ||
			strings.Contains(strings.ToLower(server.Server.Description), search) {
			result = append(result, server)
		}
	}

	return result
}

func paginateServers(servers []types.RegistryServerResponse, cursor string, limit int) types.RegistryServerList {
	// Sort servers by creation timestamp
	slices.SortStableFunc(servers, func(i, j types.RegistryServerResponse) int {
		return int(i.CreatedAtUnix - j.CreatedAtUnix)
	})

	// Simple cursor-based pagination using server name as cursor
	startIdx := 0
	if cursor != "" {
		// Find the position after the cursor
		for i, server := range servers {
			if server.Server.Name == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Get the page
	endIdx := startIdx + limit
	if endIdx > len(servers) {
		endIdx = len(servers)
	}

	page := servers[startIdx:endIdx]
	if page == nil {
		// This prevents an error from showing up in VSCode when the result is empty.
		page = []types.RegistryServerResponse{}
	}

	// Build response
	response := types.RegistryServerList{
		Servers: page,
	}

	// Add metadata if there are more results
	if endIdx < len(servers) {
		response.Metadata = &types.RegistryServerListMetadata{
			NextCursor: page[len(page)-1].Server.Name,
			Count:      len(page),
		}
	} else {
		response.Metadata = &types.RegistryServerListMetadata{
			Count: len(page),
		}
	}

	return response
}

// ListServerVersions handles GET /v0.1/servers/{serverName}/versions
func (h *Handler) ListServerVersions(req api.Context) error {
	// Extract server name from path (should be in format: reverseDNS/serverName)
	serverName := req.PathValue("serverName")
	if serverName == "" {
		return fmt.Errorf("serverName is required")
	}

	// Parse reverse DNS and actual server name
	parts := strings.SplitN(serverName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return h.notFoundError("Invalid server name format. Expected: reverseDNS/serverName")
	}
	reverseDNS, actualServerName := parts[0], parts[1]

	// Find the server
	server, err := h.findServerByName(req, actualServerName, reverseDNS)
	if err != nil {
		return h.notFoundError("Server not found")
	}

	// Return as a ServerList with single item (only "latest" version exists)
	response := types.RegistryServerList{
		Servers: []types.RegistryServerResponse{server},
		Metadata: &types.RegistryServerListMetadata{
			Count: 1,
		},
	}

	return req.Write(response)
}

// GetServerVersion handles GET /v0.1/servers/{serverName}/versions/{version}
func (h *Handler) GetServerVersion(req api.Context) error {
	// Extract parameters from path (serverName should be in format: reverseDNS/serverName)
	serverName := req.PathValue("serverName")
	version := req.PathValue("version")

	if serverName == "" {
		return fmt.Errorf("serverName is required")
	}
	if version == "" {
		return fmt.Errorf("version is required")
	}

	// Only support "latest" version
	if version != "latest" {
		return h.notFoundError("Version not found. Only 'latest' is supported.")
	}

	// Parse reverse DNS and actual server name
	parts := strings.SplitN(serverName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return h.notFoundError("Invalid server name format. Expected: reverseDNS/serverName")
	}
	reverseDNS, actualServerName := parts[0], parts[1]

	// Find the server
	server, err := h.findServerByName(req, actualServerName, reverseDNS)
	if err != nil {
		return h.notFoundError("Server not found")
	}

	return req.Write(server)
}

// findServerByName searches for a server by name and checks user access
func (h *Handler) findServerByName(req api.Context, serverName string, reverseDNS string) (types.RegistryServerResponse, error) {
	// Determine type based on prefix
	if system.IsMCPServerID(serverName) {
		// MCPServer - check ownership or ACR permissions
		return h.findMCPServer(req, serverName, reverseDNS)
	}
	// MCPServerCatalogEntry - check ACR permissions
	return h.findMCPServerCatalogEntry(req, serverName, reverseDNS)
}

// findMCPServer looks up an MCPServer and checks ownership or ACR permissions
func (h *Handler) findMCPServer(req api.Context, serverName, reverseDNS string) (types.RegistryServerResponse, error) {
	var server v1.MCPServer
	err := req.Get(&server, serverName)
	if err != nil {
		return types.RegistryServerResponse{}, fmt.Errorf("server not found")
	}

	// Skip templates and components
	if server.Spec.Template || server.Spec.CompositeName != "" {
		return types.RegistryServerResponse{}, fmt.Errorf("server not found")
	}

	// Check access based on server location
	var (
		slug    string
		credEnv map[string]string
	)

	// Get the credentials for the server. We ignore errors if we get any,
	// as we can still convert the server anyway and display it.
	// Worst-case scenario is that a server that is configured will show up without a connect URL.
	if server.Spec.UserID == req.User.GetUID() && server.Spec.MCPCatalogID == "" && server.Spec.PowerUserWorkspaceID == "" {
		// Personal server - user owns it
		slug, err = handlers.SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), "", "")
		if err != nil {
			return types.RegistryServerResponse{}, fmt.Errorf("failed to generate slug")
		}
		credEnv, _ = h.getCredentialsForServer(req, server, req.User.GetUID(), "", "")
	} else if server.Spec.MCPCatalogID != "" {
		// Catalog server - check ACR
		hasAccess, err := h.acrHelper.UserHasAccessToMCPServerInCatalog(
			req.User,
			server.Name,
			server.Spec.MCPCatalogID,
		)
		if err != nil || !hasAccess {
			return types.RegistryServerResponse{}, fmt.Errorf("server not found")
		}
		slug, err = handlers.SlugForMCPServer(req.Context(), req.Storage, server, "", server.Spec.MCPCatalogID, "")
		if err != nil {
			return types.RegistryServerResponse{}, fmt.Errorf("failed to generate slug")
		}
		credEnv, _ = h.getCredentialsForServer(req, server, "", server.Spec.MCPCatalogID, "")
	} else if server.Spec.PowerUserWorkspaceID != "" {
		// Workspace server - check ACR
		hasAccess, err := h.acrHelper.UserHasAccessToMCPServerInWorkspace(
			req.User,
			server.Name,
			server.Spec.PowerUserWorkspaceID,
			server.Spec.UserID,
		)
		if err != nil || !hasAccess {
			return types.RegistryServerResponse{}, fmt.Errorf("server not found")
		}
		slug, err = handlers.SlugForMCPServer(req.Context(), req.Storage, server, "", "", server.Spec.PowerUserWorkspaceID)
		if err != nil {
			return types.RegistryServerResponse{}, fmt.Errorf("failed to generate slug")
		}
		credEnv, _ = h.getCredentialsForServer(req, server, "", "", server.Spec.PowerUserWorkspaceID)
	} else {
		return types.RegistryServerResponse{}, fmt.Errorf("server not found")
	}

	return ConvertMCPServerToRegistry(req.Context(), server, credEnv, h.serverURL, slug, reverseDNS, req.User.GetUID())
}

// findMCPServerCatalogEntry looks up an MCPServerCatalogEntry and checks ACR permissions
func (h *Handler) findMCPServerCatalogEntry(req api.Context, entryName string, reverseDNS string) (types.RegistryServerResponse, error) {
	var entry v1.MCPServerCatalogEntry
	err := req.Get(&entry, entryName)
	if err != nil {
		return types.RegistryServerResponse{}, fmt.Errorf("catalog entry not found")
	}

	// Check access based on entry location
	if entry.Spec.MCPCatalogName != "" {
		// Catalog entry - check ACR
		hasAccess, err := h.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(
			req.User,
			entry.Name,
			entry.Spec.MCPCatalogName,
		)
		if err != nil || !hasAccess {
			return types.RegistryServerResponse{}, fmt.Errorf("catalog entry not found")
		}
	} else if entry.Spec.PowerUserWorkspaceID != "" {
		// Workspace entry - check ACR
		hasAccess, err := h.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(
			req.Context(),
			req.User,
			entry.Name,
			entry.Spec.PowerUserWorkspaceID,
		)
		if err != nil || !hasAccess {
			return types.RegistryServerResponse{}, fmt.Errorf("catalog entry not found")
		}
	} else {
		return types.RegistryServerResponse{}, fmt.Errorf("catalog entry not found")
	}

	return ConvertMCPServerCatalogEntryToRegistry(req.Context(), entry, h.serverURL, reverseDNS)
}

// notFoundError returns a standard 404 error response in the format:
// {"title":"Not Found","status":404,"detail":"<message>"}
func (h *Handler) notFoundError(detail string) error {
	return &types.ErrHTTP{
		Code:    http.StatusNotFound,
		Message: fmt.Sprintf(`{"title":"Not Found","status":404,"detail":"%s"}`, detail),
	}
}
