package handlers

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/client-go/util/retry"
)

type ConfirmHandler struct{}

func NewConfirmHandler() *ConfirmHandler {
	return &ConfirmHandler{}
}

func (c *ConfirmHandler) Confirm(req api.Context) error {
	var (
		threadID = req.PathValue("thread_id")
		confirm  types.ToolConfirmResponse
	)

	if err := req.Read(&confirm); err != nil {
		return err
	}

	if confirm.ID == "" {
		return types.NewErrBadRequest("id must be set")
	}

	// Validate input and collect approval decision and thread approved tool name
	var (
		approved = true
		toolName = ""
	)
	switch confirm.Decision {
	case types.ToolConfirmDecisionDeny:
		approved = false
	case types.ToolConfirmDecisionApprove:
	case types.ToolConfirmDecisionApproveThread:
		// User is pre-approving a tool (or all tools with "*")
		toolName = confirm.ToolName
		if toolName == "" {
			return types.NewErrBadRequest("tool name must be set for thread approval")
		}
	default:
		return types.NewErrBadRequest("invalid decision: %q", confirm.Decision)
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var thread v1.Thread
		if err := req.Get(&thread, threadID); err != nil {
			return err
		}

		if thread.Status.CurrentRunName == "" {
			return types.NewErrBadRequest("thread not running")
		}

		var run v1.Run
		if err := req.Get(&run, thread.Status.CurrentRunName); err != nil {
			return err
		}

		if toolName != "" && !slices.Contains(thread.Spec.ApprovedTools, toolName) {
			// toolName is set, add it to approve tools matching the pattern for the remainder of the thread.
			thread.Spec.ApprovedTools = append(thread.Spec.ApprovedTools, confirm.ToolName)
			if err := req.Update(&thread); err != nil {
				return err
			}
		}

		// Ensure we're not trying to change an existing decision
		if _, ok := run.Spec.CallDecisions[confirm.ID]; ok {
			return types.NewErrBadRequest("decision for call ID %s already submitted", confirm.ID)
		}

		if run.Spec.CallDecisions == nil {
			run.Spec.CallDecisions = make(map[string]bool)
		}
		run.Spec.CallDecisions[confirm.ID] = approved

		return req.Update(&run)
	})
}
