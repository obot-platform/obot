package handlers

import (
	"github.com/acorn-io/acorn/apiclient/types"
	"github.com/acorn-io/acorn/pkg/api"
	"github.com/acorn-io/acorn/pkg/events"
	"github.com/acorn-io/acorn/pkg/gz"
	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/gptscript-ai/go-gptscript"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RunHandler struct {
	events *events.Emitter
}

func NewRunHandler(events *events.Emitter) *RunHandler {
	return &RunHandler{
		events: events,
	}
}

func convertRun(run v1.Run) types.Run {
	state := "pending"
	switch run.Status.State {
	case gptscript.Creating, gptscript.Running:
		state = "running"
	case gptscript.Continue, gptscript.Finished:
		state = "completed"
	case gptscript.Error:
		state = "error"
	}
	result := types.Run{
		ID:             run.Name,
		Created:        *types.NewTime(run.CreationTimestamp.Time),
		ThreadID:       run.Spec.ThreadName,
		AgentID:        run.Spec.AgentName,
		WorkflowID:     run.Spec.WorkflowName,
		WorkflowStepID: run.Spec.WorkflowStepID,
		PreviousRunID:  run.Spec.PreviousRunName,
		Input:          run.Spec.Input,
		State:          state,
		Output:         run.Status.Output,
		Error:          run.Status.Error,
	}
	if run.Status.SubCall != nil {
		result.SubCallWorkflowID = run.Status.SubCall.Workflow
		result.SubCallInput = run.Status.SubCall.Input
	}
	return result
}

func (a *RunHandler) Debug(req api.Context) error {
	var (
		runID = req.PathValue("id")
	)

	var (
		runState v1.RunState
		run      v1.Run
	)
	if err := req.Get(&runState, runID); err != nil {
		return err
	}
	if err := req.Get(&run, runID); err != nil {
		return err
	}

	frames := map[string]any{}
	if err := gz.Decompress(&frames, runState.Spec.CallFrame); err != nil {
		return err
	}

	return req.Write(map[string]any{
		"spec":   run.Spec,
		"frames": frames,
		"status": run.Status,
	})
}

func (a *RunHandler) Events(req api.Context) error {
	var (
		runID = req.PathValue("id")
	)

	_, events, err := a.events.Watch(req.Context(), req.Namespace(), events.WatchOptions{
		LastRunName: runID,
	})
	if err != nil {
		return err
	}

	return req.WriteEvents(events)
}

func (a *RunHandler) stream(req api.Context, criteria func(*v1.Run) bool) error {
	c, err := api.Watch[*v1.Run](req, &v1.RunList{})
	if err != nil {
		return err
	}

	req.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	for run := range c {
		if !criteria(run) {
			continue
		}
		if err := req.WriteDataEvent(convertRun(*run)); err != nil {
			return err
		}
	}

	return nil
}

func runCriteria(agentName, threadName, workflowName string) func(*v1.Run) bool {
	return func(run *v1.Run) bool {
		if agentName != "" && run.Spec.AgentName != agentName {
			return false
		}
		if threadName != "" && run.Spec.ThreadName != threadName {
			return false
		}
		if workflowName != "" && run.Spec.WorkflowName != workflowName {
			return false
		}
		return true
	}
}

func (a *RunHandler) ByID(req api.Context) error {
	var (
		runID = req.PathValue("id")
	)

	var run v1.Run
	if err := req.Get(&run, runID); err != nil {
		return err
	}

	return req.Write(convertRun(run))
}

func (a *RunHandler) Delete(req api.Context) error {
	var (
		runID = req.PathValue("id")
	)

	return req.Delete(&v1.Run{
		ObjectMeta: metav1.ObjectMeta{
			Name:      runID,
			Namespace: req.Namespace(),
		},
	})
}

func (a *RunHandler) List(req api.Context) error {
	var (
		criteria = runCriteria(req.PathValue("agent"),
			req.PathValue("thread"),
			req.PathValue("workflow"))
		runList v1.RunList
	)

	if req.IsStreamRequested() {
		return a.stream(req, criteria)
	}

	if err := req.List(&runList); err != nil {
		return err
	}

	var resp types.RunList
	for _, run := range runList.Items {
		if criteria(&run) {
			resp.Items = append(resp.Items, convertRun(run))
		}
	}

	return req.Write(resp)
}
