package invoke

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type SystemTaskOptions struct {
	CredentialContextIDs []string
	Env                  []string
	Timeout              time.Duration
}

func complete(opts []SystemTaskOptions) (result SystemTaskOptions) {
	for _, opt := range opts {
		result.CredentialContextIDs = append(result.CredentialContextIDs, opt.CredentialContextIDs...)
		result.Env = append(result.Env, opt.Env...)
		if opt.Timeout > result.Timeout {
			result.Timeout = opt.Timeout // highest timeout wins
		}
	}
	return
}

func inputToString(input any) (string, error) {
	var inputString string
	switch v := input.(type) {
	case string:
		inputString = v
	case []byte:
		inputString = string(v)
	case nil:
		inputString = ""
	default:
		data, err := json.Marshal(input)
		if err != nil {
			return "", err
		}
		inputString = string(data)
	}
	// dumb hack to catch nil pointers than might be a nil value in a non-nil interface
	if inputString == "null" {
		inputString = ""
	}
	return inputString, nil
}

func (i *Invoker) SystemTask(ctx context.Context, gptClient *gptscript.GPTScript, thread *v1.Thread, tool, input any, opts ...SystemTaskOptions) (*Response, error) {
	opt := complete(opts)

	inputString, err := inputToString(input)
	if err != nil {
		return nil, err
	}

	if err := unAbortThread(ctx, i.uncached, thread); err != nil {
		return nil, err
	}

	var credContexts []string
	if thread != nil && thread.Namespace != "" {
		credContexts = append(credContexts, thread.Namespace)
	}
	credContexts = append(opt.CredentialContextIDs, credContexts...)

	return i.createRun(ctx, gptClient, i.uncached, thread, tool, inputString, runOptions{
		Env:                  opt.Env,
		CredentialContextIDs: credContexts,
		Timeout:              opt.Timeout,
	})
}
