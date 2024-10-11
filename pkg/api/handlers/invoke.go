package handlers

import (
	"net/http"

	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/invoke"
	"github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type InvokeHandler struct {
	invoker *invoke.Invoker
}

func NewInvokeHandler(invoker *invoke.Invoker) *InvokeHandler {
	return &InvokeHandler{
		invoker: invoker,
	}
}

func (i *InvokeHandler) Invoke(req api.Context) error {
	var (
		id       = req.PathValue("id")
		agentID  string
		wfID     string
		agent    v1.Agent
		wf       v1.Workflow
		ref      v1.Reference
		threadID = req.PathValue("thread")
		stepID   = req.URL.Query().Get("step")
		async    = req.URL.Query().Get("async") == "true"
	)

	if threadID == "" {
		threadID = req.Request.Header.Get("X-Otto-Thread-Id")
	}

	if system.IsAgentID(id) {
		agentID = id
	} else if system.IsWorkflowID(id) {
		wfID = id
	} else if system.IsThreadID(id) {
		var thread v1.Thread
		if err := req.Get(&thread, id); err != nil {
			return err
		}
		agentID = thread.Spec.AgentName
		wfID = thread.Spec.WorkflowName
		threadID = id
	} else {
		if err := req.Get(&ref, id); apierrors.IsNotFound(err) {
		} else if err != nil {
			return err
		} else if ref.Spec.AgentName != "" {
			agentID = ref.Spec.AgentName
		} else if ref.Spec.WorkflowName != "" {
			wfID = ref.Spec.WorkflowName
		}
	}

	if agentID != "" {
		if err := req.Get(&agent, agentID); err != nil {
			return err
		}
	} else if wfID != "" {
		if err := req.Get(&wf, wfID); err != nil {
			return err
		}
	} else {
		return apierrors.NewBadRequest("invalid id, most be agent or workflow id")
	}

	input, err := req.Body()
	if err != nil {
		return err
	}

	var resp *invoke.Response

	if agentID != "" {
		resp, err = i.invoker.Agent(req.Context(), req.Storage, &agent, string(input), invoke.Options{
			ThreadName: threadID,
			Events:     !async,
		})
		if err != nil {
			return err
		}
	} else {
		resp, err = i.invoker.Workflow(req.Context(), req.Storage, &wf, string(input), invoke.WorkflowOptions{
			ThreadName: threadID,
			Events:     !async,
			StepID:     stepID,
		})
		if err != nil {
			return err
		}
	}

	req.ResponseWriter.Header().Set("X-Otto-Thread-Id", resp.Thread.Name)

	if async {
		req.WriteHeader(http.StatusCreated)
		return req.Write(map[string]string{
			"threadID": resp.Thread.Name,
		})
	}

	return req.WriteEvents(resp.Events)
}
