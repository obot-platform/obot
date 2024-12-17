package handlers

import (
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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
		id          = req.PathValue("id")
		agent       v1.Agent
		wf          v1.Workflow
		threadID    = req.PathValue("thread")
		stepID      = req.URL.Query().Get("step")
		synchronous = req.URL.Query().Get("async") != "true"
	)

	if threadID == "" {
		threadID = req.Request.Header.Get("X-Otto-Thread-Id")
	}

	if system.IsThreadID(id) {
		var thread v1.Thread
		if err := req.Get(&thread, id); err != nil {
			return err
		}
		if thread.Spec.AgentName != "" {
			if err := req.Get(&agent, thread.Spec.AgentName); err != nil {
				return err
			}
		} else if thread.Spec.WorkflowName != "" {
			if err := req.Get(&wf, thread.Spec.WorkflowName); err != nil {
				return err
			}
		}
	} else if system.IsAgentID(id) {
		if err := req.Get(&agent, id); err != nil {
			return err
		}
	} else if system.IsWorkflowID(id) {
		if err := req.Get(&wf, id); err != nil {
			return err
		}
	} else {
		err := alias.Get(req.Context(), req.Storage, &agent, req.Namespace(), id)
		if apierrors.IsNotFound(err) {
			newErr := alias.Get(req.Context(), req.Storage, &wf, req.Namespace(), id)
			if apierrors.IsNotFound(newErr) {
				return err
			} else if newErr != nil {
				return newErr
			}
		} else if err != nil {
			return err
		}
	}

	if agent.Name == "" && wf.Name == "" {
		return apierrors.NewBadRequest("invalid id, most be agent or workflow id")
	}

	input, err := req.Body()
	if err != nil {
		return err
	}

	var resp *invoke.Response

	if agent.Name != "" {
		resp, err = i.invoker.Agent(req.Context(), req.Storage, &agent, string(input), invoke.Options{
			ThreadName:   threadID,
			Synchronous:  synchronous,
			CreateThread: true,
			UserUID:      req.User.GetUID(),
		})
		if err != nil {
			return err
		}
	} else {
		synchronous = false
		resp, err = i.invoker.Workflow(req.Context(), req.Storage, &wf, string(input), invoke.WorkflowOptions{
			ThreadName: threadID,
			StepID:     stepID,
		})
		if err != nil {
			return err
		}
	}
	defer resp.Close()

	req.ResponseWriter.Header().Set("X-Otto-Thread-Id", resp.Thread.Name)

	if synchronous {
		return req.WriteEvents(resp.Events)
	}

	req.ResponseWriter.Header().Set("Content-Type", "application/json")
	return req.Write(map[string]string{
		"threadID": resp.Thread.Name,
	})
}
