package invoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/otto/apiclient/types"
	log2 "github.com/gptscript-ai/otto/logger"
	"github.com/gptscript-ai/otto/pkg/events"
	"github.com/gptscript-ai/otto/pkg/gz"
	"github.com/gptscript-ai/otto/pkg/jwt"
	"github.com/gptscript-ai/otto/pkg/render"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/system"
	"github.com/gptscript-ai/otto/pkg/workspace"
	wclient "github.com/thedadams/workspace-provider/pkg/client"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = log2.Package()

type Invoker struct {
	gptClient               *gptscript.GPTScript
	uncached                kclient.Client
	tokenService            *jwt.TokenService
	workspaceClient         *wclient.Client
	events                  *events.Emitter
	threadWorkspaceProvider string
	knowledgeTool           string
}

func NewInvoker(c kclient.Client, gptClient *gptscript.GPTScript, tokenService *jwt.TokenService, workspaceClient *wclient.Client, events *events.Emitter, knowledgeTool string) *Invoker {
	return &Invoker{
		uncached:                c,
		gptClient:               gptClient,
		tokenService:            tokenService,
		workspaceClient:         workspaceClient,
		events:                  events,
		threadWorkspaceProvider: "directory",
		knowledgeTool:           knowledgeTool,
	}
}

type Response struct {
	cancel context.CancelFunc
	Run    *v1.Run
	Thread *v1.Thread
	Events <-chan types.Progress
}

func (r *Response) Wait() error {
	if r.Events == nil {
		return nil
	}
	var errString string
	for e := range r.Events {
		if e.Error != "" {
			errString = e.Error
		}
	}
	if errString != "" {
		return errors.New(errString)
	}
	return nil
}

type Options struct {
	Background            bool
	ThreadName            string
	PreviousRunName       string
	WorkflowName          string
	WorkflowExecutionName string
	WorkflowStepName      string
	WorkflowStepID        string
	Env                   []string
}

type NewThreadOptions struct {
	AgentName             string
	ThreadName            string
	ThreadGenerateName    string
	WorkflowName          string
	WorkflowExecutionName string
	WebhookName           string
	CronJobName           string
	WorkspaceIDs          []string
	KnowledgeWorkspaceIDs []string
}

func (i *Invoker) NewThread(ctx context.Context, c kclient.Client, namespace string, opt NewThreadOptions) (*v1.Thread, error) {
	var (
		thread               v1.Thread
		createName           string
		workspaceID          string
		knowledgeWorkspaceID string
		err                  error
	)

	if opt.ThreadName != "" {
		err := c.Get(ctx, router.Key(namespace, opt.ThreadName), &thread)
		if apierror.IsNotFound(err) {
			if system.IsThreadID(opt.ThreadName) {
				createName = opt.ThreadName
			} else {
				return nil, err
			}
		} else {
			return &thread, err
		}
	}

	if len(opt.WorkspaceIDs) > 0 {
		workspaceID, err = i.workspaceClient.Create(ctx, i.threadWorkspaceProvider, opt.WorkspaceIDs...)
		if err != nil {
			return nil, err
		}
	}

	if len(opt.KnowledgeWorkspaceIDs) > 0 {
		knowledgeWorkspaceID, err = i.workspaceClient.Create(ctx, i.threadWorkspaceProvider, opt.KnowledgeWorkspaceIDs...)
		if err != nil {
			return nil, err
		}
	}

	thread = v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			Name:         createName,
			GenerateName: system.ThreadPrefix,
			Namespace:    namespace,
		},
		Spec: v1.ThreadSpec{
			AgentName:             opt.AgentName,
			WorkflowExecutionName: opt.WorkflowExecutionName,
			WorkflowName:          opt.WorkflowName,
			WebhookName:           opt.WebhookName,
			CronJobName:           opt.CronJobName,
			WorkspaceID:           workspaceID,
			KnowledgeWorkspaceID:  knowledgeWorkspaceID,
		},
	}

	if opt.ThreadGenerateName != "" {
		if system.IsThreadID(opt.ThreadGenerateName) {
			thread.GenerateName = opt.ThreadGenerateName
		} else {
			thread.GenerateName = system.ThreadPrefix + opt.ThreadGenerateName
		}
	}

	if err := c.Create(ctx, &thread); err != nil {
		// If creating the thread fails, then ensure that the workspace is cleaned up, too.
		return nil, errors.Join(err, i.workspaceClient.Rm(ctx, thread.Spec.WorkspaceID))
	}
	return &thread, nil
}

func (i *Invoker) getChatState(ctx context.Context, c kclient.Client, run *v1.Run) (result, lastThreadName string, _ error) {
	if run.Spec.PreviousRunName == "" {
		return "", "", nil
	}

	for {
		// look for the last valid state
		var previousRun v1.Run
		if err := c.Get(ctx, router.Key(run.Namespace, run.Spec.PreviousRunName), &previousRun); err != nil {
			return "", "", err
		}
		if previousRun.Status.State == gptscript.Continue {
			break
		}
		if previousRun.Spec.PreviousRunName == "" {
			return "", "", nil
		}
		run = &previousRun
	}

	var lastRun v1.RunState
	if err := c.Get(ctx, router.Key(run.Namespace, run.Spec.PreviousRunName), &lastRun); apierror.IsNotFound(err) {
		return "", "", nil
	} else if err != nil {
		return "", "", err
	}

	err := gz.Decompress(&result, lastRun.Spec.ChatState)
	return result, lastRun.Spec.ThreadName, err
}

func (i *Invoker) Agent(ctx context.Context, c kclient.Client, agent *v1.Agent, input string, opt Options) (*Response, error) {
	threadOpt := NewThreadOptions{
		AgentName:  agent.Name,
		ThreadName: opt.ThreadName,
	}
	if agent.Status.WorkspaceName != "" {
		var ws v1.Workspace
		if err := c.Get(ctx, kclient.ObjectKey{Namespace: agent.Namespace, Name: agent.Status.WorkspaceName}, &ws); err != nil {
			return nil, err
		}
		threadOpt.WorkspaceIDs = append(threadOpt.WorkspaceIDs, ws.Status.WorkspaceID)
	}
	if agent.Status.KnowledgeWorkspaceName != "" {
		var ws v1.Workspace
		if err := c.Get(ctx, kclient.ObjectKey{Namespace: agent.Namespace, Name: agent.Status.KnowledgeWorkspaceName}, &ws); err != nil {
			return nil, err
		}
		threadOpt.KnowledgeWorkspaceIDs = append(threadOpt.KnowledgeWorkspaceIDs, ws.Status.WorkspaceID)
	}

	thread, err := i.NewThread(ctx, c, agent.Namespace, threadOpt)
	if err != nil {
		return nil, err
	}

	credContextIDs := []string{thread.Name}
	if agent.Spec.CredentialContextID != "" {
		credContextIDs = append(credContextIDs, agent.Spec.CredentialContextID)
	} else if agent.Name != "" {
		credContextIDs = append(credContextIDs, agent.Name)
	}
	credContextIDs = append(credContextIDs, agent.Namespace)

	tools, extraEnv, err := render.Agent(ctx, c, agent, render.AgentOptions{
		Thread:        thread,
		KnowledgeTool: i.knowledgeTool,
	})
	if err != nil {
		return nil, err
	}

	if len(agent.Spec.Manifest.Params) == 0 {
		data := map[string]any{}
		if err := json.Unmarshal([]byte(input), &data); err == nil {
			if msg, ok := data[render.DefaultAgentParams[0]].(string); ok && len(data) == 1 && msg != "" {
				input = msg
			}
		}
	}

	return i.createRunFromTools(ctx, c, thread, tools, input, runOptions{
		Background:            opt.Background,
		AgentName:             agent.Name,
		Env:                   append(opt.Env, extraEnv...),
		PreviousRunName:       opt.PreviousRunName,
		WorkflowName:          opt.WorkflowName,
		WorkflowExecutionName: opt.WorkflowExecutionName,
		WorkflowStepName:      opt.WorkflowStepName,
		WorkflowStepID:        opt.WorkflowStepID,
		CredentialContextIDs:  credContextIDs,
	})
}

type runOptions struct {
	AgentName             string
	Background            bool
	WorkflowName          string
	WorkflowExecutionName string
	WorkflowStepName      string
	WorkflowStepID        string
	PreviousRunName       string
	Env                   []string
	CredentialContextIDs  []string
}

func (i *Invoker) createRunFromTools(ctx context.Context, c kclient.Client, thread *v1.Thread, tools []gptscript.ToolDef, input string, opts runOptions) (*Response, error) {
	return i.createRun(ctx, c, thread, input, opts, tools)
}

func (i *Invoker) createRunFromRemoteTool(ctx context.Context, c kclient.Client, thread *v1.Thread, tool, input string, opts runOptions) (*Response, error) {
	return i.createRun(ctx, c, thread, input, opts, tool)
}

// createRun is a low-level method that creates a Run object from a list of tools or a remote tool.
// Callers should use createRunFromTools or createRunFromRemoteTool instead.
func (i *Invoker) createRun(ctx context.Context, c kclient.Client, thread *v1.Thread, input string, opts runOptions, tool any) (*Response, error) {
	previousRunName := thread.Status.LastRunName
	if opts.PreviousRunName != "" {
		previousRunName = opts.PreviousRunName
	}

	toolData, err := json.Marshal(tool)
	if err != nil {
		return nil, err
	}

	run := v1.Run{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.RunPrefix,
			Namespace:    thread.Namespace,
			Finalizers:   []string{v1.RunFinalizer},
		},
		Spec: v1.RunSpec{
			Background:            opts.Background,
			ThreadName:            thread.Name,
			AgentName:             opts.AgentName,
			WorkflowName:          opts.WorkflowName,
			WorkflowExecutionName: opts.WorkflowExecutionName,
			WorkflowStepName:      opts.WorkflowStepName,
			WorkflowStepID:        opts.WorkflowStepID,
			WorkspaceID:           thread.Spec.WorkspaceID,
			PreviousRunName:       previousRunName,
			Input:                 input,
			Tool:                  string(toolData),
			Env:                   opts.Env,
			// Just never pass cred contexts right now and always use default
			//CredentialContextIDs:  opts.CredentialContextIDs,
		},
	}

	if previousRunName != "" {
		run.Labels = map[string]string{
			v1.PreviousRunNameLabel: previousRunName,
		}
	}

	if err := c.Create(ctx, &run); err != nil {
		return nil, err
	}

	if run.Spec.Background {
		return &Response{
			Run:    &run,
			Thread: thread,
		}, nil
	}

	events, err := i.events.Watch(ctx, thread.Namespace, events.WatchOptions{
		Run: &run,
	})
	if err != nil {
		return nil, err
	}

	go i.Resume(ctx, c, thread, &run)

	return &Response{
		Run:    &run,
		Thread: thread,
		Events: events,
	}, nil
}

func (i *Invoker) Resume(ctx context.Context, c kclient.Client, thread *v1.Thread, run *v1.Run) error {
	defer func() {
		i.events.Done(run)
		time.AfterFunc(5*time.Minute, func() {
			i.events.ClearProgress(run)
		})
	}()

	chatState, prevThreadName, err := i.getChatState(ctx, c, run)
	if err != nil {
		return err
	}

	token, err := i.tokenService.NewToken(jwt.TokenContext{
		RunID:          run.Name,
		ThreadID:       thread.Name,
		AgentID:        run.Spec.AgentName,
		WorkflowID:     run.Spec.WorkflowName,
		WorkflowStepID: run.Spec.WorkflowStepID,
		Scope:          thread.Namespace,
	})
	if err != nil {
		return err
	}

	options := gptscript.Options{
		GlobalOptions: gptscript.GlobalOptions{
			Env: append(run.Spec.Env,
				"OTTO_TOKEN="+token,
				"OTTO_RUN_ID="+run.Name,
				"OTTO_THREAD_ID="+thread.Name,
				"OTTO_WORKFLOW_ID="+run.Spec.WorkflowName,
				"OTTO_WORKFLOW_STEP_ID="+run.Spec.WorkflowStepID,
				"OTTO_AGENT_ID="+run.Spec.AgentName,
			),
		},
		Input:              run.Spec.Input,
		Workspace:          workspace.GetDir(run.Spec.WorkspaceID),
		CredentialContexts: run.Spec.CredentialContextIDs,
		ChatState:          chatState,
		IncludeEvents:      true,
		ForceSequential:    true,
		Prompt:             true,
	}

	if len(run.Spec.Tool) == 0 {
		return fmt.Errorf("no tool specified")
	}

	var (
		runResp    *gptscript.Run
		toolDef    gptscript.ToolDef
		toolDefs   []gptscript.ToolDef
		toolString string
	)
	switch run.Spec.Tool[0] {
	case '"':
		if err := json.Unmarshal([]byte(run.Spec.Tool), &toolString); err != nil {
			return fmt.Errorf("invalid tool definition: %s: %w", run.Spec.Tool, err)
		}
		runResp, err = i.gptClient.Run(ctx, toolString, options)
		if err != nil {
			return err
		}
	case '[':
		if err := json.Unmarshal([]byte(run.Spec.Tool), &toolDefs); err != nil {
			return fmt.Errorf("invalid tool definition: %s: %w", run.Spec.Tool, err)
		}
		runResp, err = i.gptClient.Evaluate(ctx, options, toolDefs...)
		if err != nil {
			return err
		}
	case '{':
		if err := json.Unmarshal([]byte(run.Spec.Tool), &toolDef); err != nil {
			return fmt.Errorf("invalid tool definition: %s: %w", run.Spec.Tool, err)
		}
		runResp, err = i.gptClient.Evaluate(ctx, options, toolDef)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid tool definition: %s", run.Spec.Tool)
	}

	return i.stream(ctx, c, prevThreadName, thread, run, runResp)
}

func (i *Invoker) saveState(ctx context.Context, c kclient.Client, prevThreadName string, thread *v1.Thread, run *v1.Run, runResp *gptscript.Run, retErr error) error {
	var err error
	for j := 0; j < 3; j++ {
		err = i.doSaveState(ctx, c, prevThreadName, thread, run, runResp, retErr)
		if err == nil {
			return retErr
		}
		if !apierror.IsConflict(err) {
			return errors.Join(err, retErr)
		}
		// reload
		if err = c.Get(ctx, router.Key(run.Namespace, run.Name), run); err != nil {
			return errors.Join(err, retErr)
		}
		if err = c.Get(ctx, router.Key(thread.Namespace, thread.Name), thread); err != nil {
			return errors.Join(err, retErr)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if combinedError := errors.Join(err, retErr); combinedError != nil {
		return fmt.Errorf("failed to save state after 3 retries: %w", combinedError)
	}
	return retErr
}

func (i *Invoker) doSaveState(ctx context.Context, c kclient.Client, prevThreadName string, thread *v1.Thread, run *v1.Run, runResp *gptscript.Run, retErr error) error {
	var (
		runStateSpec v1.RunStateSpec
		runChanged   bool
		err          error
	)

	runStateSpec.ThreadName = run.Spec.ThreadName
	runStateSpec.Done = runResp.State().IsTerminal() || runResp.State() == gptscript.Continue
	if retErr != nil {
		runStateSpec.Error = retErr.Error()
	}

	if prg := runResp.Program(); prg != nil {
		runStateSpec.Program, err = gz.Compress(prg)
		if err != nil {
			return err
		}
	}

	runStateSpec.CallFrame, err = gz.Compress(runResp.Calls())
	if err != nil {
		return err
	}

	if chatState := runResp.ChatState(); chatState != "" {
		runStateSpec.ChatState, err = gz.Compress(chatState)
		if err != nil {
			return err
		}
	}

	var runState v1.RunState
	if err := i.uncached.Get(ctx, router.Key(run.Namespace, run.Name), &runState); apierror.IsNotFound(err) {
		runState = v1.RunState{
			ObjectMeta: metav1.ObjectMeta{
				Name:      run.Name,
				Namespace: run.Namespace,
			},
			Spec: runStateSpec,
		}
		if err := c.Create(ctx, &runState); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		if !bytes.Equal(runState.Spec.CallFrame, runStateSpec.CallFrame) ||
			!bytes.Equal(runState.Spec.ChatState, runStateSpec.ChatState) ||
			runState.Spec.Done != runStateSpec.Done ||
			runState.Spec.Error != runStateSpec.Error {
			runState.Spec = runStateSpec
			if err := i.uncached.Update(ctx, &runState); err != nil {
				return err
			}
		}
	}

	state := runResp.State()

	if run.Status.State != state {
		run.Status.State = state
		runChanged = true
	}

	var final bool
	switch state {
	case gptscript.Error:
		final = true
		errString := runResp.ErrorOutput()
		if errString == "" {
			errString = runResp.Err().Error()
		}
		if run.Status.Error != errString {
			run.Status.Error = errString
			runChanged = true
		}
	case gptscript.Continue, gptscript.Finished:
		final = true
		text, err := runResp.Text()
		if err != nil {
			// this should never happen because gptscript.Error would have been set
			panic(err)
		}
		if run.Status.Output != text {
			run.Status.Output = text
			runChanged = true
		}
	}

	if retErr != nil && !run.Status.State.IsTerminal() {
		run.Status.State = gptscript.Error
		if run.Status.Error == "" {
			run.Status.Error = retErr.Error()
		}
		runChanged = true
	}

	if runChanged {
		if err := c.Status().Update(ctx, run); err != nil {
			return err
		}
	}

	if final && thread.Status.LastRunName != run.Name {
		if prevThreadName != "" && prevThreadName != thread.Name {
			thread.Status.PreviousThreadName = prevThreadName
		}
		thread.Status.LastRunName = run.Name
		thread.Status.LastRunState = run.Status.State
		if err := c.Status().Update(ctx, thread); err != nil {
			return err
		}
	}

	return retErr
}

func (i *Invoker) stream(ctx context.Context, c kclient.Client, prevThreadName string, thread *v1.Thread, run *v1.Run, runResp *gptscript.Run) (retErr error) {
	var (
		runEvent = runResp.Events()
		wg       sync.WaitGroup
	)
	defer func() {
		// Don't use parent context because it may be canceled and we still want to save the state
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		retErr = i.saveState(ctx, c, prevThreadName, thread, run, runResp, retErr)
		if retErr != nil {
			log.Errorf("failed to save state: %v", retErr)
			i.events.SubmitProgress(run, types.Progress{
				RunID: run.Name,
				Error: retErr.Error(),
			})
		}
	}()

	defer wg.Wait()

	saveCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-saveCtx.Done():
				return
			case <-time.After(time.Second):
				_ = i.saveState(ctx, c, prevThreadName, thread, run, runResp, nil)
			}
		}
	}()

	defer func() {
		_ = runResp.Close()
		// drain the events in situation of an error
		for range runEvent {
		}
	}()

	runCtx, cancelRun := context.WithCancelCause(ctx)
	defer cancelRun(retErr)

	var (
		abortTimeout = func() {}
		prg          *gptscript.Program
	)

	for {
		select {
		case <-runCtx.Done():
			return context.Cause(runCtx)
		case frame, ok := <-runEvent:
			if !ok {
				if errOut := runResp.ErrorOutput(); errOut != "" {
					return errors.New(errOut)
				}
				return runResp.Err()
			}

			if frame.Run != nil {
				if frame.Run.Type == gptscript.EventTypeRunStart {
					prg = &frame.Run.Program
				}
			}

			if frame.Prompt != nil {
				msg := "\n" + frame.Prompt.Message
				if !strings.HasSuffix(msg, "\n") {
					msg += "\n"
				}
				i.events.SubmitProgress(run, types.Progress{
					RunID:   run.Name,
					Content: msg,
					Prompt: &types.Prompt{
						ID:        frame.Prompt.ID,
						Time:      types.NewTime(frame.Prompt.Time),
						Message:   frame.Prompt.Message,
						Fields:    frame.Prompt.Fields,
						Sensitive: frame.Prompt.Sensitive,
						Metadata:  frame.Prompt.Metadata,
					},
				})
				if len(frame.Prompt.Fields) == 0 {
					err := i.gptClient.PromptResponse(runCtx, gptscript.PromptResponse{
						ID: frame.Prompt.ID,
						Responses: map[string]string{
							"handled": "true",
						},
					})
					if err != nil {
						return err
					}
				}
				timeoutCtx, timoutCancel := context.WithCancel(ctx)
				abortTimeout = timoutCancel
				go func() {
					defer timoutCancel()
					select {
					case <-timeoutCtx.Done():
					case <-time.After(5 * time.Minute):
						cancelRun(fmt.Errorf("timeout waiting for prompt response from user"))
					}
				}()
			}

			if frame.Call != nil {
				switch frame.Call.Type {
				case gptscript.EventTypeCallProgress, gptscript.EventTypeCallFinish:
					abortTimeout()
					fallthrough
				case gptscript.EventTypeCallStart:
					i.events.Submit(run, prg, runResp.Calls())
				}
			}
		}
	}
}
