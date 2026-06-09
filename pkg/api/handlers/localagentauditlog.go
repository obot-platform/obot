package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

const localAgentAuditLogMaxRequestBytes = 2 * 1024 * 1024 // 2 MiB

type LocalAgentAuditLogHandler struct{}

func NewLocalAgentAuditLogHandler() *LocalAgentAuditLogHandler {
	return &LocalAgentAuditLogHandler{}
}

func (h *LocalAgentAuditLogHandler) List(req api.Context) error {
	if !canViewLocalAgentAuditLogMetadata(req) {
		return types.NewErrForbidden("you do not have access to local agent audit logs")
	}

	opts := parseLocalAgentAuditLogOpts(req.URL.Query())
	opts.WithPayloads = false
	if opts.Limit == 0 {
		opts.Limit = 100
	}

	logs, total, err := req.GatewayClient.GetLocalAgentAuditLogs(req.Context(), opts)
	if err != nil {
		return err
	}

	result := make([]types.LocalAgentAuditLog, 0, len(logs))
	for _, log := range logs {
		result = append(result, gatewaytypes.ConvertLocalAgentAuditLog(log))
	}

	return req.Write(types.LocalAgentAuditLogResponse{
		LocalAgentAuditLogList: types.LocalAgentAuditLogList{Items: result},
		Total:                  total,
		Limit:                  opts.Limit,
		Offset:                 opts.Offset,
	})
}

func (h *LocalAgentAuditLogHandler) Detail(req api.Context) error {
	if !canViewLocalAgentAuditLogMetadata(req) {
		return types.NewErrForbidden("you do not have access to this local agent audit log")
	}

	id := req.PathValue("audit_log_id")
	if id == "" {
		return types.NewErrBadRequest("missing audit log id")
	}

	auditLogID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return types.NewErrBadRequest("invalid audit log id: %v", err)
	}

	log, err := req.GatewayClient.GetLocalAgentAuditLog(req.Context(), uint(auditLogID), req.UserIsAuditor())
	if err != nil {
		return err
	}

	return req.Write(gatewaytypes.ConvertLocalAgentAuditLog(*log))
}

func (h *LocalAgentAuditLogHandler) Submit(req api.Context) error {
	if !slices.Contains(req.User.GetGroups(), types.GroupAPIKey) {
		return types.NewErrForbidden("local agent audit ingestion requires API key authentication")
	}
	if !slices.Contains(req.User.GetExtra()[types.APIKeyAuditLogsAppendExtraKey], "true") {
		return types.NewErrForbidden("API key cannot append audit logs")
	}

	body, err := req.Body(api.BodyOptions{MaxBytes: localAgentAuditLogMaxRequestBytes})
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return types.NewErrBadRequest("request body is required")
	}

	inputs, err := decodeLocalAgentAuditLogInputs(body)
	if err != nil {
		return types.NewErrBadRequest("failed to read input: %v", err)
	}
	if len(inputs) == 0 {
		return types.NewErrBadRequest("at least one audit log is required")
	}

	userID := req.User.GetUID()
	clientIP := requestinfo.GetSourceIP(req.Request)
	logs := make([]gatewaytypes.LocalAgentAuditLog, 0, len(inputs))
	for _, input := range inputs {
		auditLog := gatewaytypes.LocalAgentAuditLog{
			EventID:            strings.TrimSpace(input.EventID),
			CreatedAt:          input.CreatedAt.GetTime(),
			UserID:             userID,
			ClientName:         input.Client.Name,
			ClientVersion:      input.Client.Version,
			ClientIP:           clientIP,
			ToolName:           input.ToolName,
			ToolType:           input.ToolType,
			EventName:          strings.TrimSpace(input.EventName),
			Success:            input.Success,
			Status:             input.Status,
			ExitCode:           input.ExitCode,
			DurationMs:         input.DurationMs,
			SessionID:          input.SessionID,
			ConversationID:     input.ConversationID,
			RequestID:          input.RequestID,
			WorkspaceHash:      input.WorkspaceHash,
			WorkspaceBasename:  input.WorkspaceBasename,
			Error:              input.Error,
			PayloadTruncated:   input.PayloadTruncated,
			RawClientHookEvent: input.RawClientHookEvent,
			RawToolInput:       input.RawToolInput,
			RawToolOutput:      input.RawToolOutput,
			RawError:           input.RawError,
		}
		if auditLog.EventID == "" {
			return types.NewErrBadRequest("eventID is required")
		}
		if auditLog.EventName == "" {
			return types.NewErrBadRequest("eventName is required")
		}
		if strings.TrimSpace(auditLog.ClientName) == "" {
			return types.NewErrBadRequest("clientName is required")
		}
		logs = append(logs, auditLog)
	}

	stored, inserted, err := req.GatewayClient.CreateLocalAgentAuditLogs(req.Context(), logs)
	if err != nil {
		return err
	}

	ids := make([]uint, 0, len(stored))
	for _, auditLog := range stored {
		if auditLog.ID != 0 {
			ids = append(ids, auditLog.ID)
		}
	}

	return req.WriteCode(types.LocalAgentAuditLogIngestResponse{
		Accepted:   len(logs),
		Inserted:   inserted,
		Duplicates: len(logs) - inserted,
		IDs:        ids,
	}, http.StatusCreated)
}

var localAgentAuditLogFilterOptions = map[string]any{
	"user_id":            "",
	"event_id":           "",
	"client_name":        "",
	"client_version":     "",
	"client_ip":          "",
	"tool_name":          "",
	"tool_type":          "",
	"event_name":         "",
	"success":            nil,
	"status":             "",
	"exit_code":          nil,
	"session_id":         "",
	"conversation_id":    "",
	"request_id":         "",
	"workspace_hash":     "",
	"workspace_basename": "",
	"payload_truncated":  nil,
}

func (h *LocalAgentAuditLogHandler) FilterOptions(req api.Context) error {
	if !canViewLocalAgentAuditLogMetadata(req) {
		return types.NewErrForbidden("you do not have access to local agent audit logs")
	}

	filter := req.PathValue("filter")
	if filter == "" {
		return types.NewErrBadRequest("missing option")
	}

	exclude, ok := localAgentAuditLogFilterOptions[filter]
	if !ok {
		return types.NewErrBadRequest("invalid option: %s", filter)
	}

	opts := parseLocalAgentAuditLogOpts(req.URL.Query())
	var options []string
	var err error
	if exclude == nil {
		options, err = req.GatewayClient.GetLocalAgentAuditLogFilterOptions(req.Context(), filter, opts)
	} else {
		options, err = req.GatewayClient.GetLocalAgentAuditLogFilterOptions(req.Context(), filter, opts, exclude)
	}
	if err != nil {
		return err
	}
	sort.Strings(options)

	return req.Write(map[string]any{
		"options": options,
	})
}

func decodeLocalAgentAuditLogInputs(body []byte) ([]types.LocalAgentAuditLogIngest, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var inputs []types.LocalAgentAuditLogIngest
		if err := json.Unmarshal(trimmed, &inputs); err != nil {
			return nil, err
		}
		return inputs, nil
	}

	var input types.LocalAgentAuditLogIngest
	if err := json.Unmarshal(trimmed, &input); err != nil {
		return nil, err
	}
	return []types.LocalAgentAuditLogIngest{input}, nil
}

func canViewLocalAgentAuditLogMetadata(req api.Context) bool {
	return req.UserIsAdmin() || req.UserIsOwner() || req.UserIsAuditor()
}

func parseLocalAgentAuditLogOpts(query url.Values) gateway.LocalAgentAuditLogOptions {
	opts := gateway.LocalAgentAuditLogOptions{
		UserID:            parseLocalAgentAuditLogMultiValueParam(query, "user_id"),
		EventID:           parseLocalAgentAuditLogMultiValueParam(query, "event_id"),
		ClientName:        parseLocalAgentAuditLogMultiValueParam(query, "client_name"),
		ClientVersion:     parseLocalAgentAuditLogMultiValueParam(query, "client_version"),
		ClientIP:          parseLocalAgentAuditLogMultiValueParam(query, "client_ip"),
		ToolName:          parseLocalAgentAuditLogMultiValueParam(query, "tool_name"),
		ToolType:          parseLocalAgentAuditLogMultiValueParam(query, "tool_type"),
		EventName:         parseLocalAgentAuditLogMultiValueParam(query, "event_name"),
		Success:           parseLocalAgentAuditLogBoolParam(query, "success"),
		Status:            parseLocalAgentAuditLogMultiValueParam(query, "status"),
		ExitCode:          parseLocalAgentAuditLogIntParam(query, "exit_code"),
		SessionID:         parseLocalAgentAuditLogMultiValueParam(query, "session_id"),
		ConversationID:    parseLocalAgentAuditLogMultiValueParam(query, "conversation_id"),
		RequestID:         parseLocalAgentAuditLogMultiValueParam(query, "request_id"),
		WorkspaceHash:     parseLocalAgentAuditLogMultiValueParam(query, "workspace_hash"),
		WorkspaceBasename: parseLocalAgentAuditLogMultiValueParam(query, "workspace_basename"),
		PayloadTruncated:  parseLocalAgentAuditLogBoolParam(query, "payload_truncated"),
		SortBy:            query.Get("sort_by"),
		SortOrder:         query.Get("sort_order"),
		Query:             strings.TrimSpace(query.Get("query")),
	}

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
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			opts.Limit = l
		}
	}
	if offsetStr := query.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			opts.Offset = o
		}
	}
	if durationMsMin := query.Get("duration_ms_min"); durationMsMin != "" {
		if minVal, err := strconv.ParseInt(durationMsMin, 10, 64); err == nil && minVal >= 0 {
			opts.DurationMsMin = minVal
		}
	}
	if durationMsMax := query.Get("duration_ms_max"); durationMsMax != "" {
		if maxVal, err := strconv.ParseInt(durationMsMax, 10, 64); err == nil && maxVal >= 0 {
			opts.DurationMsMax = maxVal
		}
	}

	return opts
}

func parseLocalAgentAuditLogMultiValueParam(queryValues map[string][]string, key string) []string {
	values := queryValues[key]
	if len(values) == 0 {
		return nil
	}

	var result []string
	for _, value := range values {
		if value == "" {
			continue
		}
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

func parseLocalAgentAuditLogBoolParam(queryValues map[string][]string, key string) []bool {
	values := parseLocalAgentAuditLogMultiValueParam(queryValues, key)
	if len(values) == 0 {
		return nil
	}

	var result []bool
	for _, value := range values {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			result = append(result, parsed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func parseLocalAgentAuditLogIntParam(queryValues map[string][]string, key string) []int {
	values := parseLocalAgentAuditLogMultiValueParam(queryValues, key)
	if len(values) == 0 {
		return nil
	}

	var result []int
	for _, value := range values {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			result = append(result, parsed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
