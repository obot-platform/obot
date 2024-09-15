package handlers

import (
	"time"

	"github.com/gptscript-ai/otto/pkg/api"
	"github.com/gptscript-ai/otto/pkg/invoke"
	"github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/system"
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
		agentID  = req.PathValue("agent")
		agent    v1.Agent
		slug     v1.Slug
		threadID = req.PathValue("thread")
	)

	if threadID == "" {
		threadID = req.Request.Header.Get("X-Otto-Thread-Id")
	}

	if !system.IsSystemID(agentID) {
		if err := req.Get(&slug, agentID); apierrors.IsNotFound(err) {
		} else if err != nil {
			return err
		} else if slug.Spec.AgentName != "" {
			agentID = slug.Spec.AgentName
		}
	}

	if err := req.Get(&agent, agentID); err != nil {
		return err
	}

	input, err := req.Body()
	if err != nil {
		return err
	}

	resp, err := i.invoker.Agent(req.Context(), req.Storage, &agent, string(input), invoke.Options{
		ThreadName: threadID,
	})
	if err != nil {
		return err
	}

	req.ResponseWriter.Header().Set("X-Otto-Thread-Id", resp.Thread.Name)
	req.ResponseWriter.Header().Set("X-Otto-Run-Id", resp.Run.Name)

	// Check if SSE is requested
	sendEvents := req.IsStreamRequested()
	if sendEvents {
		req.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	}

	var lastFlush time.Time
	for event := range resp.Events {
		if sendEvents {
			if err := req.WriteDataEvent(event); err != nil {
				return err
			}
		} else {
			if err := req.Write([]byte(event.Content)); err != nil {
				return err
			}
			if lastFlush.IsZero() || time.Since(lastFlush) > 500*time.Millisecond {
				req.Flush()
				lastFlush = time.Now()
			}
		}
	}

	return nil
}
