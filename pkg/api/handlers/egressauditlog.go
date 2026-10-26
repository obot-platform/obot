package handlers

import (
	"errors"
	"net/url"
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
	opts, err := parseEgressAuditLogOptions(req.URL.Query())
	if err != nil {
		return err
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	opts.Limit = min(opts.Limit, 100)
	items, total, err := req.GatewayClient.ListEgressAuditLogs(req.Context(), opts)
	if err != nil {
		return err
	}
	return req.Write(map[string]any{"items": items, "total": total, "limit": opts.Limit, "offset": opts.Offset})
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

func (h *EgressAuditLogHandler) ListFilterOptions(req api.Context) error {
	filter := req.PathValue("filter")
	switch filter {
	case "host", "claude_session", "claude_account":
	default:
		return types.NewErrBadRequest("invalid filter: %s", filter)
	}
	opts, err := parseEgressAuditLogOptions(req.URL.Query())
	if err != nil {
		return err
	}
	options, err := req.GatewayClient.GetEgressAuditLogFilterOptions(req.Context(), filter, opts)
	if err != nil {
		return err
	}
	return req.Write(map[string]any{"options": options})
}

func parseEgressAuditLogOptions(q url.Values) (client.EgressAuditLogOptions, error) {
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	opts := client.EgressAuditLogOptions{
		Limit:         limit,
		Offset:        max(offset, 0),
		UserID:        parseStringList(q, "user_id"),
		ClaudeAccount: parseStringList(q, "claude_account"),
		EnvironmentID: parseStringList(q, "environment_id"),
		ClaudeSession: parseStringList(q, "claude_session"),
		Host:          parseStringList(q, "host"),
		Outcome:       parseStringList(q, "outcome"),
	}
	for key, dest := range map[string]*time.Time{"start_time": &opts.StartTime, "end_time": &opts.EndTime} {
		if q.Get(key) != "" {
			parsed, err := time.Parse(time.RFC3339, q.Get(key))
			if err != nil {
				return client.EgressAuditLogOptions{}, types.NewErrBadRequest("invalid %s", key)
			}
			*dest = parsed
		}
	}
	return opts, nil
}
