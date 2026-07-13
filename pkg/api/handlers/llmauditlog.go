package handlers

import (
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

type LLMAuditLogHandler struct{}

func NewLLMAuditLogHandler() *LLMAuditLogHandler {
	return &LLMAuditLogHandler{}
}

func (h *LLMAuditLogHandler) List(req api.Context) error {
	opts := parseLLMAuditLogOpts(req.URL.Query())
	if opts.Limit == 0 {
		opts.Limit = 100
	}

	logs, total, err := req.GatewayClient.GetLLMAuditLogs(req.Context(), opts)
	if err != nil {
		return err
	}

	items := make([]types.LLMAuditLog, 0, len(logs))
	for _, log := range logs {
		items = append(items, gatewaytypes.ConvertLLMAuditLog(log))
	}

	return req.Write(types.LLMAuditLogResponse{
		LLMAuditLogList: types.LLMAuditLogList{Items: items},
		Total:           total,
		Limit:           opts.Limit,
		Offset:          opts.Offset,
	})
}

func (h *LLMAuditLogHandler) Get(req api.Context) error {
	id := req.PathValue("audit_log_id")

	log, err := req.GatewayClient.GetLLMAuditLog(req.Context(), id, req.UserIsAuditor())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("LLM audit log not found")
		}
		return err
	}

	return req.Write(gatewaytypes.ConvertLLMAuditLog(*log))
}

var llmAuditLogFilterOptions = map[string]any{
	"user_id":                  "",
	"model_provider":           "",
	"target_model":             "",
	"request_path":             "",
	"response_status":          0,
	"outcome":                  "",
	"client":                   "",
	"client_session_id":        "",
	"message_policy_triggered": nil,
}

func (h *LLMAuditLogHandler) ListFilterOptions(req api.Context) error {
	filter := req.PathValue("filter")

	exclude, ok := llmAuditLogFilterOptions[filter]
	if !ok {
		return types.NewErrBadRequest("invalid filter: %s", filter)
	}

	if filter == "message_policy_triggered" {
		return req.Write(map[string]any{"options": []string{"false", "true"}})
	}

	options, err := req.GatewayClient.GetLLMAuditLogFilterOptions(req.Context(), filter, parseLLMAuditLogOpts(req.URL.Query()), exclude)
	if err != nil {
		return err
	}
	sort.Strings(options)

	return req.Write(map[string]any{"options": options})
}

func parseLLMAuditLogOpts(query url.Values) gateway.LLMAuditLogOptions {
	opts := gateway.LLMAuditLogOptions{
		UserID:          parseStringList(query, "user_id"),
		ModelProvider:   parseStringList(query, "model_provider"),
		TargetModel:     parseStringList(query, "target_model"),
		RequestPath:     parseStringList(query, "request_path"),
		Outcome:         parseStringList(query, "outcome"),
		Client:          parseStringList(query, "client"),
		ClientSessionID: parseStringList(query, "client_session_id"),
		Query:           strings.TrimSpace(query.Get("query")),
		SortBy:          query.Get("sort_by"),
		SortOrder:       query.Get("sort_order"),
		StartTime:       time.Now().UTC().AddDate(0, 0, -30),
	}
	for _, value := range parseStringList(query, "message_policy_triggered") {
		if triggered, err := strconv.ParseBool(value); err == nil {
			opts.MessagePolicyTriggered = append(opts.MessagePolicyTriggered, triggered)
		}
	}

	for _, value := range parseStringList(query, "response_status") {
		status, err := strconv.Atoi(value)
		if err == nil {
			opts.ResponseStatus = append(opts.ResponseStatus, status)
		}
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

	return opts
}

func parseStringList(queryValues url.Values, key string) []string {
	var result []string
	for _, value := range queryValues[key] {
		for part := range strings.SplitSeq(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}
