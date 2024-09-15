package handlers

import (
	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/otto/pkg/api"
	"github.com/gptscript-ai/otto/pkg/api/types"
	"github.com/gptscript-ai/otto/pkg/gz"
	"github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
)

type RunHandler struct {
}

func NewRunHandler() *RunHandler {
	return &RunHandler{}
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
	return types.Run{
		ID:            run.Name,
		Created:       run.CreationTimestamp.Time,
		ThreadID:      run.Spec.ThreadName,
		AgentID:       run.Spec.AgentName,
		WorkflowID:    run.Spec.WorkflowName,
		PreviousRunID: run.Spec.PreviousRunName,
		Input:         run.Spec.Input,
		State:         state,
		Output:        run.Status.Output,
		Error:         run.Status.Error,
	}
}

func (a *RunHandler) Debug(req api.Context) error {
	var (
		runID = req.PathValue("id")
	)

	var run v1.RunState
	if err := req.Get(&run, runID); err != nil {
		return err
	}

	calls := map[string]any{}
	if err := gz.Decompress(&calls, run.Spec.CallFrame); err != nil {
		return err
	}

	return req.Write(calls)
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

func runCriteria(agentName, threadName string) func(*v1.Run) bool {
	return func(run *v1.Run) bool {
		if agentName != "" && run.Spec.AgentName != agentName {
			return false
		}
		if threadName != "" && run.Spec.ThreadName != threadName {
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

func (a *RunHandler) List(req api.Context) error {
	var (
		criteria = runCriteria(req.PathValue("agent"), req.PathValue("thread"))
		runList  v1.RunList
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
