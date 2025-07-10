package mcpgateway

import (
	"strconv"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

type AuditLogHandler struct{}

func NewAuditLogHandler() *AuditLogHandler {
	return &AuditLogHandler{}
}

// ListAuditLogs handles GET /api/mcp-audit-logs and /api/mcp-audit-logs/{mcp_id}
func (h *AuditLogHandler) ListAuditLogs(req api.Context) error {
	query := req.URL.Query()

	var mcpServerDisplayName, userID, mcpServerCatalogEntryName string
	mcpID := req.PathValue("mcp_id")
	if mcpID == "" {
		mcpID = query.Get("mcp_id")
		// Only look at these query parameters if the MCP ID is not provided in the URL.
		mcpServerDisplayName = query.Get("mcp_server_display_name")
		mcpServerCatalogEntryName = query.Get("mcp_server_catalog_entry_name")
		userID = query.Get("user_id")
	}

	// Parse query parameters
	opts := gateway.MCPAuditLogOptions{
		// Default limit is 100.
		Limit:                     100,
		UserID:                    userID,
		MCPID:                     mcpID,
		MCPServerDisplayName:      mcpServerDisplayName,
		MCPServerCatalogEntryName: mcpServerCatalogEntryName,
		Client:                    query.Get("client"),
		CallType:                  query.Get("call_type"),
		SessionID:                 query.Get("session_id"),
	}

	// Parse time range
	if startTime := query.Get("start_time"); startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			opts.StartTime = t
		}
	}

	if endTime := query.Get("end_time"); endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			opts.EndTime = t
		}
	}

	// Parse pagination
	if limit := query.Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			opts.Limit = l
		}
	}

	if offset := query.Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			opts.Offset = o
		}
	}

	// Get audit logs
	logs, err := req.GatewayClient.GetMCPAuditLogs(req.Context(), opts)
	if err != nil {
		return err
	}

	// Convert to API types
	var result []types.MCPAuditLog
	for _, log := range logs {
		result = append(result, gatewaytypes.ConvertMCPAuditLog(log))
	}

	// Get total count for pagination
	totalCount, err := req.GatewayClient.CountMCPAuditLogs(req.Context(), opts)
	if err != nil {
		return err
	}

	return req.Write(types.MCPAuditLogResponse{
		MCPAuditLogList: types.MCPAuditLogList{
			Items: result,
		},
		Total:  totalCount,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	})
}

// GetUsageStats handles GET /api/mcp-stats and /api/mcp-stats/{mcp_id}
func (h *AuditLogHandler) GetUsageStats(req api.Context) error {
	query := req.URL.Query()

	var mcpServerDisplayName, mcpServerCatalogEntryName string
	mcpID := req.PathValue("mcp_id")
	if mcpID == "" {
		mcpID = query.Get("mcp_id")
		// Only look at these query parameters if the MCP ID is not provided.
		mcpServerDisplayName = query.Get("mcp_server_display_name")
		mcpServerCatalogEntryName = query.Get("mcp_server_catalog_entry_name")
	}

	// Parse query parameters
	opts := gateway.MCPUsageStatsOptions{
		MCPID:                     mcpID,
		MCPServerDisplayName:      mcpServerDisplayName,
		MCPServerCatalogEntryName: mcpServerCatalogEntryName,
	}

	var (
		err        error
		start, end time.Time
	)
	if startTime := query.Get("start_time"); startTime != "" {
		start, err = time.Parse(time.RFC3339, startTime)
		if err != nil {
			return types.NewErrBadRequest("invalid start_time format, expected RFC3339")
		}
	} else {
		// Default to last 24 hours
		start = time.Now().Add(-24 * time.Hour)
	}

	if endTime := query.Get("end_time"); endTime != "" {
		end, err = time.Parse(time.RFC3339, endTime)
		if err != nil {
			return types.NewErrBadRequest("invalid end_time format, expected RFC3339")
		}
	} else {
		end = time.Now()
	}

	opts.StartTime = start
	opts.EndTime = end

	// Get usage stats
	stats, err := req.GatewayClient.GetMCPUsageStats(req.Context(), opts)
	if err != nil {
		return err
	}

	// Convert to API types
	var result []types.MCPUsageStats
	for _, stat := range stats {
		result = append(result, gatewaytypes.ConvertMCPUsageStats(stat))
	}

	return req.Write(result)
}
