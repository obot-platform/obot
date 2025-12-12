package registry

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ACRHandler handles the per-ACR registry API endpoints
type ACRHandler struct {
	acrHelper      *accesscontrolrule.Helper
	serverURL      string
	registryNoAuth bool
	mimeFetcher    *mimeFetcher
}

func NewACRHandler(acrHelper *accesscontrolrule.Helper, serverURL string, registryNoAuth bool) *ACRHandler {
	return &ACRHandler{
		acrHelper:      acrHelper,
		serverURL:      serverURL,
		registryNoAuth: registryNoAuth,
		mimeFetcher:    newMimeFetcher(),
	}
}

// ListServers handles GET /mcp-registry/{acr_id}/v0.1/servers
func (h *ACRHandler) ListServers(req api.Context) error {
	acrID := req.PathValue("acr_id")
	if acrID == "" {
		return h.notFoundError("access control rule ID is required")
	}

	// Fetch the ACR
	var acr v1.AccessControlRule
	if err := req.Get(&acr, acrID); err != nil {
		return h.notFoundError("access control rule not found")
	}

	// Check authorization
	if !h.isAuthorized(req, acr) {
		return h.notFoundError("access control rule not found")
	}

	// Parse query parameters
	cursor := req.URL.Query().Get("cursor")
	limit := parseLimit(req.URL.Query().Get("limit"))
	search := req.URL.Query().Get("search")

	reverseDNS, err := ReverseDNSFromURL(h.serverURL)
	if err != nil {
		return fmt.Errorf("failed to generate reverse DNS: %w", err)
	}

	// Collect servers based on ACR resources
	servers, err := h.collectServersFromACR(req, acr, reverseDNS)
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

// isAuthorized checks if the request is authorized to access this ACR's registry
func (h *ACRHandler) isAuthorized(req api.Context, acr v1.AccessControlRule) bool {
	if h.registryNoAuth {
		// When auth is OFF, only allow if ACR has wildcard subject
		return h.hasWildcardSubject(acr)
	}

	// When auth is ON, check if user is targeted by ACR subjects
	return h.userMatchesSubjects(req, acr)
}

// hasWildcardSubject checks if the ACR targets all users via wildcard
func (h *ACRHandler) hasWildcardSubject(acr v1.AccessControlRule) bool {
	for _, subject := range acr.Spec.Manifest.Subjects {
		if subject.Type == types.SubjectTypeSelector && subject.ID == "*" {
			return true
		}
	}
	return false
}

// userMatchesSubjects checks if the current user matches any of the ACR's subjects
func (h *ACRHandler) userMatchesSubjects(req api.Context, acr v1.AccessControlRule) bool {
	userID := req.User.GetUID()
	groups := authGroupSet(req.User)

	for _, subject := range acr.Spec.Manifest.Subjects {
		switch subject.Type {
		case types.SubjectTypeUser:
			if subject.ID == userID {
				return true
			}
		case types.SubjectTypeGroup:
			if _, ok := groups[subject.ID]; ok {
				return true
			}
		case types.SubjectTypeSelector:
			if subject.ID == "*" {
				return true
			}
		}
	}
	return false
}

// collectServersFromACR collects servers/entries based on ACR resources
func (h *ACRHandler) collectServersFromACR(req api.Context, acr v1.AccessControlRule, reverseDNS string) ([]types.RegistryServerResponse, error) {
	var result []types.RegistryServerResponse
	userID := req.User.GetUID()

	// Track catalog entries that have been "overridden" by deployed servers
	addedCatalogEntries := make(map[string]bool)

	// Determine scope: catalog or workspace
	catalogID := acr.Spec.MCPCatalogID
	workspaceID := acr.Spec.PowerUserWorkspaceID

	// If auth is ON, first check for user's deployed servers from entries in this ACR
	if !h.registryNoAuth {
		deployedServers, err := h.collectDeployedServersFromACREntries(req, acr, reverseDNS, userID)
		if err != nil {
			return nil, err
		}
		result = append(result, deployedServers...)

		// Mark which catalog entries are now "overridden"
		for _, server := range deployedServers {
			// The server name in registry format contains the original entry name
			// We track by the MCPServerCatalogEntryName from the server spec
			addedCatalogEntries[server.Server.Name] = true
		}
	}

	// Process each resource in the ACR
	for _, resource := range acr.Spec.Manifest.Resources {
		switch resource.Type {
		case types.ResourceTypeMCPServerCatalogEntry:
			// Skip if user has already deployed from this entry
			if addedCatalogEntries[resource.ID] {
				continue
			}
			server, err := h.fetchCatalogEntry(req, resource.ID, catalogID, workspaceID, reverseDNS)
			if err != nil {
				// Skip entries that can't be fetched
				continue
			}
			result = append(result, server)

		case types.ResourceTypeMCPServer:
			server, err := h.fetchMCPServer(req, resource.ID, catalogID, workspaceID, reverseDNS, userID)
			if err != nil {
				// Skip servers that can't be fetched
				continue
			}
			result = append(result, server)

		case types.ResourceTypeSelector:
			if resource.ID == "*" {
				// Wildcard: include all servers/entries in the scope
				if catalogID != "" {
					servers, err := h.collectAllFromCatalog(req, catalogID, reverseDNS, userID, addedCatalogEntries)
					if err != nil {
						return nil, err
					}
					result = append(result, servers...)
				} else if workspaceID != "" {
					servers, err := h.collectAllFromWorkspace(req, workspaceID, reverseDNS, userID, addedCatalogEntries)
					if err != nil {
						return nil, err
					}
					result = append(result, servers...)
				}
			}
		}
	}

	return result, nil
}

// collectDeployedServersFromACREntries finds user's deployed servers that came from entries in this ACR
func (h *ACRHandler) collectDeployedServersFromACREntries(req api.Context, acr v1.AccessControlRule, reverseDNS, userID string) ([]types.RegistryServerResponse, error) {
	var result []types.RegistryServerResponse

	// Build set of catalog entry names in this ACR
	entryNames := make(map[string]bool)
	hasWildcard := false
	for _, resource := range acr.Spec.Manifest.Resources {
		if resource.Type == types.ResourceTypeMCPServerCatalogEntry {
			entryNames[resource.ID] = true
		} else if resource.Type == types.ResourceTypeSelector && resource.ID == "*" {
			hasWildcard = true
		}
	}

	// If no entries and no wildcard, nothing to check
	if len(entryNames) == 0 && !hasWildcard {
		return result, nil
	}

	// List user's personal servers
	var serverList v1.MCPServerList
	if err := req.Storage.List(req.Context(), &serverList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID":               userID,
			"spec.mcpCatalogID":         "",
			"spec.powerUserWorkspaceID": "",
		}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list personal servers: %w", err)
	}

	for _, server := range serverList.Items {
		// Skip templates and components
		if server.Spec.Template || server.Spec.CompositeName != "" {
			continue
		}

		// Check if this server was deployed from an entry in this ACR
		entryName := server.Spec.MCPServerCatalogEntryName
		if entryName == "" {
			continue
		}

		// Fetch the catalog entry to verify its scope matches the ACR's scope
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, entryName); err != nil {
			// Entry not found, skip this server
			continue
		}

		// Verify the entry's scope matches the ACR's scope
		if acr.Spec.MCPCatalogID != "" {
			// ACR is catalog-scoped, entry must be in the same catalog
			if entry.Spec.MCPCatalogName != acr.Spec.MCPCatalogID {
				continue
			}
		} else if acr.Spec.PowerUserWorkspaceID != "" {
			// ACR is workspace-scoped, entry must be in the same workspace
			if entry.Spec.PowerUserWorkspaceID != acr.Spec.PowerUserWorkspaceID {
				continue
			}
		} else {
			// ACR has no scope, this shouldn't happen but skip to be safe
			continue
		}

		// Check if entry is in ACR (directly or via wildcard)
		inACR := entryNames[entryName] || hasWildcard
		if !inACR {
			continue
		}

		// Get slug for this server
		slug, err := handlers.SlugForMCPServer(req.Context(), req.Storage, server, userID, "", "")
		if err != nil {
			continue
		}

		// Get credentials
		credEnv, _ := h.getCredentialsForServer(req, server, userID, "", "")

		converted, err := ConvertMCPServerToRegistry(req.Context(), server, credEnv, h.serverURL, slug, reverseDNS, userID, h.mimeFetcher)
		if err != nil {
			continue
		}
		result = append(result, converted)
	}

	return result, nil
}

// fetchCatalogEntry fetches a single catalog entry and converts to registry format
func (h *ACRHandler) fetchCatalogEntry(req api.Context, entryName, catalogID, workspaceID, reverseDNS string) (types.RegistryServerResponse, error) {
	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return types.RegistryServerResponse{}, fmt.Errorf("entry not found")
	}

	// Verify scope matches
	if catalogID != "" && entry.Spec.MCPCatalogName != catalogID {
		return types.RegistryServerResponse{}, fmt.Errorf("entry not in catalog")
	}
	if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
		return types.RegistryServerResponse{}, fmt.Errorf("entry not in workspace")
	}

	return ConvertMCPServerCatalogEntryToRegistry(req.Context(), entry, h.serverURL, reverseDNS, h.mimeFetcher)
}

// fetchMCPServer fetches a single MCP server and converts to registry format
func (h *ACRHandler) fetchMCPServer(req api.Context, serverName, catalogID, workspaceID, reverseDNS, userID string) (types.RegistryServerResponse, error) {
	var server v1.MCPServer
	if err := req.Get(&server, serverName); err != nil {
		return types.RegistryServerResponse{}, fmt.Errorf("server not found")
	}

	// Skip templates and components
	if server.Spec.Template || server.Spec.CompositeName != "" {
		return types.RegistryServerResponse{}, fmt.Errorf("server is template or component")
	}

	// Verify scope matches
	if catalogID != "" && server.Spec.MCPCatalogID != catalogID {
		return types.RegistryServerResponse{}, fmt.Errorf("server not in catalog")
	}
	if workspaceID != "" && server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.RegistryServerResponse{}, fmt.Errorf("server not in workspace")
	}

	// Get slug
	var (
		slug string
		err  error
	)
	if catalogID != "" {
		slug, err = handlers.SlugForMCPServer(req.Context(), req.Storage, server, "", catalogID, "")
	} else if workspaceID != "" {
		slug, err = handlers.SlugForMCPServer(req.Context(), req.Storage, server, "", "", workspaceID)
	} else {
		return types.RegistryServerResponse{}, fmt.Errorf("no scope for server")
	}
	if err != nil {
		return types.RegistryServerResponse{}, fmt.Errorf("failed to generate slug")
	}

	// Get credentials
	credEnv, _ := h.getCredentialsForServer(req, server, "", catalogID, workspaceID)

	return ConvertMCPServerToRegistry(req.Context(), server, credEnv, h.serverURL, slug, reverseDNS, userID, h.mimeFetcher)
}

// collectAllFromCatalog collects all entries and servers from a catalog
func (h *ACRHandler) collectAllFromCatalog(req api.Context, catalogID, reverseDNS, userID string, exclude map[string]bool) ([]types.RegistryServerResponse, error) {
	var result []types.RegistryServerResponse

	// List catalog entries
	var entryList v1.MCPServerCatalogEntryList
	if err := req.Storage.List(req.Context(), &entryList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.mcpCatalogName": catalogID,
		}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list catalog entries: %w", err)
	}

	for _, entry := range entryList.Items {
		if exclude[entry.Name] {
			continue
		}
		converted, err := ConvertMCPServerCatalogEntryToRegistry(req.Context(), entry, h.serverURL, reverseDNS, h.mimeFetcher)
		if err != nil {
			continue
		}
		result = append(result, converted)
	}

	// List servers in catalog
	var serverList v1.MCPServerList
	if err := req.Storage.List(req.Context(), &serverList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.mcpCatalogID": catalogID,
		}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list catalog servers: %w", err)
	}

	for _, server := range serverList.Items {
		if server.Spec.Template || server.Spec.CompositeName != "" {
			continue
		}
		slug, err := handlers.SlugForMCPServer(req.Context(), req.Storage, server, "", catalogID, "")
		if err != nil {
			continue
		}
		credEnv, _ := h.getCredentialsForServer(req, server, "", catalogID, "")
		converted, err := ConvertMCPServerToRegistry(req.Context(), server, credEnv, h.serverURL, slug, reverseDNS, userID, h.mimeFetcher)
		if err != nil {
			continue
		}
		result = append(result, converted)
	}

	return result, nil
}

// collectAllFromWorkspace collects all entries and servers from a workspace
func (h *ACRHandler) collectAllFromWorkspace(req api.Context, workspaceID, reverseDNS, userID string, exclude map[string]bool) ([]types.RegistryServerResponse, error) {
	var result []types.RegistryServerResponse

	// List workspace entries
	var entryList v1.MCPServerCatalogEntryList
	if err := req.Storage.List(req.Context(), &entryList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.powerUserWorkspaceID": workspaceID,
		}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list workspace entries: %w", err)
	}

	for _, entry := range entryList.Items {
		if exclude[entry.Name] {
			continue
		}
		converted, err := ConvertMCPServerCatalogEntryToRegistry(req.Context(), entry, h.serverURL, reverseDNS, h.mimeFetcher)
		if err != nil {
			continue
		}
		result = append(result, converted)
	}

	// List workspace servers
	var serverList v1.MCPServerList
	if err := req.Storage.List(req.Context(), &serverList, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.powerUserWorkspaceID": workspaceID,
		}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list workspace servers: %w", err)
	}

	for _, server := range serverList.Items {
		if server.Spec.Template || server.Spec.CompositeName != "" {
			continue
		}
		slug, err := handlers.SlugForMCPServer(req.Context(), req.Storage, server, "", "", workspaceID)
		if err != nil {
			continue
		}
		credEnv, _ := h.getCredentialsForServer(req, server, "", "", workspaceID)
		converted, err := ConvertMCPServerToRegistry(req.Context(), server, credEnv, h.serverURL, slug, reverseDNS, userID, h.mimeFetcher)
		if err != nil {
			continue
		}
		result = append(result, converted)
	}

	return result, nil
}

// getCredentialsForServer retrieves credentials for a server
func (h *ACRHandler) getCredentialsForServer(req api.Context, server v1.MCPServer, userID, catalogID, workspaceID string) (map[string]string, error) {
	var ctx string
	if catalogID != "" {
		ctx = fmt.Sprintf("%s-%s", catalogID, server.Name)
	} else if workspaceID != "" {
		ctx = fmt.Sprintf("%s-%s", workspaceID, server.Name)
	} else if userID != "" {
		ctx = fmt.Sprintf("%s-%s", userID, server.Name)
	} else {
		ctx = fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)
	}

	revealed, err := req.GPTClient.RevealCredential(req.Context(), []string{ctx}, server.Name)
	if err != nil {
		return make(map[string]string), nil
	}
	return revealed.Env, nil
}

// ListServerVersions handles GET /mcp-registry/{acr_id}/v0.1/servers/{serverName}/versions
func (h *ACRHandler) ListServerVersions(req api.Context) error {
	acrID := req.PathValue("acr_id")
	serverName := req.PathValue("serverName")

	if acrID == "" {
		return h.notFoundError("access control rule ID is required")
	}
	if serverName == "" {
		return h.notFoundError("serverName is required")
	}

	// Fetch the ACR
	var acr v1.AccessControlRule
	if err := req.Get(&acr, acrID); err != nil {
		return h.notFoundError("access control rule not found")
	}

	// Check authorization
	if !h.isAuthorized(req, acr) {
		return h.notFoundError("access control rule not found")
	}

	// Parse reverse DNS and actual server name
	parts := strings.SplitN(serverName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return h.notFoundError("Invalid server name format. Expected: reverseDNS/serverName")
	}
	reverseDNS, actualServerName := parts[0], parts[1]

	// Find the server within this ACR's scope
	server, err := h.findServerInACR(req, acr, actualServerName, reverseDNS)
	if err != nil {
		return h.notFoundError("Server not found")
	}

	// Return as a ServerList with single item
	response := types.RegistryServerList{
		Servers: []types.RegistryServerResponse{server},
		Metadata: &types.RegistryServerListMetadata{
			Count: 1,
		},
	}

	return req.Write(response)
}

// GetServerVersion handles GET /mcp-registry/{acr_id}/v0.1/servers/{serverName}/versions/{version}
func (h *ACRHandler) GetServerVersion(req api.Context) error {
	acrID := req.PathValue("acr_id")
	serverName := req.PathValue("serverName")
	version := req.PathValue("version")

	if acrID == "" {
		return h.notFoundError("access control rule ID is required")
	}
	if serverName == "" {
		return h.notFoundError("serverName is required")
	}
	if version == "" {
		return h.notFoundError("version is required")
	}

	// Only support "latest" version
	if version != "latest" {
		return h.notFoundError("Version not found. Only 'latest' is supported.")
	}

	// Fetch the ACR
	var acr v1.AccessControlRule
	if err := req.Get(&acr, acrID); err != nil {
		return h.notFoundError("access control rule not found")
	}

	// Check authorization
	if !h.isAuthorized(req, acr) {
		return h.notFoundError("access control rule not found")
	}

	// Parse reverse DNS and actual server name
	parts := strings.SplitN(serverName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return h.notFoundError("Invalid server name format. Expected: reverseDNS/serverName")
	}
	reverseDNS, actualServerName := parts[0], parts[1]

	// Find the server within this ACR's scope
	server, err := h.findServerInACR(req, acr, actualServerName, reverseDNS)
	if err != nil {
		return h.notFoundError("Server not found")
	}

	return req.Write(server)
}

// findServerInACR finds a specific server within the ACR's resource scope
func (h *ACRHandler) findServerInACR(req api.Context, acr v1.AccessControlRule, serverName, reverseDNS string) (types.RegistryServerResponse, error) {
	userID := req.User.GetUID()
	catalogID := acr.Spec.MCPCatalogID
	workspaceID := acr.Spec.PowerUserWorkspaceID

	// Check if this server is in the ACR's resources
	if !h.serverInACRResources(acr, serverName) {
		return types.RegistryServerResponse{}, fmt.Errorf("server not in ACR")
	}

	// Determine if this is an MCPServer or MCPServerCatalogEntry
	if system.IsMCPServerID(serverName) {
		return h.fetchMCPServer(req, serverName, catalogID, workspaceID, reverseDNS, userID)
	}
	return h.fetchCatalogEntry(req, serverName, catalogID, workspaceID, reverseDNS)
}

// serverInACRResources checks if a server/entry is included in the ACR's resources
func (h *ACRHandler) serverInACRResources(acr v1.AccessControlRule, serverName string) bool {
	for _, resource := range acr.Spec.Manifest.Resources {
		switch resource.Type {
		case types.ResourceTypeMCPServer, types.ResourceTypeMCPServerCatalogEntry:
			if resource.ID == serverName {
				return true
			}
		case types.ResourceTypeSelector:
			if resource.ID == "*" {
				return true
			}
		}
	}
	return false
}

// notFoundError returns a standard 404 error
func (h *ACRHandler) notFoundError(detail string) error {
	return &types.ErrHTTP{
		Code:    http.StatusNotFound,
		Message: fmt.Sprintf(`{"title":"Not Found","status":404,"detail":"%s"}`, detail),
	}
}

// authGroupSet extracts auth groups from user info (reuse from helper.go)
func authGroupSet(user interface{ GetExtra() map[string][]string }) map[string]struct{} {
	extra := user.GetExtra()
	groups := extra["auth_provider_groups"]
	set := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		set[group] = struct{}{}
	}
	return set
}
