package handlers

import (
	"errors"
	"strconv"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"gorm.io/gorm"
)

type EgressAuditLogHandler struct{}

func (h *EgressAuditLogHandler) Environments(req api.Context) error {
	environments, err := req.GatewayClient.EgressAuditEnvironments(req.Context())
	if err != nil {
		return err
	}
	return req.Write(environments)
}

func (h *EgressAuditLogHandler) List(req api.Context) error {
	q := req.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 100)
	offset, _ := strconv.Atoi(q.Get("offset"))
	offset = max(offset, 0)
	opts := client.EgressAuditLogOptions{Limit: limit, Offset: offset, UserID: q.Get("user_id"), ClaudeAccount: q.Get("claude_account"), EnvironmentID: q.Get("environment_id"), ClaudeSession: q.Get("claude_session"), Host: q.Get("host"), Outcome: q.Get("outcome")}
	for key, dest := range map[string]*time.Time{"start_time": &opts.StartTime, "end_time": &opts.EndTime} {
		if q.Get(key) != "" {
			parsed, err := time.Parse(time.RFC3339, q.Get(key))
			if err != nil {
				return types.NewErrBadRequest("invalid %s", key)
			}
			*dest = parsed
		}
	}
	items, total, err := req.GatewayClient.ListEgressAuditLogs(req.Context(), opts)
	if err != nil {
		return err
	}
	return req.Write(map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *EgressAuditLogHandler) Get(req api.Context) error {
	entry, err := req.GatewayClient.GetEgressAuditLog(req.Context(), req.PathValue("audit_log_id"), req.UserIsAuditor())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("egress audit log not found")
	}
	if err != nil {
		return err
	}
	return req.Write(entry)
}
