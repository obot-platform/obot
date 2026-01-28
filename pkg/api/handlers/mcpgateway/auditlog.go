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

// Authorization levels for audit log access, from highest to lowest privilege.
// Each level determines what audit logs a user can see and what details are included.
type auditLogAuthLevel int

const (
	// authLevelAdmin can see all audit logs with full details.
	authLevelAdmin auditLogAuthLevel = iota
	// authLevelAuditor can see all audit logs with full details.
	authLevelAuditor
	// authLevelPowerUser can see audit logs scoped to their workspace.
	authLevelPowerUser
	// authLevelBasicUser can only see audit logs for single-user servers they own.
	authLevelBasicUser
)

// getAuthLevel determines the user's authorization level for audit log access.
func getAuthLevel(req api.Context) auditLogAuthLevel {
	switch {
	case req.UserIsAdmin():
		return authLevelAdmin
	case req.UserIsAuditor():
		return authLevelAuditor
	case req.UserIsPowerUser():
		return authLevelPowerUser
	default:
		return authLevelBasicUser
	}
}

// getOwnedSingleUserServerIDs returns the IDs of single-user MCP servers owned by the user.
// Single-user servers are identified by having no MCPCatalogID set.
// Returns an empty slice if the user owns no single-user servers.
func getOwnedSingleUserServerIDs(req api.Context) ([]string, error) {
	var serverList v1.MCPServerList
	if err := req.List(&serverList, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.userID", req.User.GetUID()),
	}); err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	// Filter to only single-user servers (no MCPCatalogID means not multi-user)
	serverIDs := make([]string, 0, len(serverList.Items))
	for _, server := range serverList.Items {
		if server.Spec.MCPCatalogID == "" {
			serverIDs = append(serverIDs, server.Name)
		}
	}
	return serverIDs, nil
}

// filterToOwnedServers filters the requested MCP IDs to only include servers the user owns.
// If requestedIDs is empty, returns all owned server IDs.
// If requestedIDs is non-empty, returns the intersection with owned server IDs.
func filterToOwnedServers(requestedIDs, ownedServerIDs []string) []string {
	if len(requestedIDs) == 0 {
		return ownedServerIDs
	}

	filtered := make([]string, 0, len(requestedIDs))
	for _, id := range requestedIDs {
		if slices.Contains(ownedServerIDs, id) {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

// emptyAuditLogResponse returns an empty audit log response with the given pagination.
func emptyAuditLogResponse(limit, offset int) types.MCPAuditLogResponse {
	return types.MCPAuditLogResponse{
		MCPAuditLogList: types.MCPAuditLogList{Items: []types.MCPAuditLog{}},
		Total:           0,
		Limit:           limit,
		Offset:          offset,
	}
}

// emptyUsageStatsResponse returns an empty usage stats response.
func emptyUsageStatsResponse() types.MCPUsageStats {
	return types.MCPUsageStats{
		TimeStart:   *types.NewTime(time.Now()),
		TimeEnd:     *types.NewTime(time.Now()),
		TotalCalls:  0,
		UniqueUsers: 0,
		Items:       []types.MCPUsageStatItem{},
	}
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

	// Parse query parameters with support for multiple values.
	// Any filters here must also be available in ListAuditLogFilterOptions.
	opts := gateway.MCPAuditLogOptions{
		WithRequestAndResponse:    false, // Exclude bodies from list responses for performance
		Limit:                     100,   // Default limit
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

	// Path parameter takes precedence over query parameter
	if pathMcpID := req.PathValue("mcp_id"); pathMcpID != "" {
		opts.MCPID = []string{pathMcpID}
	}

	// Apply authorization-based filtering
	authLevel := getAuthLevel(req)
	switch authLevel {
	case authLevelAdmin, authLevelAuditor:
		// Full access - no filtering needed

	case authLevelPowerUser:
		// Scope to their workspace
		opts.PowerUserWorkspaceID = []string{system.GetPowerUserWorkspaceID(req.User.GetUID())}

	case authLevelBasicUser:
		// Basic users can only see audit logs for their own single-user servers
		ownedServerIDs, err := getOwnedSingleUserServerIDs(req)
		if err != nil {
			return err
		}
		if len(ownedServerIDs) == 0 {
			return req.Write(emptyAuditLogResponse(opts.Limit, opts.Offset))
		}

		// Filter to only owned servers
		opts.MCPID = filterToOwnedServers(opts.MCPID, ownedServerIDs)
		if len(opts.MCPID) == 0 {
			return req.Write(emptyAuditLogResponse(opts.Limit, opts.Offset))
		}
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
	id := req.PathValue("audit_log_id")
	if id == "" {
		return types.NewErrBadRequest("missing audit log id")
	}

	var auditLogID uint64
	if _, err := fmt.Sscanf(id, "%d", &auditLogID); err != nil {
		return types.NewErrBadRequest("invalid audit log id: %v", err)
	}

	// Determine access level and whether to include full payload
	authLevel := getAuthLevel(req)
	includeFullPayload := authLevel == authLevelAdmin || authLevel == authLevelAuditor

	// For non-privileged users, verify access to this specific audit log
	if authLevel == authLevelPowerUser || authLevel == authLevelBasicUser {
		logMeta, err := req.GatewayClient.GetMCPAuditLog(req.Context(), uint(auditLogID), false)
		if err != nil {
			return err
		}

		if logMeta.MCPID != "" {
			var server v1.MCPServer
			if err := req.Get(&server, logMeta.MCPID); err == nil {
				userOwnsServer := server.Spec.UserID == req.User.GetUID()
				isSingleUserServer := server.Spec.MCPCatalogID == ""

				switch authLevel {
				case authLevelPowerUser:
					// Power users can access logs for servers in their workspace
					// Full payload only for single-user servers they own
					if userOwnsServer && isSingleUserServer {
						includeFullPayload = true
					}

				case authLevelBasicUser:
					// Basic users can only access single-user servers they own
					if !userOwnsServer || !isSingleUserServer {
						return types.NewErrNotFound("audit log not found")
					}
					includeFullPayload = true
				}
			} else if authLevel == authLevelBasicUser {
				// Server not found - basic users are denied
				return types.NewErrNotFound("audit log not found")
			}
		}
	}

	log, err := req.GatewayClient.GetMCPAuditLog(req.Context(), uint(auditLogID), includeFullPayload)
	if err != nil {
		return err
	}

	return req.Write(gatewaytypes.ConvertMCPAuditLog(*log))
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

// ListAuditLogFilterOptions handles GET /api/mcp-audit-logs/filter-options/{filter}
func (h *AuditLogHandler) ListAuditLogFilterOptions(req api.Context) error {
	filter := req.PathValue("filter")
	if filter == "" {
		return types.NewErrBadRequest("missing option")
	}

	query := req.URL.Query()
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

	// Apply authorization-based filtering (same as ListAuditLogs)
	authLevel := getAuthLevel(req)
	switch authLevel {
	case authLevelAdmin, authLevelAuditor:
		// Full access - no filtering needed

	case authLevelPowerUser:
		// Scope to their workspace
		opts.PowerUserWorkspaceID = []string{system.GetPowerUserWorkspaceID(req.User.GetUID())}

	case authLevelBasicUser:
		// Basic users can only see filter options for their own single-user servers
		ownedServerIDs, err := getOwnedSingleUserServerIDs(req)
		if err != nil {
			return err
		}
		if len(ownedServerIDs) == 0 {
			return req.Write(map[string]any{"options": []string{}})
		}

		opts.MCPID = filterToOwnedServers(opts.MCPID, ownedServerIDs)
		if len(opts.MCPID) == 0 {
			return req.Write(map[string]any{"options": []string{}})
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

	// Parse MCP ID from path or query
	mcpID := req.PathValue("mcp_id")
	if mcpID == "" {
		mcpID = query.Get("mcp_id")
	}

	// Additional filters only apply when no specific MCP ID is provided
	var mcpServerDisplayNames, mcpServerCatalogEntryNames, userIDs []string
	if mcpID == "" {
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

	// Apply authorization-based filtering
	authLevel := getAuthLevel(req)
	switch authLevel {
	case authLevelAdmin, authLevelAuditor:
		// Full access - no filtering needed

	case authLevelPowerUser:
		// Scope to their workspace
		opts.PowerUserWorkspaceID = []string{system.GetPowerUserWorkspaceID(req.User.GetUID())}

	case authLevelBasicUser:
		// Basic users can only see usage stats for their own single-user servers
		ownedServerIDs, err := getOwnedSingleUserServerIDs(req)
		if err != nil {
			return err
		}
		if len(ownedServerIDs) == 0 {
			return req.Write(emptyUsageStatsResponse())
		}

		// If a specific server was requested, verify ownership
		if opts.MCPID != "" {
			if !slices.Contains(ownedServerIDs, opts.MCPID) {
				return req.Write(emptyUsageStatsResponse())
			}
		} else {
			// For aggregate queries, constrain to user's own activity only
			// This prevents seeing other users' activity on shared servers
			opts.UserIDs = []string{req.User.GetUID()}
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
