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
	"sync/atomic"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/uncached"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/events"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gz"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/jwt"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	log              = logger.Package()
	ephemeralCounter atomic.Int32
)

const (
	ephemeralRunPrefix = "ephemeral-run"
	runOutputMaxLength = 2000
)

type Invoker struct {
	gptClient     *gptscript.GPTScript
	uncached      kclient.WithWatch
	gatewayClient *client.Client
	tokenService  *jwt.TokenService
	events        *events.Emitter
	serverURL     string
	serverPort    int
}

func NewInvoker(c kclient.WithWatch, gptClient *gptscript.GPTScript, gatewayClient *client.Client, serverURL string, serverPort int, tokenService *jwt.TokenService, events *events.Emitter) *Invoker {
	return &Invoker{
		uncached:      c,
		gptClient:     gptClient,
		gatewayClient: gatewayClient,
		tokenService:  tokenService,
		events:        events,
		serverURL:     serverURL,
		serverPort:    serverPort,
	}
}

type Response struct {
	Run               *v1.Run
	Thread            *v1.Thread
	WorkflowExecution *v1.WorkflowExecution
	Events            <-chan types.Progress

	uncached kclient.WithWatch
	cancel   func()
}

type TaskResult struct {
	// Task output
	Output string
}

func (r *Response) Close() {
	r.cancel()
	//nolint:revive
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
	//nolint:revive
	for range r.Events {
	}

	runState, err := wait.For(ctx, r.uncached, &v1.RunState{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Run.Name,
			Namespace: r.Run.Namespace,
		},
	}, func(run *v1.RunState) (bool, error) {
		return run.Spec.Done, nil
	})
	if apierror.IsNotFound(err) {
		return TaskResult{}, ErrToolResult{
			Message: "run not found",
		}
	} else if err != nil {
		return TaskResult{}, err
	}

	if runState.Name != r.Run.Name {
		panic("runState doesnt match")
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
	WorkflowExecutionName string
	PreviousRunName       string
	ForceNoResume         bool
	CreateThread          bool
	ThreadCredentialScope *bool
	UserUID               string
	AgentAlias            string
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

	return CreateThreadForAgent(ctx, c, agent, opt.ThreadName, opt.UserUID, opt.AgentAlias)
}

func CreateThreadForAgent(ctx context.Context, c kclient.WithWatch, agent *v1.Agent, threadName, userUID, agentAlias string) (*v1.Thread, error) {
	var (
		fromWorkspaceNames []string
		err                error
	)

	if agent.Name != "" {
		agent, err = wait.For(ctx, c, agent, func(agent *v1.Agent) (bool, error) {
			return agent.Status.WorkspaceName != "" && len(agent.Status.KnowledgeSetNames) > 0, nil
		})
		if err != nil {
			return nil, err
		}
		fromWorkspaceNames = []string{agent.Status.WorkspaceName}
	}

	var agentKnowledgeSet v1.KnowledgeSet
	if err = c.Get(ctx, router.Key(agent.Namespace, agent.Status.KnowledgeSetNames[0]), &agentKnowledgeSet); err != nil {
		return nil, err
	}

	thread := v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix,
			Name:         threadName,
			Namespace:    agent.Namespace,
			Finalizers:   []string{v1.ThreadFinalizer},
		},
		Spec: v1.ThreadSpec{
			Manifest: types.ThreadManifest{
				Tools: agent.Spec.Manifest.DefaultThreadTools,
			},
			AgentName:          agent.Name,
			FromWorkspaceNames: fromWorkspaceNames,
			UserUID:            userUID,
			AgentAlias:         agentAlias,
			TextEmbeddingModel: agentKnowledgeSet.Spec.TextEmbeddingModel,
		},
	}
	return &thread, c.Create(ctx, &thread)
}

func (i *Invoker) updateThreadFields(ctx context.Context, c kclient.WithWatch, agent *v1.Agent, thread *v1.Thread, opt Options) error {
	var updated bool
	if opt.AgentAlias != "" && thread.Spec.AgentAlias != opt.AgentAlias {
		thread.Spec.AgentAlias = opt.AgentAlias
		updated = true
	}
	if thread.Spec.AgentName != agent.Name {
		thread.Spec.AgentName = agent.Name
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
		thread, err = CreateThreadForAgent(ctx, c, agent, opt.ThreadName, opt.UserUID, opt.AgentAlias)
	}
	if err != nil {
		return nil, err
	}

	if err := unAbortThread(ctx, c, thread); err != nil {
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

	if err := i.updateThreadFields(ctx, c, agent, thread, opt); err != nil {
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

	return i.createRun(ctx, c, thread, tools, input, runOptions{
		Synchronous:           opt.Synchronous,
		AgentName:             agent.Name,
		Env:                   extraEnv,
		CredentialContextIDs:  credContextIDs,
		WorkflowStepName:      opt.WorkflowStepName,
		WorkflowStepID:        opt.WorkflowStepID,
		WorkflowExecutionName: opt.WorkflowExecutionName,
		PreviousRunName:       opt.PreviousRunName,
		ForceNoResume:         opt.ForceNoResume,
	})
}

func unAbortThread(ctx context.Context, c kclient.Client, thread *v1.Thread) error {
	if thread.Spec.Abort {
		thread.Spec.Abort = false
		return c.Update(ctx, thread)
	}
	return nil
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
	Timeout               time.Duration
	Ephemeral             bool
}

func isEphemeral(run *v1.Run) bool {
	return strings.HasPrefix(run.Name, ephemeralRunPrefix)
}

func (i *Invoker) createRun(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, tool any, input string, opts runOptions) (_ *Response, retErr error) {
	previousRunName := thread.Status.LastRunName
	if opts.PreviousRunName != "" {
		previousRunName = opts.PreviousRunName
	}

	if opts.ForceNoResume || opts.Ephemeral {
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
			DefaultModel:          string(types.DefaultModelAliasTypeLLM),
			Timeout:               metav1.Duration{Duration: opts.Timeout},
		},
	}

	if opts.Ephemeral {
		run.Name = fmt.Sprintf("%s-%d", ephemeralRunPrefix, ephemeralCounter.Add(1))
	} else {
		if err := c.Create(ctx, &run); err != nil {
			return nil, err
		}
	}

	if !thread.Spec.SystemTask && !opts.Ephemeral {
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			// Ensure that, regardless of which client is being used, we get an uncached version of the thread for updating.
			// The first uncached.Get method ensures that we get an uncached version when calling this from a controller.
			// That will fail when calling this outside a controller, so try a "bare" get in that case.
			if err := c.Get(ctx, kclient.ObjectKeyFromObject(thread), uncached.Get(thread)); err != nil {
				if err := c.Get(ctx, kclient.ObjectKeyFromObject(thread), thread); err != nil {
					return err
				}
			}
			thread.Status.CurrentRunName = run.Name
			return c.Status().Update(ctx, thread)
		})
		if err != nil {
			// Don't return error it's not critical, and will mostly likely make caller loose track of this
			log.Errorf("failed to update thread %q for run %q: %v", thread.Name, run.Name, err)
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

func (i *Invoker) Resume(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, run *v1.Run) (err error) {
	defer func() {
		if err != nil {
			i.events.SubmitProgress(run, types.Progress{
				RunID: run.Name,
				Time:  types.NewTime(time.Now()),
				Error: err.Error(),
			})
		}
		i.events.Done(run)
		time.AfterFunc(20*time.Second, func() {
			i.events.ClearProgress(run)
		})
	}()

	if !isEphemeral(run) {
		thread, err = wait.For(ctx, c, thread, func(thread *v1.Thread) (bool, error) {
			if thread.Spec.Abort {
				return false, fmt.Errorf("thread was aborted while waiting for workspace")
			}
			return thread.Status.WorkspaceID != "", nil
		})
		if err != nil {
			return fmt.Errorf("failed to wait for thread to be ready: %w", err)
		}
	}

	chatState, prevThreadName, err := i.getChatState(ctx, c, run)
	if err != nil {
		return err
	}

	var userID, userName, userEmail, userTimezone string
	if thread.Spec.UserUID != "" && thread.Spec.UserUID != "anonymous" && thread.Spec.UserUID != "nobody" {
		u, err := i.gatewayClient.UserByID(ctx, thread.Spec.UserUID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}

		userID, userName, userEmail, userTimezone = thread.Spec.UserUID, u.Username, u.Email, u.Timezone
	}

	token, err := i.tokenService.NewToken(jwt.TokenContext{
		RunID:          run.Name,
		ThreadID:       thread.Name,
		AgentID:        run.Spec.AgentName,
		WorkflowID:     run.Spec.WorkflowName,
		WorkflowStepID: run.Spec.WorkflowStepID,
		Scope:          thread.Namespace,
		UserID:         userID,
		UserName:       userName,
		UserEmail:      userEmail,
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
				fmt.Sprintf("GPTSCRIPT_MODEL_PROVIDER_PROXY_URL=http://localhost:%d/api/llm-proxy", i.serverPort),
				"GPTSCRIPT_MODEL_PROVIDER_PROXY_TOKEN="+token,
				"GPTSCRIPT_MODEL_PROVIDER_TOKEN="+token,
				"OBOT_SERVER_URL="+i.serverURL,
				"OBOT_TOKEN="+token,
				"OBOT_RUN_ID="+run.Name,
				"OBOT_THREAD_ID="+thread.Name,
				"OBOT_WORKFLOW_ID="+run.Spec.WorkflowName,
				"OBOT_WORKFLOW_STEP_ID="+run.Spec.WorkflowStepID,
				"OBOT_AGENT_ID="+run.Spec.AgentName,
				"OBOT_DEFAULT_LLM_MODEL="+string(types.DefaultModelAliasTypeLLM),
				"OBOT_DEFAULT_LLM_MINI_MODEL="+string(types.DefaultModelAliasTypeLLMMini),
				"OBOT_DEFAULT_TEXT_EMBEDDING_MODEL="+string(types.DefaultModelAliasTypeTextEmbedding),
				"OBOT_DEFAULT_IMAGE_GENERATION_MODEL="+string(types.DefaultModelAliasTypeImageGeneration),
				"OBOT_DEFAULT_VISION_MODEL="+string(types.DefaultModelAliasTypeVision),
				"OBOT_USER_ID="+userID,
				"OBOT_USER_NAME="+userName,
				"OBOT_USER_EMAIL="+userEmail,
				"OBOT_USER_TIMEZONE="+userTimezone,
				"GPTSCRIPT_HTTP_ENV=OBOT_TOKEN,OBOT_RUN_ID,OBOT_THREAD_ID,OBOT_WORKFLOW_ID,OBOT_WORKFLOW_STEP_ID,OBOT_AGENT_ID",
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
	if isEphemeral(run) {
		// Ephemeral run, don't save state
		return retErr
	}

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
		if len(shortText) > runOutputMaxLength {
			shortText = shortText[:runOutputMaxLength]
		}
		if run.Status.Output != shortText {
			if run.Status.SubCall == nil && run.Status.TaskResult == nil {
				runChanged = true
			}
			run.Status.SubCall = toSubCall(text)
			run.Status.TaskResult = toTaskResult(text)
			if run.Status.SubCall == nil && run.Status.TaskResult == nil {
				run.Status.Output = shortText
			}
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

func toTaskResult(output string) *v1.TaskResult {
	var call v1.TaskResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &call); err != nil || call.Type != "ObotTaskResult" || call.ID == "" {
		return nil
	}

	return &call
}

func toSubCall(output string) *v1.SubCall {
	var call call
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &call); err != nil || call.Type != "ObotSubFlow" || call.Workflow == "" {
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

func (i *Invoker) stream(ctx context.Context, c kclient.WithWatch, prevThreadName string, thread *v1.Thread, run *v1.Run, runResp *gptscript.Run) (retErr error) {
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
		// drain the events on error
		//nolint:revive
		for range runEvent {
		}
	}()

	runCtx, cancelRun := context.WithCancelCause(ctx)
	defer cancelRun(retErr)

	timeout := 10 * time.Minute
	if run.Spec.Timeout.Duration > 0 {
		timeout = run.Spec.Timeout.Duration
	}
	go timeoutAfter(runCtx, cancelRun, timeout)
	if !isEphemeral(run) {
		// Don't watch thread abort for ephemeral runs
		go watchThreadAbort(runCtx, c, thread, cancelRun)
	}

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

				var (
					timeoutMsg = "timeout waiting for prompt response from user"
					timeout    = 5 * time.Minute
				)
				if len(frame.Prompt.Fields) == 0 {
					// In this case, we're waiting for an OAuth prompt
					timeoutMsg = "timeout waiting for oauth"
					timeout = 90 * time.Second
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
					case <-time.After(timeout):
						cancelRun(errors.New(timeoutMsg))
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

func watchThreadAbort(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, cancel context.CancelCauseFunc) {
	_, _ = wait.For(ctx, c, thread, func(thread *v1.Thread) (bool, error) {
		if thread.Spec.Abort {
			cancel(fmt.Errorf("thread was aborted, cancelling run"))
			return true, nil
		}
		return false, nil
	}, wait.Option{
		Timeout: 11 * time.Minute,
	})
}

func timeoutAfter(ctx context.Context, cancel func(err error), d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
		cancel(fmt.Errorf("run exceeded maximum time of %v", d))
	}
}
