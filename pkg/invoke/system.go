package invoke

import (
	"context"
	"encoding/json"
	"time"

	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
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

func (i *Invoker) SystemTask(ctx context.Context, thread *v1.Thread, tool, input any, opts ...SystemTaskOptions) (*Response, error) {
	opt := complete(opts)

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
			return nil, err
		}
		inputString = string(data)
	}
	// dumb hack to catch nil pointers than might be a nil value in a non-nil interface
	if inputString == "null" {
		inputString = ""
	}

	if err := unAbortThread(ctx, i.uncached, thread); err != nil {
		return nil, err
	}

	return i.createRun(ctx, i.uncached, thread, tool, inputString, runOptions{
		Env:                  opt.Env,
		CredentialContextIDs: opt.CredentialContextIDs,
		Synchronous:          true,
		Timeout:              opt.Timeout,
	})
}
