package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/jsonschema"
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
	Type                  string `json:"type"` // "catalog_entry" or "multi_user_server"
	RequiresConfiguration bool   `json:"requires_configuration"`
	NeedsURL              bool   `json:"needs_url,omitempty"`
	DeploymentStatus      string `json:"deployment_status,omitempty"`
}

type listResult struct {
	CatalogEntries   []serverInfo `json:"catalog_entries"`
	MultiUserServers []serverInfo `json:"multi_user_servers"`
	TotalCount       int          `json:"total_count"`
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
			Description: "List all available MCP servers in Obot that you have access to. Returns both catalog entries (server templates) and multi-user servers (shared instances).",
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

func (s *Server) handleListMCPServers(ctx context.Context, _ *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]any]) (*mcp.CallToolResultFor[any], error) {
	user := userFromContext(ctx)
	if user == nil {
		return errorResult("unauthorized: no user in context"), nil
	}

	// Parse arguments
	args := params.Arguments
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

	// Calculate remaining limit for servers
	remainingLimit := 0
	if limit > 0 {
		remainingLimit = limit - len(entries)
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

	// Convert to response format
	result := listResult{
		CatalogEntries:   make([]serverInfo, 0, len(entries)),
		MultiUserServers: make([]serverInfo, 0, len(servers)),
	}

	for _, entry := range entries {
		result.CatalogEntries = append(result.CatalogEntries, catalogEntryToServerInfo(entry))
	}

	for _, server := range servers {
		result.MultiUserServers = append(result.MultiUserServers, serverToServerInfo(server))
	}

	result.TotalCount = len(result.CatalogEntries) + len(result.MultiUserServers)

	return jsonResult(result)
}

func (s *Server) handleSearchMCPServers(ctx context.Context, _ *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]any]) (*mcp.CallToolResultFor[any], error) {
	user := userFromContext(ctx)
	if user == nil {
		return errorResult("unauthorized: no user in context"), nil
	}

	// Parse arguments
	args := params.Arguments
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

	// Apply keyword search
	entries = listing.SearchCatalogEntries(entries, query)

	// List all multi-user servers (no limit) since we need to search through all of them
	servers, err := s.lister.ListServers(ctx, userInfo, isAdmin, 0)
	if err != nil {
		log.Errorf("failed to list servers: %v", err)
		return errorResult("failed to list servers"), nil
	}

	// Apply keyword search
	servers = listing.SearchServers(servers, query)

	// Convert to response format, applying limit after search
	result := listResult{
		CatalogEntries:   make([]serverInfo, 0, min(len(entries), limit)),
		MultiUserServers: make([]serverInfo, 0, len(servers)),
	}

	// Apply the limit to the combined total of catalog entries and multi-user servers.
	remaining := limit

	for _, entry := range entries {
		if remaining == 0 {
			break
		}
		result.CatalogEntries = append(result.CatalogEntries, catalogEntryToServerInfo(entry))
		remaining--
	}

	for _, server := range servers {
		if remaining == 0 {
			break
		}
		result.MultiUserServers = append(result.MultiUserServers, serverToServerInfo(server))
		remaining--
	}

	result.TotalCount = len(result.CatalogEntries) + len(result.MultiUserServers)

	return jsonResult(result)
}

func (s *Server) handleGetMCPServerConnection(ctx context.Context, _ *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]any]) (*mcp.CallToolResultFor[any], error) {
	user := userFromContext(ctx)
	if user == nil {
		return errorResult("unauthorized: no user in context"), nil
	}

	// Parse arguments
	args := params.Arguments
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
	// - IDs starting with "ms1" are multi-user MCP servers
	// - All other IDs are catalog entries
	if system.IsMCPServerID(serverID) {
		return s.getMultiUserServerConnection(ctx, userInfo, serverID)
	}
	return s.getCatalogEntryConnection(ctx, userInfo, serverID)
}

func (s *Server) getCatalogEntryConnection(ctx context.Context, userInfo *mcpUserInfo, serverID string) (*mcp.CallToolResultFor[any], error) {
	effectiveRole := effectiveRoleFromContext(ctx)
	isAdmin := effectiveRole.HasRole(types.RoleAdmin)

	entry, err := s.lister.GetCatalogEntry(ctx, userInfo, serverID, isAdmin)
	if err != nil {
		log.Errorf("failed to get catalog entry %s: %v", serverID, err)
		return errorResult("failed to get catalog entry"), nil
	}

	result := connectionResult{}

	// Check if this entry requires configuration
	needsConfig := listing.RequiresConfiguration(entry.Spec.Manifest)
	needsURL := listing.RequiresURLConfiguration(entry.Spec.Manifest)

	if needsConfig || needsURL {
		result.Status = "requires_configuration"
		result.NeedsURL = needsURL
		result.ConfigureURL = fmt.Sprintf("%s/mcp-servers/c/%s", s.serverURL, serverID)
		result.Message = "This server requires configuration before it can be used. Please visit the configuration URL to set it up."
	} else {
		result.Status = "ready"
		result.ConnectURL = system.MCPConnectURL(s.serverURL, serverID)
		result.Message = "Server is ready to use. Connect using the provided URL."
	}

	return jsonResult(result)
}

func (s *Server) getMultiUserServerConnection(ctx context.Context, userInfo *mcpUserInfo, serverID string) (*mcp.CallToolResultFor[any], error) {
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

	// Multi-user servers don't have a direct connect URL from here
	// Users need to add them to a project to get a connection
	result.Status = "ready"
	result.ConfigureURL = system.MCPConnectURL(s.serverURL, serverID)
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

func errorResult(msg string) *mcp.CallToolResultFor[any] {
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
		IsError: true,
	}
}

func jsonResult(v any) (*mcp.CallToolResultFor[any], error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil
}
