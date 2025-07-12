package invoke

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	var extraEnv []string
	if toolString, ok := tool.(string); ok {
		toolRef, err := render.ResolveToolReference(ctx, i.uncached, "", thread.Namespace, toolString)
		if err != nil {
			return "", err
		}

		var agent v1.Agent
		if thread.Spec.AgentName != "" {
			if err := i.uncached.Get(ctx, router.Key(thread.Namespace, thread.Spec.AgentName), &agent); err != nil {
				return "", err
			}
		}

		tool, extraEnv, err = render.Agent(ctx, i.tokenService, i.uncached, i.gptClient, &v1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: thread.Namespace,
			},
			Spec: v1.AgentSpec{
				Manifest: types.AgentManifest{
					Prompt: "#!sys.call " + toolRef,
					Tools:  []string{toolRef},
					Env:    agent.Spec.Manifest.Env,
				},
			},
		}, i.serverURL, render.AgentOptions{
			Thread: thread,
		})
		if err != nil {
			return "", err
		}
	}

	var credContexts []string
	if thread.Name != "" {
		credContexts = append(credContexts, thread.Name)
	}
	if thread.Spec.AgentName != "" {
		credContexts = append(credContexts, thread.Spec.AgentName)
	}
	if thread.Namespace != "" {
		credContexts = append(credContexts, thread.Namespace)
	}

	resp, err := i.createRun(ctx, i.uncached, thread, tool, inputString, runOptions{
		Ephemeral:            true,
		Env:                  append(opt.Env, extraEnv...),
		CredentialContextIDs: append(opt.CredentialContextIDs, credContexts...),
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

	var credContexts []string
	if thread != nil && thread.Namespace != "" {
		credContexts = append(credContexts, thread.Namespace)
	}
	credContexts = append(opt.CredentialContextIDs, credContexts...)

	return i.createRun(ctx, i.uncached, thread, tool, inputString, runOptions{
		Env:                  opt.Env,
		CredentialContextIDs: credContexts,
		Synchronous:          true,
		Timeout:              opt.Timeout,
	})
}
