package handlers

import (
	"slices"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type ConfirmHandler struct{}

func NewConfirmHandler() *ConfirmHandler {
	return &ConfirmHandler{}
}

func (c *ConfirmHandler) Confirm(req api.Context) error {
	var (
		threadID = req.PathValue("thread_id")
		thread   v1.Thread
	)

	if err := req.Get(&thread, threadID); err != nil {
		return err
	}

	var confirm types.ToolConfirmResponse
	if err := req.Read(&confirm); err != nil {
		return err
	}

	approved := true
	switch confirm.Decision {
	case types.ToolConfirmDecisionDeny:
		approved = false
	case types.ToolConfirmDecisionApprove:
	case types.ToolConfirmDecisionApproveThread:
		// User is pre-approving a tool (or all tools with "*"), update the thread
		if confirm.ToolName == "" {
			return types.NewErrBadRequest("tool name must be set for thread approval")
		}

		if !slices.Contains(thread.Spec.ApprovedTools, confirm.ToolName) {
			thread.Spec.ApprovedTools = append(thread.Spec.ApprovedTools, confirm.ToolName)
			if err := req.Update(&thread); err != nil {
				return err
			}
		}
	default:
		return types.NewErrBadRequest("invalid decision: %q", confirm.Decision)
	}

	return req.GPTClient.Confirm(req.Context(), gptscript.AuthResponse{
		ID:     confirm.ID,
		Accept: approved,
	})
}
