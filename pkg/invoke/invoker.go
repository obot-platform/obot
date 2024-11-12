package invoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/nah/pkg/router"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/logger"
	"github.com/otto8-ai/otto8/pkg/events"
	"github.com/otto8-ai/otto8/pkg/gz"
	"github.com/otto8-ai/otto8/pkg/hash"
	"github.com/otto8-ai/otto8/pkg/jwt"
	"github.com/otto8-ai/otto8/pkg/render"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"github.com/otto8-ai/otto8/pkg/wait"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Invoker struct {
	gptClient               *gptscript.GPTScript
	uncached                kclient.WithWatch
	tokenService            *jwt.TokenService
	events                  *events.Emitter
	threadWorkspaceProvider string
	serverURL               string
}

func NewInvoker(c kclient.WithWatch, gptClient *gptscript.GPTScript, serverURL, workspaceProviderType string, tokenService *jwt.TokenService, events *events.Emitter) *Invoker {
	return &Invoker{
		uncached:                c,
		gptClient:               gptClient,
		tokenService:            tokenService,
		events:                  events,
		threadWorkspaceProvider: workspaceProviderType,
		serverURL:               serverURL,
	}
}

type Response struct {
	Run    *v1.Run
	Thread *v1.Thread
	Events <-chan types.Progress

	uncached kclient.WithWatch
	cancel   func()
}

type TaskResult struct {
	// Task output
	Output string
}

func (r *Response) Close() {
	r.cancel()
	for range r.Events {
	}
}

type ErrToolResult struct {
	Message string
}

func (e ErrToolResult) Error() string {
	return e.Message
}

func (r *Response) Result(ctx context.Context) (TaskResult, error) {
	if r.uncached == nil {
		panic("can not get resource of asynchronous task")
	}
	for range r.Events {
	}

	runState, err := wait.For(ctx, r.uncached, &v1.RunState{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Run.Name,
			Namespace: r.Run.Namespace,
		},
	}, func(run *v1.RunState) bool {
		return run.Spec.Done
	})
	if apierror.IsNotFound(err) {
		return TaskResult{}, ErrToolResult{
			Message: "run not found",
		}
	} else if err != nil {
		return TaskResult{}, err
	}

	if runState.Spec.Error != "" {
		return TaskResult{}, ErrToolResult{
			Message: runState.Spec.Error,
		}
	}

	var (
		errString string
		content   string
		data      = map[string]any{}
	)

	if err := gz.Decompress(&content, runState.Spec.Output); err != nil {
		return TaskResult{}, err
	}

	_ = json.Unmarshal([]byte(content), &data)
	if err, ok := data["error"].(string); ok {
		errString = err
	}

	if errString != "" {
		return TaskResult{}, ErrToolResult{
			Message: errString,
		}
	}
	return TaskResult{
		Output: content,
	}, nil
}

type Options struct {
	Synchronous           bool
	ThreadName            string
	WorkflowStepName      string
	WorkflowStepID        string
	PreviousRunName       string
	ForceNoResume         bool
	CreateThread          bool
	ThreadCredentialScope *bool
	UserUID               string
	AgentRefName          string
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

	if len(lastRun.Spec.ChatState) == 0 {
		return "", lastRun.Spec.ThreadName, nil
	}
	err := gz.Decompress(&result, lastRun.Spec.ChatState)
	return result, lastRun.Spec.ThreadName, err
}

func getThreadForAgent(ctx context.Context, c kclient.WithWatch, agent *v1.Agent, opt Options) (*v1.Thread, error) {
	if opt.ThreadName != "" {
		var thread v1.Thread
		return &thread, c.Get(ctx, router.Key(agent.Namespace, opt.ThreadName), &thread)
	}

	return createThreadForAgent(ctx, c, agent, opt.ThreadName, opt.UserUID, opt.AgentRefName)
}

func createThreadForAgent(ctx context.Context, c kclient.WithWatch, agent *v1.Agent, threadName, userUID, agentRefName string) (*v1.Thread, error) {
	var (
		fromWorkspaceNames []string
		err                error
	)

	if agent.Name != "" {
		agent, err = wait.For(ctx, c, agent, func(agent *v1.Agent) bool {
			return agent.Status.WorkspaceName != ""
		})
		if err != nil {
			return nil, err
		}
		fromWorkspaceNames = []string{agent.Status.WorkspaceName}
	}

	thread := v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix,
			Name:         threadName,
			Namespace:    agent.Namespace,
		},
		Spec: v1.ThreadSpec{
			Manifest: types.ThreadManifest{
				Tools: agent.Spec.Manifest.DefaultThreadTools,
			},
			AgentName:          agent.Name,
			FromWorkspaceNames: fromWorkspaceNames,
			UserUID:            userUID,
			AgentRefName:       agentRefName,
		},
	}
	return &thread, c.Create(ctx, &thread)
}

func (i *Invoker) updateThreadFields(ctx context.Context, c kclient.WithWatch, agent *v1.Agent, thread *v1.Thread, opt Options) error {
	var updated bool
	if opt.AgentRefName != "" && thread.Spec.AgentRefName != opt.AgentRefName {
		thread.Spec.AgentRefName = opt.AgentRefName
		updated = true
	}
	if thread.Spec.AgentName != agent.Name {
		thread.Spec.AgentName = agent.Name
		updated = true
	}
	if thread.Spec.UserUID != opt.UserUID {
		thread.Spec.UserUID = opt.UserUID
		updated = true
	}
	if updated {
		return c.Status().Update(ctx, thread)
	}
	return nil
}

func (i *Invoker) Agent(ctx context.Context, c kclient.WithWatch, agent *v1.Agent, input string, opt Options) (_ *Response, err error) {
	thread, err := getThreadForAgent(ctx, c, agent, opt)
	if apierror.IsNotFound(err) && opt.CreateThread && strings.HasPrefix(opt.ThreadName, system.ThreadPrefix) {
		thread, err = createThreadForAgent(ctx, c, agent, opt.ThreadName, opt.UserUID, opt.AgentRefName)
	}
	if err != nil {
		return nil, err
	}

	if err := i.updateThreadFields(ctx, c, agent, thread, opt); err != nil {
		return nil, err
	}

	credContextIDs := []string{thread.Name}
	if opt.ThreadCredentialScope != nil && !*opt.ThreadCredentialScope {
		credContextIDs = nil
	}
	if agent.Spec.CredentialContextID != "" {
		credContextIDs = append(credContextIDs, agent.Spec.CredentialContextID)
	} else if agent.Name != "" {
		credContextIDs = append(credContextIDs, agent.Name)
	}
	credContextIDs = append(credContextIDs, agent.Namespace)

	tools, extraEnv, err := render.Agent(ctx, c, agent, i.serverURL, render.AgentOptions{
		Thread: thread,
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

	defaultModel := agent.Spec.Manifest.Model
	if defaultModel == "" {
		var models v1.ModelList
		if err := c.List(ctx, &models, &kclient.ListOptions{
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.manifest.default": "true",
			}),
			Namespace: agent.Namespace,
		}); err != nil {
			return nil, err
		}

		if len(models.Items) > 0 {
			for _, model := range models.Items {
				if model.Spec.Manifest.Active {
					defaultModel = model.Name
					break
				}
			}
		}
	}

	return i.createRun(ctx, c, thread, tools, input, runOptions{
		Synchronous:          opt.Synchronous,
		AgentName:            agent.Name,
		DefaultModel:         defaultModel,
		Env:                  extraEnv,
		CredentialContextIDs: credContextIDs,
		WorkflowStepName:     opt.WorkflowStepName,
		WorkflowStepID:       opt.WorkflowStepID,
		PreviousRunName:      opt.PreviousRunName,
		ForceNoResume:        opt.ForceNoResume,
	})
}

type runOptions struct {
	AgentName             string
	Synchronous           bool
	WorkflowName          string
	WorkflowExecutionName string
	WorkflowStepName      string
	WorkflowStepID        string
	PreviousRunName       string
	ForceNoResume         bool
	Env                   []string
	CredentialContextIDs  []string
	DefaultModel          string
}

var (
	synchronousPending = map[string]struct{}{}
	synchrounousLock   sync.Mutex
)

func (i *Invoker) IsSynchronousPending(runName string) bool {
	synchrounousLock.Lock()
	defer synchrounousLock.Unlock()
	_, ok := synchronousPending[runName]
	return ok
}

func (i *Invoker) createRun(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, tool any, input string, opts runOptions) (_ *Response, retErr error) {
	previousRunName := thread.Status.LastRunName
	if opts.PreviousRunName != "" {
		previousRunName = opts.PreviousRunName
	}

	if opts.ForceNoResume {
		previousRunName = ""
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
			Synchronous:           opts.Synchronous,
			ThreadName:            thread.Name,
			AgentName:             opts.AgentName,
			WorkflowName:          opts.WorkflowName,
			WorkflowExecutionName: opts.WorkflowExecutionName,
			WorkflowStepName:      opts.WorkflowStepName,
			WorkflowStepID:        opts.WorkflowStepID,
			PreviousRunName:       previousRunName,
			Input:                 input,
			Tool:                  string(toolData),
			Env:                   opts.Env,
			CredentialContextIDs:  opts.CredentialContextIDs,
			DefaultModel:          opts.DefaultModel,
		},
	}

	if err := c.Create(ctx, &run); err != nil {
		return nil, err
	}

	if opts.Synchronous {
		synchrounousLock.Lock()
		synchronousPending[run.Name] = struct{}{}
		synchrounousLock.Unlock()
	}

	defer func() {
		if retErr != nil {
			synchrounousLock.Lock()
			delete(synchronousPending, run.Name)
			synchrounousLock.Unlock()
		}
	}()

	if !thread.Spec.SystemTask {
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := c.Get(ctx, kclient.ObjectKeyFromObject(thread), thread); err != nil {
				return err
			}
			thread.Status.CurrentRunName = run.Name
			return c.Status().Update(ctx, thread)
		})
		if err != nil {
			return nil, err
		}
	}

	resp := &Response{
		Run:    &run,
		Thread: thread,
	}

	if !opts.Synchronous {
		noEvents := make(chan types.Progress)
		close(noEvents)
		resp.Events = noEvents
		resp.cancel = func() {}
		return resp, nil
	}

	ctx, cancel := context.WithCancel(ctx)

	_, events, err := i.events.Watch(ctx, thread.Namespace, events.WatchOptions{
		Run: &run,
	})
	if err != nil {
		cancel()
		// Cleanup orphaned run
		_ = i.uncached.Delete(ctx, &run)
		return nil, err
	}

	resp.Events = events
	resp.uncached = i.uncached
	resp.cancel = cancel
	go func() {
		if err := i.Resume(ctx, c, thread, &run); err != nil {
			log.Errorf("run failed: %v", err)
		}
	}()

	return resp, nil
}

func (i *Invoker) Resume(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, run *v1.Run) error {
	defer func() {
		i.events.Done(run)
		time.AfterFunc(20*time.Second, func() {
			i.events.ClearProgress(run)
		})
	}()

	thread, err := wait.For(ctx, c, thread, func(thread *v1.Thread) bool {
		return thread.Status.WorkspaceID != ""
	})
	if err != nil {
		return fmt.Errorf("failed to wait for thread to be ready: %w", err)
	}

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

	modelProvider, err := render.ResolveToolReference(ctx, c, types.ToolReferenceTypeSystem, thread.Namespace, system.ModelProviderTool)
	if err != nil {
		return fmt.Errorf("failed to resolve model provider: %w", err)
	}

	options := gptscript.Options{
		GlobalOptions: gptscript.GlobalOptions{
			Env: append(run.Spec.Env,
				fmt.Sprintf("GPTSCRIPT_MODEL_PROVIDER_PROXY_URL=%s/api/llm-proxy", i.serverURL),
				"GPTSCRIPT_MODEL_PROVIDER_PROXY_TOKEN="+token,
				"GPTSCRIPT_MODEL_PROVIDER_TOKEN="+token,
				"OTTO8_SERVER_URL="+i.serverURL,
				"OTTO8_TOKEN="+token,
				"OTTO8_RUN_ID="+run.Name,
				"OTTO8_THREAD_ID="+thread.Name,
				"OTTO8_WORKFLOW_ID="+run.Spec.WorkflowName,
				"OTTO8_WORKFLOW_STEP_ID="+run.Spec.WorkflowStepID,
				"OTTO8_AGENT_ID="+run.Spec.AgentName,
				"GPTSCRIPT_HTTP_ENV=OTTO8_TOKEN,OTTO8_RUN_ID,OTTO8_THREAD_ID,OTTO8_WORKFLOW_ID,OTTO8_WORKFLOW_STEP_ID,OTTO8_AGENT_ID",
			),
			DefaultModel:         run.Spec.DefaultModel,
			DefaultModelProvider: modelProvider,
		},
		Input:              run.Spec.Input,
		Workspace:          thread.Status.WorkspaceID,
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
		toolRef, err := render.ResolveToolReference(ctx, c, run.Spec.ToolReferenceType, run.Namespace, toolString)
		if err != nil {
			return err
		}
		runResp, err = i.gptClient.Run(ctx, toolRef, options)
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
	} else if runStateSpec.Done {
		text, err := runResp.Text()
		if err == nil {
			// ignore errors, it will be recorded or handled elsewhere
			runStateSpec.Output, err = gz.Compress(text)
			if err != nil {
				return err
			}
		}
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
		shortText := text
		if len(shortText) > 100 {
			shortText = shortText[:100]
		}
		if run.Status.Output != shortText {
			run.Status.SubCall = toSubCall(text)
			if run.Status.SubCall == nil {
				run.Status.Output = shortText
			}
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
		if run.Status.EndTime.IsZero() && final {
			run.Status.EndTime = metav1.Now()
		}
		if err := c.Status().Update(ctx, run); err != nil {
			return err
		}
	}

	if !thread.Spec.SystemTask {
		var workflowState types.WorkflowState
		if thread.Spec.WorkflowExecutionName != "" {
			var wfe v1.WorkflowExecution
			if err := c.Get(ctx, router.Key(thread.Namespace, thread.Spec.WorkflowExecutionName), &wfe); err == nil {
				workflowState = wfe.Status.State
			}
		}

		if final && thread.Status.LastRunName != run.Name {
			if prevThreadName != "" && prevThreadName != thread.Name {
				thread.Status.PreviousThreadName = prevThreadName
			}
			thread.Status.CurrentRunName = ""
			thread.Status.LastRunName = run.Name
			thread.Status.LastRunState = run.Status.State
			if workflowState != "" {
				thread.Status.WorkflowState = workflowState
			}
			if err := c.Status().Update(ctx, thread); err != nil {
				return err
			}
		} else if workflowState != "" && thread.Status.WorkflowState != workflowState {
			if err := c.Status().Update(ctx, thread); err != nil {
				return err
			}
		}
	}

	return retErr
}

type call struct {
	Type     string `json:"type,omitempty"`
	Workflow string `json:"workflow,omitempty"`
	Input    any    `json:"input,omitempty"`
}

func toSubCall(output string) *v1.SubCall {
	var call call
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &call); err != nil || call.Type != "OttoSubFlow" || call.Workflow == "" {
		return nil
	}

	var inputString string
	switch v := call.Input.(type) {
	case string:
		inputString = v
	default:
		inputBytes, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		inputString = string(inputBytes)
	}

	if inputString == "{}" {
		inputString = ""
	}

	return &v1.SubCall{
		Type:     call.Type,
		Workflow: call.Workflow,
		Input:    inputString,
	}
}

func getCredentialCallingTool(runResp *gptscript.Run) (result gptscript.Tool) {
	calls := runResp.Calls()
	// Look for an in progress cred tool and just assume that's it
	for _, call := range calls {
		if call.ToolCategory == gptscript.CredentialToolCategory && call.End.IsZero() && call.ParentID != "" {
			return calls[call.ParentID].Tool
		}
	}
	return
}

func (i *Invoker) stream(ctx context.Context, c kclient.Client, prevThreadName string, thread *v1.Thread, run *v1.Run, runResp *gptscript.Run) (retErr error) {
	var (
		runEvent = runResp.Events()
		wg       sync.WaitGroup
	)

	// We might modify these objects so make a local copy
	thread = thread.DeepCopyObject().(*v1.Thread)
	run = run.DeepCopyObject().(*v1.Run)

	defer func() {
		// Don't use parent context because it may be canceled and we still want to save the state
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		retErr = i.saveState(ctx, c, prevThreadName, thread, run, runResp, retErr)
		if retErr != nil {
			log.Errorf("failed to save state: %v", retErr)
			i.events.SubmitProgress(run, types.Progress{
				RunID: run.Name,
				Time:  types.NewTime(time.Now()),
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
				callingTool := getCredentialCallingTool(runResp)
				metadata := map[string]string{}
				maps.Copy(metadata, frame.Prompt.Metadata)
				maps.Copy(metadata, callingTool.MetaData)
				prompt := &types.Prompt{
					ID:          frame.Prompt.ID,
					Name:        callingTool.Name,
					Description: callingTool.Description,
					Time:        types.NewTime(frame.Prompt.Time),
					Message:     frame.Prompt.Message,
					Fields:      frame.Prompt.Fields,
					Sensitive:   frame.Prompt.Sensitive,
					Metadata:    metadata,
				}
				contentID := hash.String(prompt)[:8]
				i.events.SubmitProgress(run, types.Progress{
					RunID:     run.Name,
					Content:   msg,
					ContentID: contentID,
					Time:      types.NewTime(time.Now()),
					Prompt:    prompt,
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
				case gptscript.EventTypeCallFinish:
					abortTimeout()
					fallthrough
				case gptscript.EventTypeCallProgress, gptscript.EventTypeCallStart:
					i.events.Submit(run, prg, runResp.Calls())
				}
			}
		}
	}
}
