package mcpgateway

import (
	"sort"
	"strconv"
	"strings"
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

// parseMultiValueParam parses query parameters that can have multiple values
// Supports both comma-separated values in single parameter and repeated parameters
func parseMultiValueParam(queryValues map[string][]string, key string) []string {
	values := queryValues[key]
	if len(values) == 0 {
		return nil
	}

	var result []string
	for _, value := range values {
		if value == "" {
			continue
		}
		// Split by comma to support comma-separated values
		for part := range strings.SplitSeq(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// ListAuditLogs handles GET /api/mcp-audit-logs and /api/mcp-audit-logs/{mcp_id}
func (h *AuditLogHandler) ListAuditLogs(req api.Context) error {
	query := req.URL.Query()

	// Parse query parameters with support for multiple values
	opts := gateway.MCPAuditLogOptions{
		WithRequestAndResponse: req.UserIsAuditor(),
		// Default limit is 100.
		Limit: 100,
		// Any of these filters that can be passed via query parameter need to be available in the "filter options" API.
		// In order for that to be the case, the map in the GetAuditLogFilterOptions method should be updated.
		UserID:                    parseMultiValueParam(query, "user_id"),
		MCPID:                     parseMultiValueParam(query, "mcp_id"),
		MCPServerDisplayName:      parseMultiValueParam(query, "mcp_server_display_name"),
		MCPServerCatalogEntryName: parseMultiValueParam(query, "mcp_server_catalog_entry_name"),
		CallType:                  parseMultiValueParam(query, "call_type"),
		CallIdentifier:            parseMultiValueParam(query, "call_identifier"),
		SessionID:                 parseMultiValueParam(query, "session_id"),
		ClientName:                parseMultiValueParam(query, "client_name"),
		ClientVersion:             parseMultiValueParam(query, "client_version"),
		ResponseStatus:            parseMultiValueParam(query, "response_status"),
		ClientIP:                  parseMultiValueParam(query, "client_ip"),
		Query:                     strings.TrimSpace(query.Get("query")),
	}

	// Handle path parameter for mcp_id (takes precedence over query parameter)
	if pathMcpID := req.PathValue("mcp_id"); pathMcpID != "" {
		opts.MCPID = []string{pathMcpID}
	}

	// Parse processing time range
	if processingTimeMin := query.Get("processing_time_min"); processingTimeMin != "" {
		if minVal, err := strconv.ParseInt(processingTimeMin, 10, 64); err == nil && minVal >= 0 {
			opts.ProcessingTimeMin = minVal
		}
	}

	if processingTimeMax := query.Get("processing_time_max"); processingTimeMax != "" {
		if maxVal, err := strconv.ParseInt(processingTimeMax, 10, 64); err == nil && maxVal >= 0 {
			opts.ProcessingTimeMax = maxVal
		}
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

	// Parse sorting parameters
	opts.SortBy = query.Get("sort_by")
	opts.SortOrder = query.Get("sort_order")

	// Get audit logs
	logs, total, err := req.GatewayClient.GetMCPAuditLogs(req.Context(), opts)
	if err != nil {
		return err
	}

	// Convert to API types
	result := make([]types.MCPAuditLog, 0, len(logs))
	for _, log := range logs {
		result = append(result, gatewaytypes.ConvertMCPAuditLog(log))
	}

	return req.Write(types.MCPAuditLogResponse{
		MCPAuditLogList: types.MCPAuditLogList{
			Items: result,
		},
		Total:  total,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	})
}

// filterOptions represent the values that a user can use to filter audit logs.
// The values of this map represent the "zero" values that are excluded when looking for options in the database.
// For example, "" for strings and 0 for numbers.
var filterOptions = map[string]any{
	"user_id":                       "",
	"mcp_id":                        "",
	"mcp_server_display_name":       "",
	"mcp_server_catalog_entry_name": "",
	"call_type":                     "",
	"call_identifier":               "",
	"session_id":                    "",
	"client_name":                   "",
	"client_version":                "",
	"response_status":               0,
	"client_ip":                     "",
}

// defaultFilterOptions will always be present of the given filter, regardless of what is in the database.
var defaultFilterOptions = map[string][]string{
	"call_type": {"prompts/list", "resources/read", "tools/list", "tools/call", "prompts/get", "resources/list"},
}

func (h *AuditLogHandler) ListAuditLogFilterOptions(req api.Context) error {
	filter := req.PathValue("filter")
	if filter == "" {
		return types.NewErrBadRequest("missing option")
	}

	query := req.URL.Query()
	// Parse field filters (same as ListAuditLogs, excluding sorting)
	opts := gateway.MCPAuditLogOptions{
		UserID:                    parseMultiValueParam(query, "user_id"),
		MCPID:                     parseMultiValueParam(query, "mcp_id"),
		MCPServerDisplayName:      parseMultiValueParam(query, "mcp_server_display_name"),
		MCPServerCatalogEntryName: parseMultiValueParam(query, "mcp_server_catalog_entry_name"),
		CallType:                  parseMultiValueParam(query, "call_type"),
		CallIdentifier:            parseMultiValueParam(query, "call_identifier"),
		SessionID:                 parseMultiValueParam(query, "session_id"),
		ClientName:                parseMultiValueParam(query, "client_name"),
		ClientVersion:             parseMultiValueParam(query, "client_version"),
		ResponseStatus:            parseMultiValueParam(query, "response_status"),
		ClientIP:                  parseMultiValueParam(query, "client_ip"),
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

	exclude, ok := filterOptions[filter]
	if !ok {
		return types.NewErrBadRequest("invalid option: %s", filter)
	}

	options, err := req.GatewayClient.GetAuditLogFilterOptions(req.Context(), filter, opts, exclude)
	if err != nil {
		return err
	}

	if defaultOptions := defaultFilterOptions[filter]; len(defaultOptions) > 0 {
		existingOptions := make(map[string]struct{}, len(options))
		for _, option := range options {
			existingOptions[option] = struct{}{}
		}

		for _, option := range defaultOptions {
			if _, ok := existingOptions[option]; !ok {
				options = append(options, option)
			}
		}
	}

	// Ensure final options are lexicographically sorted after merging defaults
	sort.Strings(options)

	return req.Write(map[string]any{
		"options": options,
	})
}

// GetUsageStats handles GET /api/mcp-stats and /api/mcp-stats/{mcp_id}
func (h *AuditLogHandler) GetUsageStats(req api.Context) error {
	query := req.URL.Query()

	var mcpServerDisplayNames, mcpServerCatalogEntryNames, userIDs []string
	mcpID := req.PathValue("mcp_id")
	if mcpID == "" {
		mcpID = query.Get("mcp_id")
		// Only look at these query parameters if the MCP ID is not provided.
		mcpServerDisplayNames = parseMultiValueParam(query, "mcp_server_display_names")
		mcpServerCatalogEntryNames = parseMultiValueParam(query, "mcp_server_catalog_entry_names")
		userIDs = parseMultiValueParam(query, "user_ids")
	}

	opts := gateway.MCPUsageStatsOptions{
		MCPID:                      mcpID,
		MCPServerDisplayNames:      mcpServerDisplayNames,
		MCPServerCatalogEntryNames: mcpServerCatalogEntryNames,
		UserIDs:                    userIDs,
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
	var result []types.MCPUsageStatItem
	for _, stat := range stats.Items {
		result = append(result, gatewaytypes.ConvertMCPUsageStats(stat))
	}

	return req.Write(types.MCPUsageStats{
		TimeStart:   *types.NewTime(stats.TimeStart),
		TimeEnd:     *types.NewTime(stats.TimeEnd),
		TotalCalls:  stats.TotalCalls,
		UniqueUsers: stats.UniqueUsers,
		Items:       result,
	})
}
