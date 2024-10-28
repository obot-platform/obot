package invoke

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
)

const (
	SystemThreadLabel = "otto8.ai/system-thread"
	SystemThreadTTL   = 2 * time.Hour
)

type SystemActionOptions struct {
	Events bool
	// Only one of RemoteTool or Tools can be set, precedent is given to Tools
	RemoteTool   string
	Tools        []gptscript.ToolDef
	Input        any
	CredContexts []string
	Env          []string
}

func (i *Invoker) SystemAction(ctx context.Context, generateName, namespace string, opts SystemActionOptions) (*Response, error) {
	thread, err := i.NewThread(ctx, i.uncached, namespace, NewThreadOptions{
		ThreadGenerateName: generateName,
		Labels: map[string]string{
			SystemThreadLabel: "true",
		},
	})
	if err != nil {
		return nil, err
	}

	return i.SystemActionWithThread(ctx, thread, opts)
}

func (i *Invoker) SystemActionWithThread(ctx context.Context, thread *v1.Thread, opts SystemActionOptions) (*Response, error) {
	var inputString string
	switch v := opts.Input.(type) {
	case string:
		inputString = v
	case []byte:
		inputString = string(v)
	case nil:
		inputString = ""
	default:
		data, err := json.Marshal(opts.Input)
		if err != nil {
			return nil, err
		}
		inputString = string(data)
	}
	// dumb hack to catch nil pointers than might be a nil value in a non-nil interface
	if inputString == "null" {
		inputString = ""
	}

	if len(opts.Tools) > 0 {
		return i.createRunFromTools(ctx, i.uncached, thread, opts.Tools, inputString, runOptions{
			Events:               opts.Events,
			AgentName:            thread.Spec.AgentName,
			CredentialContextIDs: opts.CredContexts,
			Env:                  opts.Env,
		})
	}

	return i.createRunFromRemoteTool(ctx, i.uncached, thread, opts.RemoteTool, inputString, runOptions{
		Events:               opts.Events,
		AgentName:            thread.Spec.AgentName,
		CredentialContextIDs: opts.CredContexts,
		Env:                  opts.Env,
	})
}
