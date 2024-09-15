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

type Invoker struct {
	gptClient               *gptscript.GPTScript
	uncached                kclient.Client
	tokenService            *jwt.TokenService
	workspaceClient         *wclient.Client
	threadWorkspaceProvider string
	knowledgeTool           string
}

func NewInvoker(c kclient.Client, gptClient *gptscript.GPTScript, tokenService *jwt.TokenService, workspaceClient *wclient.Client, knowledgeTool string) *Invoker {
	return &Invoker{
		uncached:                c,
		gptClient:               gptClient,
		tokenService:            tokenService,
		workspaceClient:         workspaceClient,
		threadWorkspaceProvider: "directory",
		knowledgeTool:           knowledgeTool,
	}
}

type Response struct {
	Run    *v1.Run
	Thread *v1.Thread
	Events <-chan v1.Progress
}

func (r *Response) Wait() {
	if r.Events == nil {
		return
	}
	for range r.Events {
	}
}

type Options struct {
	Background       bool
	ThreadName       string
	PreviousRunName  string
	WorkflowName     string
	WorkflowStepName string
	Env              []string
}

type NewThreadOptions struct {
	AgentName             string
	ThreadName            string
	ThreadGenerateName    string
	WorkflowName          string
	WorkflowExecutionName string
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
			Finalizers:   []string{v1.ThreadFinalizer},
		},
		Spec: v1.ThreadSpec{
			AgentName:             opt.AgentName,
			WorkflowExecutionName: opt.WorkflowExecutionName,
			WorkflowName:          opt.WorkflowName,
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

func (i *Invoker) getChatState(ctx context.Context, c kclient.Client, run *v1.Run) (result string, _ error) {
	if run.Spec.PreviousRunName == "" {
		return "", nil
	}

	for {
		// look for the last valid state
		var previousRun v1.Run
		if err := c.Get(ctx, router.Key(run.Namespace, run.Spec.PreviousRunName), &previousRun); err != nil {
			return "", err
		}
		if previousRun.Status.State == gptscript.Continue {
			break
		}
		if previousRun.Spec.PreviousRunName == "" {
			return "", nil
		}
		run = &previousRun
	}

	var lastRun v1.RunState
	if err := c.Get(ctx, router.Key(run.Namespace, run.Spec.PreviousRunName), &lastRun); apierror.IsNotFound(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	err := gz.Decompress(&result, lastRun.Spec.ChatState)
	return result, err
}

func (i *Invoker) Agent(ctx context.Context, c kclient.Client, agent *v1.Agent, input string, opt Options) (*Response, error) {
	threadOpt := NewThreadOptions{
		AgentName:  agent.Name,
		ThreadName: opt.ThreadName,
	}
	if agent.Status.Workspace.WorkspaceID != "" {
		threadOpt.WorkspaceIDs = append(threadOpt.WorkspaceIDs, agent.Status.Workspace.WorkspaceID)
	}
	if agent.Status.KnowledgeWorkspace.KnowledgeWorkspaceID != "" {
		threadOpt.KnowledgeWorkspaceIDs = append(threadOpt.KnowledgeWorkspaceIDs, agent.Status.KnowledgeWorkspace.KnowledgeWorkspaceID)
	}

	thread, err := i.NewThread(ctx, c, agent.Namespace, threadOpt)
	if err != nil {
		return nil, err
	}

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
		Background:       true,
		AgentName:        agent.Name,
		Env:              append(opt.Env, extraEnv...),
		PreviousRunName:  opt.PreviousRunName,
		WorkflowName:     opt.WorkflowName,
		WorkflowStepName: opt.WorkflowStepName,
	})
}

type runOptions struct {
	AgentName        string
	Background       bool
	WorkflowName     string
	WorkflowStepName string
	PreviousRunName  string
	Env              []string
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
			Background:       opts.Background,
			ThreadName:       thread.Name,
			AgentName:        opts.AgentName,
			WorkflowName:     opts.WorkflowName,
			WorkflowStepName: opts.WorkflowStepName,
			PreviousRunName:  previousRunName,
			Input:            input,
			Tool:             string(toolData),
			Env:              opts.Env,
		},
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

	return i.Resume(ctx, c, thread, &run)
}

func (i *Invoker) Resume(ctx context.Context, c kclient.Client, thread *v1.Thread, run *v1.Run) (*Response, error) {
	chatState, err := i.getChatState(ctx, c, run)
	if err != nil {
		return nil, err
	}

	token, err := i.tokenService.NewToken(jwt.TokenContext{
		RunID:          run.Name,
		ThreadID:       thread.Name,
		AgentID:        run.Spec.AgentName,
		WorkflowID:     run.Spec.WorkflowName,
		WorkflowStepID: run.Spec.WorkflowStepName,
		Scope:          thread.Namespace,
	})

	options := gptscript.Options{
		GlobalOptions: gptscript.GlobalOptions{
			Env: append(run.Spec.Env,
				"OTTO_TOKEN="+token,
				"OTTO_RUN_ID="+run.Name,
				"OTTO_THREAD_ID="+thread.Name,
				"OTTO_WORKFLOW_ID="+run.Spec.WorkflowName,
				"OTTO_WORKFLOW_STEP_ID="+run.Spec.WorkflowStepName,
				"OTTO_AGENT_ID="+run.Spec.AgentName,
			),
		},
		Input:           run.Spec.Input,
		Workspace:       workspace.GetDir(thread.Spec.WorkspaceID),
		ChatState:       chatState,
		IncludeEvents:   true,
		ForceSequential: true,
	}

	if len(run.Spec.Tool) == 0 {
		return nil, fmt.Errorf("no tool specified")
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
			return nil, fmt.Errorf("invalid tool definition: %s: %w", run.Spec.Tool, err)
		}
		runResp, err = i.gptClient.Run(ctx, toolString, options)
		if err != nil {
			return nil, err
		}
	case '[':
		if err := json.Unmarshal([]byte(run.Spec.Tool), &toolDefs); err != nil {
			return nil, fmt.Errorf("invalid tool definition: %s: %w", run.Spec.Tool, err)
		}
		runResp, err = i.gptClient.Evaluate(ctx, options, toolDefs...)
		if err != nil {
			return nil, err
		}
	case '{':
		if err := json.Unmarshal([]byte(run.Spec.Tool), &toolDef); err != nil {
			return nil, fmt.Errorf("invalid tool definition: %s: %w", run.Spec.Tool, err)
		}
		runResp, err = i.gptClient.Evaluate(ctx, options, toolDef)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid tool definition: %s", run.Spec.Tool)
	}

	var events = make(chan v1.Progress)
	go i.stream(ctx, c, events, thread, run, runResp)

	return &Response{
		Run:    run,
		Thread: thread,
		Events: events,
	}, nil
}

func (i *Invoker) saveState(ctx context.Context, c kclient.Client, thread *v1.Thread, run *v1.Run, runResp *gptscript.Run, retErr error) error {
	for j := 0; j < 3; j++ {
		err := i.doSaveState(ctx, c, thread, run, runResp, retErr)
		if err == nil {
			return retErr
		}
		if !apierror.IsConflict(err) {
			return errors.Join(err, retErr)
		}
		// reload
		if err := c.Get(ctx, router.Key(run.Namespace, run.Name), run); err != nil {
			return errors.Join(err, retErr)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("failed to save state after 3 retries: %w", retErr)
}

func (i *Invoker) doSaveState(ctx context.Context, c kclient.Client, thread *v1.Thread, run *v1.Run, runResp *gptscript.Run, retErr error) error {
	var (
		runStateSpec v1.RunStateSpec
		runChanged   bool
		err          error
	)

	runStateSpec.ThreadName = run.Spec.ThreadName

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
			!bytes.Equal(runState.Spec.ChatState, runStateSpec.ChatState) {
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

	if runChanged {
		if err := c.Status().Update(ctx, run); err != nil {
			return err
		}
	}

	if final && thread.Status.LastRunName != run.Name {
		thread.Status.LastRunName = run.Name
		thread.Status.LastRunState = run.Status.State
		thread.Status.LastRunOutput = run.Status.Output
		thread.Status.LastRunError = run.Status.Error
		if err := c.Status().Update(ctx, thread); err != nil {
			return err
		}
	}

	return retErr
}

func (i *Invoker) stream(ctx context.Context, c kclient.Client, events chan v1.Progress, thread *v1.Thread, run *v1.Run, runResp *gptscript.Run) (retErr error) {
	defer close(events)

	var (
		runEvent = runResp.Events()
		wg       sync.WaitGroup
	)
	defer func() {
		retErr = i.saveState(ctx, c, thread, run, runResp, retErr)
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
			case <-time.After(1 * time.Second):
				_ = i.saveState(ctx, c, thread, run, runResp, nil)
			}
		}
	}()

	var (
		lastPrint = map[string][]gptscript.Output{}
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		case frame, ok := <-runEvent:
			if !ok {
				return runResp.Err()
			}

			if frame.Call != nil {
				switch frame.Call.Type {
				case gptscript.EventTypeCallStart:
					if frame.Call.ToolCategory == gptscript.NoCategory && !frame.Call.Tool.Chat {
						events <- v1.Progress{
							Tool: v1.ToolProgress{
								Name:        frame.Call.Tool.Name,
								Description: frame.Call.Tool.Description,
								Input:       frame.Call.Input,
							},
						}
					}
				case gptscript.EventTypeCallProgress, gptscript.EventTypeCallFinish:
					if frame.Call.ToolCategory == gptscript.NoCategory && frame.Call.Tool.Chat {
						printCall(frame.Call, lastPrint, events)
					}
				}
			}
		}
	}
}

func printString(out chan v1.Progress, last, current string) {
	toPrint := current
	if strings.HasPrefix(current, last) {
		toPrint = current[len(last):]
	}

	toPrint, waitingOnModel := strings.CutPrefix(toPrint, "Waiting for model response...")
	toPrint, toolPrint, isToolCall := strings.Cut(toPrint, "<tool call> ")
	toolName := ""

	if isToolCall {
		toolName = strings.Split(toolPrint, " ->")[0]
	} else {
		_, wasToolPrint, wasToolCall := strings.Cut(current, "<tool call> ")
		if wasToolCall {
			toolName = strings.Split(wasToolPrint, " ->")[0]
			toolPrint = toPrint
			toPrint = ""
		}
	}

	toolPrint = strings.TrimPrefix(toolPrint, toolName+" -> ")

	out <- v1.Progress{
		Content: toPrint,
		Tool: v1.ToolProgress{
			GeneratingInputForName: toolName,
			GeneratingInput:        isToolCall,
			PartialInput:           toolPrint,
		},
		WaitingOnModel: waitingOnModel,
	}
}

func printCall(call *gptscript.CallFrame, lastPrint map[string][]gptscript.Output, out chan v1.Progress) {
	lastOutputs := lastPrint[call.ID]
	for i, currentOutput := range call.Output {
		for i >= len(lastOutputs) {
			lastOutputs = append(lastOutputs, gptscript.Output{})
		}
		last := lastOutputs[i]

		if last.Content != currentOutput.Content {
			printString(out, last.Content, currentOutput.Content)
			last.Content = currentOutput.Content
		}
		lastOutputs[i] = currentOutput
	}

	lastPrint[call.ID] = lastOutputs
}
