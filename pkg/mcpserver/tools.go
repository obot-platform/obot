package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp/listing"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// Tool output types
type serverInfo struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Description           string `json:"description"`
	Runtime               string `json:"runtime"`
	Type                  string `json:"type"` // "catalog_entry", "multi_user_server", or "single_user_server"
	RequiresConfiguration bool   `json:"requires_configuration"`
	NeedsURL              bool   `json:"needs_url,omitempty"`
	DeploymentStatus      string `json:"deployment_status,omitempty"`
}

type listResult struct {
	CatalogEntries    []serverInfo `json:"catalog_entries"`
	MultiUserServers  []serverInfo `json:"multi_user_servers"`
	SingleUserServers []serverInfo `json:"single_user_servers"`
	TotalCount        int          `json:"total_count"`
}

type searchResult struct {
	Servers    []serverInfo `json:"servers"`
	TotalCount int          `json:"total_count"`
}

type connectionResult struct {
	Status           string `json:"status"`
	ConnectURL       string `json:"connect_url,omitempty"`
	ConfigureURL     string `json:"configure_url,omitempty"`
	NeedsURL         bool   `json:"needs_url,omitempty"`
	DeploymentStatus string `json:"deployment_status,omitempty"`
	Message          string `json:"message"`
}

// registerTools registers all MCP tools with the server
func (s *Server) registerTools() {
	// List MCP Servers tool
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "obot_list_mcp_servers",
			Description: "List all available MCP servers in Obot that you have access to. Returns catalog entries (server templates), multi-user servers (shared instances), and your single-user servers.",
			InputSchema: listMCPServersSchema(),
		},
		s.handleListMCPServers,
	)

	// Search MCP Servers tool
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "obot_search_mcp_servers",
			Description: "Search for MCP servers by keyword. Searches in server names and descriptions.",
			InputSchema: searchMCPServersSchema(),
		},
		s.handleSearchMCPServers,
	)

	// Get MCP Server Connection tool
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        "obot_get_mcp_server_connection",
			Description: "Get connection information for a specific MCP server. Returns the connection URL if the server is ready, or configuration requirements if setup is needed.",
			InputSchema: getMCPServerConnectionSchema(),
		},
		s.handleGetMCPServerConnection,
	)
}

func listMCPServersSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"limit": {
				Type:        "integer",
				Description: "Maximum number of results to return (default: 50)",
			},
		},
	}
}

func searchMCPServersSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"query": {
				Type:        "string",
				Description: "Search query to match against server names and descriptions",
			},
			"limit": {
				Type:        "integer",
				Description: "Maximum number of results to return (default: 20)",
			},
		},
		Required: []string{"query"},
	}
}

func getMCPServerConnectionSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"server_id": {
				Type:        "string",
				Description: "The ID of the MCP server to get connection info for",
			},
		},
		Required: []string{"server_id"},
	}
}

func (s *Server) handleListMCPServers(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user := userFromContext(ctx)
	if user == nil {
		return errorResult("unauthorized: no user in context"), nil
	}

	// Parse arguments
	var args map[string]any
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return errorResult("invalid arguments: " + err.Error()), nil
		}
	}
	limit := 50
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	if limit > 1000 {
		limit = 1000
	}

	// Create a user.Info wrapper for ACR checks
	effectiveRole := effectiveRoleFromContext(ctx)
	userInfo := &mcpUserInfo{
		user:          user,
		authGroupIDs:  groupIDsFromContext(ctx),
		effectiveRole: effectiveRole,
	}

	// Check if user has admin privileges
	isAdmin := effectiveRole.HasRole(types.RoleAdmin)

	// List catalog entries with limit
	entries, err := s.lister.ListCatalogEntries(ctx, userInfo, isAdmin, limit)
	if err != nil {
		log.Errorf("failed to list catalog entries: %v", err)
		return errorResult("failed to list catalog entries"), nil
	}

	// List single-user servers owned by this user (needed early for filtering catalog entries)
	singleUserServers, err := s.lister.ListSingleUserServers(ctx, userInfo.GetUID(), 0)
	if err != nil {
		log.Errorf("failed to list single-user servers: %v", err)
		return errorResult("failed to list single-user servers"), nil
	}

	// Build set of catalog entries that have single-user instances
	instantiated := instantiatedCatalogEntryNames(singleUserServers)

	// Filter catalog entries before calculating remaining limits so that
	// filtered-out entries don't consume quota from other server types.
	var filteredEntries []v1.MCPServerCatalogEntry
	for _, entry := range entries {
		if missingStaticOAuthCredentials(entry) {
			continue
		}
		if _, exists := instantiated[entry.Name]; exists {
			continue
		}
		filteredEntries = append(filteredEntries, entry)
	}

	// Calculate remaining limit for multi-user servers based on filtered entry count
	remainingLimit := 0
	if limit > 0 {
		remainingLimit = limit - len(filteredEntries)
		if remainingLimit < 0 {
			remainingLimit = 0
		}
	}

	// List multi-user servers with remaining limit
	servers, err := s.lister.ListServers(ctx, userInfo, isAdmin, remainingLimit)
	if err != nil {
		log.Errorf("failed to list servers: %v", err)
		return errorResult("failed to list servers"), nil
	}

	// Trim single-user servers to the remaining limit
	singleUserResultLimit := limit - len(filteredEntries) - len(servers)
	if singleUserResultLimit < 0 {
		singleUserResultLimit = 0
	}
	if singleUserResultLimit < len(singleUserServers) {
		singleUserServers = singleUserServers[:singleUserResultLimit]
	}

	// Convert to response format
	result := listResult{
		CatalogEntries:    make([]serverInfo, 0, len(filteredEntries)),
		MultiUserServers:  make([]serverInfo, 0, len(servers)),
		SingleUserServers: make([]serverInfo, 0, len(singleUserServers)),
	}

	for _, entry := range filteredEntries {
		result.CatalogEntries = append(result.CatalogEntries, catalogEntryToServerInfo(entry))
	}

	for _, server := range servers {
		result.MultiUserServers = append(result.MultiUserServers, serverToServerInfo(server))
	}

	for _, server := range singleUserServers {
		result.SingleUserServers = append(result.SingleUserServers, singleUserServerToServerInfo(server))
	}

	result.TotalCount = len(result.CatalogEntries) + len(result.MultiUserServers) + len(result.SingleUserServers)

	return jsonResult(result)
}

func (s *Server) handleSearchMCPServers(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user := userFromContext(ctx)
	if user == nil {
		return errorResult("unauthorized: no user in context"), nil
	}

	// Parse arguments
	var args map[string]any
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return errorResult("invalid arguments: " + err.Error()), nil
		}
	}
	query, _ := args["query"].(string)
	if query == "" {
		return errorResult("query parameter is required"), nil
	}

	limit := 20
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	if limit > 1000 {
		limit = 1000
	}

	// Create a user.Info wrapper for ACR checks
	effectiveRole := effectiveRoleFromContext(ctx)
	userInfo := &mcpUserInfo{
		user:          user,
		authGroupIDs:  groupIDsFromContext(ctx),
		effectiveRole: effectiveRole,
	}

	// Check if user has admin privileges
	isAdmin := effectiveRole.HasRole(types.RoleAdmin)

	// List all catalog entries (no limit) since we need to search through all of them
	entries, err := s.lister.ListCatalogEntries(ctx, userInfo, isAdmin, 0)
	if err != nil {
		log.Errorf("failed to list catalog entries: %v", err)
		return errorResult("failed to list catalog entries"), nil
	}

	// List all multi-user servers (no limit) since we need to search through all of them
	servers, err := s.lister.ListServers(ctx, userInfo, isAdmin, 0)
	if err != nil {
		log.Errorf("failed to list servers: %v", err)
		return errorResult("failed to list servers"), nil
	}

	// List all single-user servers (no limit) since we need to search through all of them
	singleUserServers, err := s.lister.ListSingleUserServers(ctx, userInfo.GetUID(), 0)
	if err != nil {
		log.Errorf("failed to list single-user servers: %v", err)
		return errorResult("failed to list single-user servers"), nil
	}

	// Build set of catalog entries that have single-user instances
	instantiated := instantiatedCatalogEntryNames(singleUserServers)

	// Collect all matching servers into a single list with match quality info
	type scoredServer struct {
		info        serverInfo
		matchesName bool // true if query matches name, false if only matches description
	}
	var allServers []scoredServer
	queryLower := strings.ToLower(query)

	// Add matching catalog entries
	for _, entry := range entries {
		// Filter out catalog entries missing static OAuth credentials
		if missingStaticOAuthCredentials(entry) {
			continue
		}
		// Filter out catalog entries that have single-user server instances
		if _, exists := instantiated[entry.Name]; exists {
			continue
		}
		info := catalogEntryToServerInfo(entry)
		if matchesQuery(info.Name, info.Description, queryLower) {
			allServers = append(allServers, scoredServer{
				info:        info,
				matchesName: strings.Contains(strings.ToLower(info.Name), queryLower),
			})
		}
	}

	// Add matching multi-user servers
	for _, server := range servers {
		info := serverToServerInfo(server)
		if matchesQuery(info.Name, info.Description, queryLower) {
			allServers = append(allServers, scoredServer{
				info:        info,
				matchesName: strings.Contains(strings.ToLower(info.Name), queryLower),
			})
		}
	}

	// Add matching single-user servers
	for _, server := range singleUserServers {
		info := singleUserServerToServerInfo(server)
		if matchesQuery(info.Name, info.Description, queryLower) {
			allServers = append(allServers, scoredServer{
				info:        info,
				matchesName: strings.Contains(strings.ToLower(info.Name), queryLower),
			})
		}
	}

	// Sort by match quality: name matches first, then description-only matches
	sort.SliceStable(allServers, func(i, j int) bool {
		// Name matches come before description-only matches
		if allServers[i].matchesName != allServers[j].matchesName {
			return allServers[i].matchesName
		}
		return false // preserve original order for same match quality
	})

	// Apply limit
	if len(allServers) > limit {
		allServers = allServers[:limit]
	}

	// Build result
	result := searchResult{
		Servers: make([]serverInfo, 0, len(allServers)),
	}
	for _, s := range allServers {
		result.Servers = append(result.Servers, s.info)
	}
	result.TotalCount = len(result.Servers)

	return jsonResult(result)
}

// matchesQuery returns true if the query matches either the name or description (case-insensitive).
func matchesQuery(name, description, queryLower string) bool {
	return strings.Contains(strings.ToLower(name), queryLower) ||
		strings.Contains(strings.ToLower(description), queryLower)
}

func (s *Server) handleGetMCPServerConnection(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user := userFromContext(ctx)
	if user == nil {
		return errorResult("unauthorized: no user in context"), nil
	}

	// Parse arguments
	var args map[string]any
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return errorResult("invalid arguments: " + err.Error()), nil
		}
	}
	serverID, _ := args["server_id"].(string)

	if serverID == "" {
		return errorResult("server_id parameter is required"), nil
	}

	userInfo := &mcpUserInfo{
		user:          user,
		authGroupIDs:  groupIDsFromContext(ctx),
		effectiveRole: effectiveRoleFromContext(ctx),
	}

	// Determine server type based on ID prefix:
	// - IDs starting with "ms1" are MCP servers (both multi-user and single-user)
	// - All other IDs are catalog entries
	if system.IsMCPServerID(serverID) {
		return s.getMCPServerConnection(ctx, userInfo, serverID)
	}
	return s.getCatalogEntryConnection(ctx, userInfo, serverID)
}

func (s *Server) getCatalogEntryConnection(ctx context.Context, userInfo *mcpUserInfo, serverID string) (*mcp.CallToolResult, error) {
	effectiveRole := effectiveRoleFromContext(ctx)
	isAdmin := effectiveRole.HasRole(types.RoleAdmin)

	entry, err := s.lister.GetCatalogEntry(ctx, userInfo, serverID, isAdmin)
	if err != nil {
		log.Errorf("failed to get catalog entry %s: %v", serverID, err)
		return errorResult("failed to get catalog entry"), nil
	}

	if missingStaticOAuthCredentials(*entry) {
		return nil, fmt.Errorf("this catalog entry requires an admin to configure it before it can be used")
	}

	result := connectionResult{}

	// Check if this entry requires configuration
	needsConfig := listing.RequiresConfiguration(entry.Spec.Manifest)
	needsURL := listing.RequiresURLConfiguration(entry.Spec.Manifest)
	isComposite := entry.Spec.Manifest.Runtime == types.RuntimeComposite

	if needsConfig || needsURL || isComposite {
		result.Status = "requires_configuration"
		result.NeedsURL = needsURL
		result.ConfigureURL = fmt.Sprintf("%s/mcp-servers/c/%s", s.serverURL, serverID)
		if isComposite {
			result.Message = "This is a composite server. Please visit the configuration URL to select which tools to enable."
		} else {
			result.Message = "This server requires configuration before it can be used. Please visit the configuration URL to set it up."
		}
	} else {
		result.Status = "ready"
		result.ConnectURL = system.MCPConnectURL(s.serverURL, serverID)
		result.Message = "Server is ready to use. Connect using the provided URL."
	}

	return jsonResult(result)
}

func (s *Server) getMCPServerConnection(ctx context.Context, userInfo *mcpUserInfo, serverID string) (*mcp.CallToolResult, error) {
	effectiveRole := effectiveRoleFromContext(ctx)
	isAdmin := effectiveRole.HasRole(types.RoleAdmin)

	server, err := s.lister.GetServer(ctx, userInfo, serverID, isAdmin)
	if err != nil {
		log.Errorf("failed to get server %s: %v", serverID, err)
		return errorResult("failed to get server"), nil
	}

	result := connectionResult{}

	// Check deployment status
	if server.Status.DeploymentStatus != "" && server.Status.DeploymentStatus != "Ready" {
		result.Status = "not_ready"
		result.Message = fmt.Sprintf("Server deployment is not ready. Current status: %s", server.Status.DeploymentStatus)
		result.DeploymentStatus = server.Status.DeploymentStatus
		return jsonResult(result)
	}

	result.Status = "ready"
	result.ConnectURL = system.MCPConnectURL(s.serverURL, serverID)
	result.Message = "Server is ready to use. Connect using the provided URL."

	return jsonResult(result)
}

// Helper functions

func catalogEntryToServerInfo(entry v1.MCPServerCatalogEntry) serverInfo {
	return serverInfo{
		ID:                    entry.Name,
		Name:                  entry.Spec.Manifest.Name,
		Description:           entry.Spec.Manifest.Description,
		Runtime:               string(entry.Spec.Manifest.Runtime),
		Type:                  "catalog_entry",
		RequiresConfiguration: listing.RequiresConfiguration(entry.Spec.Manifest),
		NeedsURL:              listing.RequiresURLConfiguration(entry.Spec.Manifest),
	}
}

func serverToServerInfo(server v1.MCPServer) serverInfo {
	return serverInfo{
		ID:               server.Name,
		Name:             server.Spec.Manifest.Name,
		Description:      server.Spec.Manifest.Description,
		Runtime:          string(server.Spec.Manifest.Runtime),
		Type:             "multi_user_server",
		DeploymentStatus: server.Status.DeploymentStatus,
	}
}

func singleUserServerToServerInfo(server v1.MCPServer) serverInfo {
	return serverInfo{
		ID:               server.Name,
		Name:             server.Spec.Manifest.Name,
		Description:      server.Spec.Manifest.Description,
		Runtime:          string(server.Spec.Manifest.Runtime),
		Type:             "single_user_server",
		DeploymentStatus: server.Status.DeploymentStatus,
	}
}

// missingStaticOAuthCredentials returns true if the catalog entry requires static OAuth client credentials that are not yet configured.
// Such entries should be filtered out from the integrated MCP server responses since they require admin configuration before they can be used.
func missingStaticOAuthCredentials(entry v1.MCPServerCatalogEntry) bool {
	return entry.Spec.Manifest.RemoteConfig != nil && entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired && !entry.Status.OAuthCredentialConfigured
}

// instantiatedCatalogEntryNames returns a set of catalog entry names that have
// single-user server instances created from them.
func instantiatedCatalogEntryNames(singleUserServers []v1.MCPServer) map[string]struct{} {
	result := make(map[string]struct{})
	for _, server := range singleUserServers {
		if server.Spec.MCPServerCatalogEntryName != "" {
			result[server.Spec.MCPServerCatalogEntryName] = struct{}{}
		}
	}
	return result
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
		IsError: true,
	}
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil
}
