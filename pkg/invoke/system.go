package invoke

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

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

func (i *Invoker) EphemeralThreadTask(ctx context.Context, thread *v1.Thread, tool, input any, opts ...SystemTaskOptions) (string, error) {
	opt := complete(opts)

	inputString, err := inputToString(input)
	if err != nil {
		return "", err
	}

	resp, err := i.createRun(ctx, i.uncached, thread, tool, inputString, runOptions{
		Ephemeral:            true,
		Env:                  opt.Env,
		CredentialContextIDs: opt.CredentialContextIDs,
		Synchronous:          true,
		Timeout:              opt.Timeout,
	})
	if err != nil {
		return "", err
	}
	defer resp.Close()
	result := strings.Builder{}
	for event := range resp.Events {
		if event.Error != "" {
			return "", errors.New(event.Error)
		}
		result.WriteString(event.Content)
	}
	return result.String(), nil
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

func (i *Invoker) SystemTask(ctx context.Context, thread *v1.Thread, tool, input any, opts ...SystemTaskOptions) (*Response, error) {
	opt := complete(opts)

	inputString, err := inputToString(input)
	if err != nil {
		return nil, err
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
