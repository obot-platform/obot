package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

const localAgentAuditLogMaxRequestBytes = 2 * 1024 * 1024 // 2 MiB

type LocalAgentAuditLogHandler struct{}

func NewLocalAgentAuditLogHandler() *LocalAgentAuditLogHandler {
	return &LocalAgentAuditLogHandler{}
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
			ClientName:         input.ClientInfo.Name,
			ClientVersion:      input.ClientInfo.Version,
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

func decodeLocalAgentAuditLogInputs(body []byte) ([]types.LocalAgentAuditLog, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var inputs []types.LocalAgentAuditLog
		if err := json.Unmarshal(trimmed, &inputs); err != nil {
			return nil, err
		}
		return inputs, nil
	}

	var input types.LocalAgentAuditLog
	if err := json.Unmarshal(trimmed, &input); err != nil {
		return nil, err
	}
	return []types.LocalAgentAuditLog{input}, nil
}
