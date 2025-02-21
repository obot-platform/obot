package handlers

import (
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
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
		threadID    = req.PathValue("thread")
		synchronous = req.URL.Query().Get("async") != "true"
	)

	if threadID == "" {
		threadID = req.Request.Header.Get("X-Obot-Thread-Id")
	}

	if system.IsThreadID(id) {
		var thread v1.Thread
		if err := req.Get(&thread, id); err != nil {
			return err
		}
		if err := req.Get(&agent, thread.Spec.AgentName); err != nil {
			return err
		}
	} else if system.IsAgentID(id) {
		if err := req.Get(&agent, id); err != nil {
			return err
		}
	} else {
		if err := alias.Get(req.Context(), req.Storage, &agent, req.Namespace(), id); err != nil {
			return err
		}
	}

	if agent.Name == "" {
		return apierrors.NewBadRequest("invalid id, most be agent or workflow id")
	}

	input, err := req.Body()
	if err != nil {
		return err
	}

	resp, err := i.invoker.Agent(req.Context(), req.Storage, &agent, string(input), invoke.Options{
		GenerateName: system.ChatRunPrefix,
		ThreadName:   threadID,
		Synchronous:  synchronous,
		CreateThread: true,
		UserUID:      req.User.GetUID(),
	})
	if err != nil {
		return err
	}
	defer resp.Close()

	req.ResponseWriter.Header().Set("X-Obot-Thread-Id", resp.Thread.Name)

	if synchronous {
		return req.WriteEvents(resp.Events)
	}

	var runID string
	if resp.Run != nil {
		runID = resp.Run.Name
	}

	req.ResponseWriter.Header().Set("Content-Type", "application/json")
	return req.Write(map[string]string{
		"threadID": resp.Thread.Name,
		"runID":    runID,
	})
}
