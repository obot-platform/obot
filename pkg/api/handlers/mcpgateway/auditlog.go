package mcpgateway

import (
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AuditLogHandler struct{}

func NewAuditLogHandler() *AuditLogHandler {
	return &AuditLogHandler{}
}

// getOwnServerMCPIDs returns the MCP server IDs for servers that the user owns directly
// (not through a workspace or catalog). These are servers where:
// - Spec.UserID == user's ID
// - Spec.MCPCatalogID == ""
// - Spec.PowerUserWorkspaceID == ""
func getOwnServerMCPIDs(req api.Context) ([]string, error) {
	var mcpServers v1.MCPServerList
	if err := req.List(&mcpServers, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.userID", req.User.GetUID()),
	}); err != nil {
		return nil, err
	}

	var mcpIDs []string
	for _, server := range mcpServers.Items {
		if server.Spec.MCPCatalogID == "" && server.Spec.PowerUserWorkspaceID == "" {
			mcpIDs = append(mcpIDs, server.Name)
		}
	}
	return mcpIDs, nil
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

type auditLogInput struct {
	gatewaytypes.MCPAuditLog `json:",inline"`
	Metadata                 map[string]string `json:"metadata"`
	Subject                  string            `json:"subject"`
}

// SubmitAuditLogs handles POST /api/mcp-audit-logs
// This endpoint is not protected by authentication nor authorization. We have to do it in the handler.
func (h *AuditLogHandler) SubmitAuditLogs(req api.Context) error {
	token := strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return types.NewErrHTTP(http.StatusUnauthorized, "no token provided")
	}

	// Get the MCP server ID from the token
	var mcpServers v1.MCPServerList
	if err := req.List(&mcpServers, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("auditLogTokenHash", hash.Digest(token)),
	}); err != nil {
		return err
	}
	if len(mcpServers.Items) != 1 {
		return types.NewErrHTTP(http.StatusUnauthorized, "invalid token")
	}

	mcpServerName := mcpServers.Items[0].Name

	var auditLogs []auditLogInput
	if err := req.Read(&auditLogs); err != nil {
		return types.NewErrBadRequest("failed to read input: %v", err)
	}

	for _, auditLog := range auditLogs {
		if auditLog.MCPID == "" {
			auditLog.MCPID = auditLog.Metadata["mcpID"]
		}
		if auditLog.MCPID != mcpServerName {
			return types.NewErrForbidden("audit log does not belong to MCP server %q", mcpServerName)
		}
		if auditLog.UserID == "" {
			auditLog.UserID = auditLog.Subject
		}
		if auditLog.MCPServerCatalogEntryName == "" {
			auditLog.MCPServerCatalogEntryName = auditLog.Metadata["mcpServerCatalogEntryName"]
		}
		if auditLog.PowerUserWorkspaceID == "" {
			auditLog.PowerUserWorkspaceID = auditLog.Metadata["powerUserWorkspaceID"]
		}
		if auditLog.MCPServerDisplayName == "" {
			auditLog.MCPServerDisplayName = auditLog.Metadata["mcpServerDisplayName"]
		}

		req.GatewayClient.LogMCPAuditEntry(auditLog.MCPAuditLog)
	}

	return nil
}

// ListAuditLogs handles GET /api/mcp-audit-logs and /api/mcp-audit-logs/{mcp_id}
func (h *AuditLogHandler) ListAuditLogs(req api.Context) error {
	query := req.URL.Query()

	// Parse query parameters with support for multiple values
	opts := gateway.MCPAuditLogOptions{
		// Always exclude request/response bodies from list responses for performance
		WithRequestAndResponse: false,
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

	// Apply scope filtering based on user role
	if !req.UserIsAdmin() && !req.UserIsAuditor() {
		ownServerMCPIDs, err := getOwnServerMCPIDs(req)
		if err != nil {
			return fmt.Errorf("failed to get own server MCPIDs: %w", err)
		}
		opts.OwnServerMCPIDs = ownServerMCPIDs

		// PowerUsers also see workspace servers
		if req.UserIsPowerUser() {
			workspaceID := system.GetPowerUserWorkspaceID(req.User.GetUID())
			opts.PowerUserWorkspaceID = []string{workspaceID}
		}

		// Return empty if no access scope
		if len(opts.OwnServerMCPIDs) == 0 && len(opts.PowerUserWorkspaceID) == 0 {
			return req.Write(types.MCPAuditLogResponse{
				MCPAuditLogList: types.MCPAuditLogList{Items: []types.MCPAuditLog{}},
				Total:           0,
				Limit:           opts.Limit,
				Offset:          opts.Offset,
			})
		}
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

// GetAuditLog handles GET /api/mcp-audit-logs/detail/{audit_log_id}
func (h *AuditLogHandler) GetAuditLog(req api.Context) error {
	// Parse ID from path
	id := req.PathValue("audit_log_id")
	if id == "" {
		return types.NewErrBadRequest("missing audit log id")
	}

	// Convert ID to uint
	var auditLogID uint64
	if _, err := fmt.Sscanf(id, "%d", &auditLogID); err != nil {
		return types.NewErrBadRequest("invalid audit log id: %v", err)
	}

	// Fetch metadata first to check authorization
	log, err := req.GatewayClient.GetMCPAuditLog(req.Context(), uint(auditLogID), false)
	if err != nil {
		return err
	}

	canAccessFullPayload := req.UserIsAuditor()
	if !req.UserIsAuditor() {
		ownServerMCPIDs, err := getOwnServerMCPIDs(req)
		if err != nil {
			return fmt.Errorf("failed to get own server MCPIDs: %w", err)
		}

		isOwnServer := slices.Contains(ownServerMCPIDs, log.MCPID)

		isInWorkspace := false
		if req.UserIsPowerUser() {
			workspaceID := system.GetPowerUserWorkspaceID(req.User.GetUID())
			isInWorkspace = log.PowerUserWorkspaceID == workspaceID
		}

		// Admins can see all logs.
		// For non-admins, it needs to be in the workspace or be their own server to be viewable.
		if !req.UserIsAdmin() && !isOwnServer && !isInWorkspace {
			return types.NewErrForbidden("you do not have access to this audit log")
		}

		// Full payload only for OWN servers (not workspace servers or catalog entry workspace servers)
		canAccessFullPayload = isOwnServer
	}

	// Re-fetch with full payload if authorized
	if canAccessFullPayload {
		log, err = req.GatewayClient.GetMCPAuditLog(req.Context(), uint(auditLogID), true)
		if err != nil {
			return err
		}
	}

	// Convert to API type
	result := gatewaytypes.ConvertMCPAuditLog(*log)

	return req.Write(result)
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

	// Apply scope filtering based on user role
	if !req.UserIsAdmin() && !req.UserIsAuditor() {
		ownServerMCPIDs, err := getOwnServerMCPIDs(req)
		if err != nil {
			return fmt.Errorf("failed to get own server MCPIDs: %w", err)
		}
		opts.OwnServerMCPIDs = ownServerMCPIDs

		// PowerUsers also see workspace servers
		if req.UserIsPowerUser() {
			workspaceID := system.GetPowerUserWorkspaceID(req.User.GetUID())
			opts.PowerUserWorkspaceID = []string{workspaceID}
		}

		// Return empty if no access scope
		if len(opts.OwnServerMCPIDs) == 0 && len(opts.PowerUserWorkspaceID) == 0 {
			return req.Write(map[string]any{
				"options": []string{},
			})
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

	// Apply scope filtering based on user role (same logic as audit logs)
	if !req.UserIsAdmin() && !req.UserIsAuditor() {
		ownServerMCPIDs, err := getOwnServerMCPIDs(req)
		if err != nil {
			return fmt.Errorf("failed to get own server MCPIDs: %w", err)
		}
		opts.OwnServerMCPIDs = ownServerMCPIDs

		// PowerUsers also see workspace servers
		if req.UserIsPowerUser() {
			workspaceID := system.GetPowerUserWorkspaceID(req.User.GetUID())
			opts.PowerUserWorkspaceID = []string{workspaceID}
		}

		// Return empty if no access scope
		if len(opts.OwnServerMCPIDs) == 0 && len(opts.PowerUserWorkspaceID) == 0 {
			return req.Write(types.MCPUsageStats{
				TimeStart: *types.NewTime(time.Now().Add(-24 * time.Hour)),
				TimeEnd:   *types.NewTime(time.Now()),
				Items:     []types.MCPUsageStatItem{},
			})
		}
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
