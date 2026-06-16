package handlers

import (
	"encoding/json"
	"io"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
)

const (
	auditEventsMaxBodyBytes = 2 * 1024 * 1024
	auditEventsMaxBatch     = 100
)

type AuditEventsHandler struct{}

func NewAuditEventsHandler() *AuditEventsHandler {
	return &AuditEventsHandler{}
}

func (*AuditEventsHandler) Create(req api.Context) error {
	body, err := req.Body(api.BodyOptions{MaxBytes: auditEventsMaxBodyBytes})
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return io.EOF
	}

	var events []types.AuditEvent
	if err := json.Unmarshal(body, &events); err != nil {
		return types.NewErrBadRequest("failed to read input: %v", err)
	}
	if len(events) > auditEventsMaxBatch {
		return types.NewErrBadRequest("audit event batch exceeds %d events", auditEventsMaxBatch)
	}

	statuses, err := req.GatewayClient.InsertAuditEvents(req.Context(), req.User.GetUID(), requestinfo.GetSourceIP(req.Request), events)
	if err != nil {
		return err
	}
	return req.Write(types.AuditEventSubmitResponse{Items: statuses})
}
