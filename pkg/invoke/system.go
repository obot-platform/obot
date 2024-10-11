package invoke

import (
	"context"
	"encoding/json"
	"time"

	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
)

const (
	SystemThreadLabel = "otto8.ai/system-thread"
	SystemThreadTTL   = 2 * time.Hour
)

func (i *Invoker) SystemAction(ctx context.Context, generateName, namespace, tool string, input any, env ...string) (*Response, error) {
	thread, err := i.NewThread(ctx, i.uncached, namespace, NewThreadOptions{
		ThreadGenerateName: generateName,
		Labels: map[string]string{
			SystemThreadLabel: "true",
		},
	})
	if err != nil {
		return nil, err
	}

	return i.SystemActionWithThread(ctx, thread, tool, input, env...)
}

func (i *Invoker) SystemActionWithThread(ctx context.Context, thread *v1.Thread, tool string, input any, env ...string) (*Response, error) {
	var inputString string
	switch v := input.(type) {
	case string:
		inputString = v
	case []byte:
		inputString = string(v)
	default:
		data, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		inputString = string(data)
	}
	// dumb hack to catch nil pointers than might be a nil value in a non-nil interface
	if inputString == "null" {
		inputString = ""
	}

	return i.createRunFromRemoteTool(ctx, i.uncached, thread, tool, inputString, runOptions{
		AgentName: thread.Spec.AgentName,
		Env:       env,
	})
}
