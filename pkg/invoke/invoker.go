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
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/controller/handlers/retention"
	"github.com/obot-platform/obot/pkg/events"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/gz"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/jwt/ephemeral"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/projects"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	threadmodel "github.com/obot-platform/obot/pkg/thread"
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
	gptClient         *gptscript.GPTScript
	uncached          kclient.WithWatch
	gatewayClient     *client.Client
	tokenService      *ephemeral.TokenService
	mcpSessionManager *mcp.SessionManager
	events            *events.Emitter
	serverURL         string
	serverPort        int
}

func NewInvoker(c kclient.WithWatch, gptClient *gptscript.GPTScript, gatewayClient *client.Client, mcpSessionManager *mcp.SessionManager, serverURL string, serverPort int, tokenService *ephemeral.TokenService, events *events.Emitter) *Invoker {
	return &Invoker{
		uncached:          c,
		gptClient:         gptClient,
		gatewayClient:     gatewayClient,
		tokenService:      tokenService,
		mcpSessionManager: mcpSessionManager,
		events:            events,
		serverURL:         serverURL,
		serverPort:        serverPort,
	}
}

type Response struct {
	Run               *v1.Run
	Thread            *v1.Thread
	WorkflowExecution *v1.WorkflowExecution
	Events            <-chan types.Progress
	Message           string

	uncached      kclient.WithWatch
	gatewayClient *client.Client
	cancel        func()
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
	if r.uncached == nil || r.gatewayClient == nil {
		panic("can not get resource of asynchronous task")
	}
	//nolint:revive
	for range r.Events {
	}

	runState, err := pollRunState(ctx, r.gatewayClient, r.Run, func(run *gtypes.RunState) (bool, error) {
		return run.Done, nil
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

	if runState.Error != "" {
		return TaskResult{}, ErrToolResult{
			Message: runState.Error,
		}
	}

	var (
		errString string
		content   string
		data      = map[string]any{}
	)

	if err := gz.Decompress(&content, runState.Output); err != nil {
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

func pollRunState(ctx context.Context, c *client.Client, run *v1.Run, done func(*gtypes.RunState) (bool, error)) (*gtypes.RunState, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			r, err := c.RunState(ctx, run.Namespace, run.Name)
			if err != nil {
				return nil, err
			}
			if stop, err := done(r); err != nil {
				return nil, err
			} else if stop {
				return r, nil
			}
		}
	}
}

type Options struct {
	Synchronous           bool
	EphemeralThread       bool
	Thread                *v1.Thread
	ThreadName            string
	ParentThreadName      string
	WorkflowName          string
	WorkflowStepName      string
	WorkflowStepID        string
	WorkflowExecutionName string
	PreviousRunName       string
	ForceNoResume         bool
	CreateThread          bool
	CredentialContextIDs  []string
	UserUID               string
	IgnoreMCPErrors       bool
	GenerateName          string
	ExtraEnv              []string
}

func (i *Invoker) getChatState(ctx context.Context, c kclient.Client, run *v1.Run) (result string, _ error) {
	if run.Status.State == v1.Waiting {
		if run.Status.ExternalCall == nil {
			return "", fmt.Errorf("invalid state, external call is unset")
		}
		id := v1.RunStateNameWithExternalID(run.Name, run.Status.ExternalCall.ID)
		lastRun, err := i.gatewayClient.RunState(ctx, run.Namespace, id)
		if apierror.IsNotFound(err) {
			// Copy existing state for future idempotent calls
			lastRun, err = i.gatewayClient.RunState(ctx, run.Namespace, run.Name)
			if err != nil {
				return "", err
			}
			lastRun.Name = id
			if err := i.gatewayClient.CreateRunState(ctx, lastRun); err != nil {
				return "", err
			}
		} else if err != nil {
			return "", err
		}
		return result, gz.Decompress(&result, lastRun.ChatState)
	}

	if run.Spec.PreviousRunName == "" {
		return "", nil
	}

	for {
		// look for the last valid state
		var previousRun v1.Run
		if err := c.Get(ctx, router.Key(run.Namespace, run.Spec.PreviousRunName), &previousRun); err != nil {
			if !apierror.IsNotFound(err) {
				return "", err
			}
			// If not found, use the uncached client
			if err := i.uncached.Get(ctx, router.Key(run.Namespace, run.Spec.PreviousRunName), &previousRun); err != nil {
				return "", err
			}
		}
		if previousRun.Status.State == v1.RunStateState(gptscript.Continue) {
			break
		}
		if previousRun.Spec.PreviousRunName == "" {
			return "", nil
		}
		run = &previousRun
	}

	lastRun, err := i.gatewayClient.RunState(ctx, run.Namespace, run.Spec.PreviousRunName)
	if apierror.IsNotFound(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	if len(lastRun.ChatState) == 0 {
		return "", nil
	}

	return result, gz.Decompress(&result, lastRun.ChatState)
}

func getThreadForAgent(ctx context.Context, c kclient.WithWatch, agent *v1.Agent, opt Options) (*v1.Thread, error) {
	if opt.ThreadName != "" {
		var thread v1.Thread
		return &thread, c.Get(ctx, router.Key(agent.Namespace, opt.ThreadName), &thread)
	}

	var parentThreadName string
	if opt.ParentThreadName != "" {
		parentThreadName = opt.ParentThreadName
	} else if opt.PreviousRunName != "" {
		var run v1.Run
		if err := c.Get(ctx, router.Key(agent.Namespace, opt.PreviousRunName), &run); err != nil {
			return nil, err
		}
		parentThreadName = run.Spec.ThreadName
	}

	return createThreadForAgent(ctx, c, agent, opt.ThreadName, parentThreadName, opt.UserUID, opt.EphemeralThread)
}

func CreateProjectFromProject(ctx context.Context, c kclient.WithWatch, projectThread *v1.Thread, threadName, userUID string) (*v1.Thread, error) {
	thread := v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			Name:       threadName,
			Namespace:  projectThread.Namespace,
			Finalizers: []string{v1.ThreadFinalizer},
		},
		Spec: v1.ThreadSpec{
			Manifest: types.ThreadManifest{
				ThreadManifestManagedFields: types.ThreadManifestManagedFields{
					Name:        projectThread.Spec.Manifest.Name,
					Description: projectThread.Spec.Manifest.Description,
					Icons:       projectThread.Spec.Manifest.Icons,
				},
				Prompt: projectThread.Spec.Manifest.Prompt,
			},
			AgentName:        projectThread.Spec.AgentName,
			ParentThreadName: projectThread.Name,
			UserID:           userUID,
			Project:          true,
		},
	}

	return &thread, c.Create(ctx, &thread)
}

func createThreadForAgent(ctx context.Context, c kclient.WithWatch, agent *v1.Agent, threadName, parentThreadName, userID string, ephemeral bool) (*v1.Thread, error) {
	thread := &v1.Thread{
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
			Ephemeral:        ephemeral,
			AgentName:        agent.Name,
			ParentThreadName: parentThreadName,
			UserID:           userID,
		},
	}

	return thread, c.Create(ctx, thread)
}

func (i *Invoker) Thread(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, input string, opt Options) (*Response, error) {
	var agent v1.Agent
	if err := c.Get(ctx, router.Key(thread.Namespace, thread.Spec.AgentName), &agent); err != nil {
		return nil, err
	}
	opt.Thread = thread
	return i.Agent(ctx, c, &agent, input, opt)
}

func (i *Invoker) Agent(ctx context.Context, c kclient.WithWatch, agent *v1.Agent, input string, opt Options) (*Response, error) {
	thread := opt.Thread
	if thread == nil {
		var err error
		thread, err = getThreadForAgent(ctx, c, agent, opt)
		if err != nil {
			return nil, err
		}
	}

	if thread.Spec.AgentName != agent.Name {
		return nil, fmt.Errorf("thread %q is not associated with agent %q", thread.Name, agent.Name)
	}

	if err := unAbortThread(ctx, c, thread); err != nil {
		return nil, err
	}

	var (
		credContextIDs []string
		err            error
	)
	if opt.CredentialContextIDs != nil {
		credContextIDs = opt.CredentialContextIDs
	} else {
		credContextIDs = []string{thread.Name}
		if thread.Spec.ParentThreadName != "" {
			credContextIDs, err = projects.ThreadIDs(ctx, c, thread)
			if err != nil {
				return nil, err
			}
			credContextIDs[0] = credContextIDs[1] + "-local"
		}
		if agent.Name != "" {
			credContextIDs = append(credContextIDs, agent.Name)
		}
		if agent.Namespace != "" {
			credContextIDs = append(credContextIDs, agent.Namespace)
		}
	}

	renderedAgent, err := render.Agent(ctx, i.tokenService, i.mcpSessionManager, c, agent, i.serverURL, render.AgentOptions{
		Thread:          thread,
		WorkflowStepID:  opt.WorkflowStepID,
		UserID:          opt.UserUID,
		IgnoreMCPErrors: opt.IgnoreMCPErrors,
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

	resp, err := i.createRun(ctx, c, thread, renderedAgent.Tools, input, runOptions{
		Synchronous:           opt.Synchronous,
		WorkflowName:          opt.WorkflowName,
		AgentName:             agent.Name,
		Env:                   append(renderedAgent.Env, opt.ExtraEnv...),
		CredentialContextIDs:  credContextIDs,
		WorkflowStepName:      opt.WorkflowStepName,
		WorkflowStepID:        opt.WorkflowStepID,
		WorkflowExecutionName: opt.WorkflowExecutionName,
		PreviousRunName:       opt.PreviousRunName,
		ForceNoResume:         opt.ForceNoResume,
		GenerateName:          opt.GenerateName,
		UserID:                opt.UserUID,
	})
	if err != nil {
		return nil, err
	}

	if len(renderedAgent.MCPErrors) > 0 {
		resp.Message = fmt.Sprintf("Your chat message was sent successfully. However, there were errors listing tools for some of the MCP servers:\n\n%s", strings.Join(renderedAgent.MCPErrors, "\n"))
	}

	return resp, nil
}

func unAbortThread(ctx context.Context, c kclient.Client, thread *v1.Thread) error {
	if thread.Spec.Abort {
		thread.Spec.Abort = false
		return c.Update(ctx, thread)
	}
	return nil
}

type runOptions struct {
	GenerateName          string
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
	UserID                string
}

func isEphemeral(run *v1.Run) bool {
	return strings.HasPrefix(run.Name, ephemeralRunPrefix)
}

func (i *Invoker) createRun(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, tool any, input string, opts runOptions) (*Response, error) {
	if thread.Spec.Project && !opts.Ephemeral {
		return nil, fmt.Errorf("project threads cannot be invoked")
	}

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

	generateName := opts.GenerateName
	if generateName == "" {
		generateName = system.RunPrefix
	}

	run := v1.Run{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: generateName,
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
			Timeout:               metav1.Duration{Duration: opts.Timeout},
		},
	}

	if opts.UserID != "" {
		u, err := i.gatewayClient.UserByID(ctx, opts.UserID)
		if err != nil {
			return nil, err
		}
		run.Spec.Username = u.DisplayName
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
			if err := i.uncached.Get(ctx, kclient.ObjectKeyFromObject(thread), thread); err != nil {
				return err
			}
			thread.Status.CurrentRunName = run.Name
			if err := retention.SetLastUsedTime(ctx, c, thread); err != nil {
				return err
			}
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
	resp.gatewayClient = i.gatewayClient
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
			errStr, _, _ := strings.Cut(err.Error(), ": exit status")
			i.events.SubmitProgress(run, types.Progress{
				RunID: run.Name,
				Time:  types.NewTime(time.Now()),
				Error: errStr,
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
			return thread.Status.Created, nil
		})
		if err != nil {
			return fmt.Errorf("failed to wait for thread to be ready: %w", err)
		}
	}

	input := run.Spec.Input
	if run.Status.State == v1.Waiting {
		if run.Status.ExternalCall == nil {
			return fmt.Errorf("invalid state, external call should be set for waiting run")
		}

		found := false
		for _, newInput := range run.Spec.ExternalCallResults {
			if newInput.ID == run.Status.ExternalCall.ID {
				inputData, err := json.Marshal(v1.ExternalCallResume{
					Type:   "obotExternalCallResume",
					Call:   *run.Status.ExternalCall,
					Result: newInput,
				})
				if err != nil {
					return fmt.Errorf("failed to marshal external call resume: %w", err)
				}
				input = string(inputData)
				found = true
				break
			}
		}

		if !found {
			// Still waiting for input
			return nil
		}
	}

	chatState, err := i.getChatState(ctx, c, run)
	if err != nil {
		return fmt.Errorf("failed to get chat state: %w", err)
	}

	var userID, userName, userEmail, userTimezone string
	var userGroups []string
	if thread.Spec.UserID != "" && thread.Spec.UserID != "anonymous" && thread.Spec.UserID != "nobody" {
		u, err := i.gatewayClient.UserByID(ctx, thread.Spec.UserID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}

		userID, userName, userEmail, userTimezone = thread.Spec.UserID, u.Username, u.Email, u.Timezone

		// Add groups based on user's role
		userGroups = u.Role.Groups()
		// Note: AuthenticatedGroup is added by default in the token service
	}

	model, modelProvider, err := threadmodel.GetModelAndModelProviderForThread(ctx, c, thread)
	if err != nil {
		return fmt.Errorf("failed to get model and model provider: %w", err)
	}

	project, err := projects.GetRoot(ctx, c, thread)
	if err != nil {
		return fmt.Errorf("failed to get root project: %w", err)
	}

	token, err := i.tokenService.NewToken(ephemeral.TokenContext{
		Namespace:         run.Namespace,
		RunID:             run.Name,
		ThreadID:          thread.Name,
		ProjectID:         thread.Spec.ParentThreadName,
		TopLevelProjectID: project.Name,
		ModelProvider:     modelProvider,
		Model:             model,
		AgentID:           run.Spec.AgentName,
		WorkflowID:        run.Spec.WorkflowName,
		WorkflowStepID:    run.Spec.WorkflowStepID,
		Scope:             thread.Namespace,
		UserID:            userID,
		UserName:          userName,
		UserEmail:         userEmail,
		UserGroups:        userGroups,
	})
	if err != nil {
		return err
	}

	modelProvider, err = render.ResolveToolReference(ctx, c, types.ToolReferenceTypeSystem, thread.Namespace, system.ModelProviderTool)
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
				"OBOT_PROJECT_ID="+thread.Spec.ParentThreadName,
				"OBOT_WORKFLOW_ID="+run.Spec.WorkflowName,
				"OBOT_WORKFLOW_STEP_ID="+run.Spec.WorkflowStepID,
				"OBOT_AGENT_ID="+run.Spec.AgentName,
				"OBOT_DEFAULT_LLM_MODEL="+model,
				"OBOT_DEFAULT_LLM_MINI_MODEL="+string(types.DefaultModelAliasTypeLLMMini),
				"OBOT_DEFAULT_TEXT_EMBEDDING_MODEL="+string(types.DefaultModelAliasTypeTextEmbedding),
				"OBOT_DEFAULT_IMAGE_GENERATION_MODEL="+string(types.DefaultModelAliasTypeImageGeneration),
				"OBOT_DEFAULT_VISION_MODEL="+string(types.DefaultModelAliasTypeVision),
				"OBOT_USER_ID="+userID,
				"OBOT_USER_NAME="+userName,
				"OBOT_USER_EMAIL="+userEmail,
				"OBOT_USER_TIMEZONE="+userTimezone,
				"GPTSCRIPT_HTTP_ENV=OBOT_TOKEN,OBOT_RUN_ID,OBOT_THREAD_ID,OBOT_PROJECT_ID,OBOT_WORKFLOW_ID,OBOT_WORKFLOW_STEP_ID,OBOT_AGENT_ID",
			),
			DefaultModel:         model,
			DefaultModelProvider: modelProvider,
		},
		Input:              input,
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
			return fmt.Errorf("failed to resolve tool reference: %w", err)
		}
		runResp, err = i.gptClient.Run(ctx, toolRef, options)
		if err != nil {
			return fmt.Errorf("failed to run tool: %w", err)
		}
	case '[':
		if err := json.Unmarshal([]byte(run.Spec.Tool), &toolDefs); err != nil {
			return fmt.Errorf("invalid tool definition: %s: %w", run.Spec.Tool, err)
		}
		runResp, err = i.gptClient.Evaluate(ctx, options, toolDefs...)
		if err != nil {
			return fmt.Errorf("failed to evaluate tool: %w", err)
		}
	case '{':
		if err := json.Unmarshal([]byte(run.Spec.Tool), &toolDef); err != nil {
			return fmt.Errorf("invalid tool definition: %s: %w", run.Spec.Tool, err)
		}
		runResp, err = i.gptClient.Evaluate(ctx, options, toolDef)
		if err != nil {
			return fmt.Errorf("failed to evaluate tool: %w", err)
		}
	default:
		return fmt.Errorf("invalid tool definition: %s", run.Spec.Tool)
	}

	if err := i.stream(ctx, c, thread, run, runResp); err != nil {
		return fmt.Errorf("failed to stream: %w", err)
	}

	return nil
}

func (i *Invoker) saveState(ctx context.Context, c kclient.Client, thread *v1.Thread, run *v1.Run, runResp *gptscript.Run, retErr error) error {
	errs := []error{retErr}

	if isEphemeral(run) {
		// Ephemeral run, don't save state
		return errors.Join(errs...)
	}

	var err error
	for j := 0; j < 3; j++ {
		err = i.doSaveState(ctx, c, thread, run, runResp, retErr)
		if err == nil {
			return errors.Join(errs...)
		}
		if !apierror.IsConflict(err) {
			return errors.Join(append(errs, err)...)
		}
		// reload
		if err = c.Get(ctx, router.Key(run.Namespace, run.Name), run); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err = c.Get(ctx, router.Key(thread.Namespace, thread.Name), thread); err != nil {
			return errors.Join(append(errs, err)...)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if combinedError := errors.Join(append(errs, err)...); combinedError != nil {
		return fmt.Errorf("failed to save state after 3 retries: %w", combinedError)
	}
	return retErr
}

func (i *Invoker) doSaveState(ctx context.Context, c kclient.Client, thread *v1.Thread, run *v1.Run, runResp *gptscript.Run, retErr error) error {
	var (
		runStateSpec gtypes.RunState
		extCall      *v1.ExternalCall
		runChanged   bool
		err          error
	)

	runStateSpec.Name = run.Name
	runStateSpec.Namespace = run.Namespace
	runStateSpec.UserID = thread.Spec.UserID
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

			extCall = toExternalCall(text)
			if extCall != nil {
				// waiting state
				runStateSpec.Done = false
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

	runState, err := i.gatewayClient.RunState(ctx, run.Namespace, run.Name)
	if apierror.IsNotFound(err) {
		runState = &gtypes.RunState{
			UserID:     thread.Spec.UserID,
			Name:       run.Name,
			Namespace:  run.Namespace,
			ThreadName: runStateSpec.ThreadName,
			Program:    runStateSpec.Program,
			ChatState:  runStateSpec.ChatState,
			CallFrame:  runStateSpec.CallFrame,
			Output:     runStateSpec.Output,
			Done:       runStateSpec.Done,
			Error:      runStateSpec.Error,
		}
		if err = i.gatewayClient.CreateRunState(ctx, runState); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		if !bytes.Equal(runState.CallFrame, runStateSpec.CallFrame) ||
			!bytes.Equal(runState.ChatState, runStateSpec.ChatState) ||
			runState.Done != runStateSpec.Done ||
			runState.Error != runStateSpec.Error {
			*runState = runStateSpec
			if err = i.gatewayClient.UpdateRunState(ctx, runState); err != nil {
				return err
			}
		}
	}

	state := v1.RunStateState(runResp.State())
	if state == v1.Continue && extCall != nil {
		state = v1.Waiting
	}

	if run.Status.State != state {
		run.Status.State = state
		runChanged = true
	}

	var final bool
	switch state {
	case v1.Error:
		final = true
		errString := runResp.ErrorOutput()
		if errString == "" {
			errString = runResp.Err().Error()
		}
		if run.Status.Error != errString {
			run.Status.Error = errString
			runChanged = true
		}
	case v1.Continue, v1.Finished, v1.Waiting:
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
			if run.Status.ExternalCall == nil {
				runChanged = true
			}
			run.Status.ExternalCall = extCall
			if run.Status.ExternalCall == nil {
				run.Status.Output = shortText
			}
		}
	}

	if retErr != nil && !gptscript.RunState(run.Status.State).IsTerminal() {
		run.Status.State = v1.RunStateState(gptscript.Error)
		if run.Status.Error == "" {
			run.Status.Error = retErr.Error()
		}
		runChanged = true
	}

	if runChanged {
		if run.Status.ExternalCall != nil && run.Status.State != v1.Waiting {
			run.Status.ExternalCall = nil // clear external call if we are done
		}
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
			thread.Status.CurrentRunName = ""
			if err := retention.SetLastUsedTime(ctx, c, thread); err != nil {
				return err
			}

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

	return nil
}

func toExternalCall(output string) *v1.ExternalCall {
	var call v1.ExternalCall
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &call); err != nil || call.Type != "obotExternalCall" || call.ID == "" {
		return nil
	}
	return &call
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

func (i *Invoker) stream(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, run *v1.Run, runResp *gptscript.Run) (retErr error) {
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
		retErr = i.saveState(ctx, c, thread, run, runResp, retErr)
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
				_ = i.saveState(ctx, c, thread, run, runResp, nil)
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
		go i.watchThreadAbort(runCtx, c, thread, cancelRun, runResp)
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
					Fields:      types.ToFields(frame.Prompt.Fields),
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
				timeoutCtx, timeoutCancel := context.WithCancel(ctx)
				abortTimeout = timeoutCancel
				go func() {
					defer timeoutCancel()
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

func (i *Invoker) watchThreadAbort(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, cancel context.CancelCauseFunc, run *gptscript.Run) {
	_, _ = wait.For(ctx, c, thread, func(thread *v1.Thread) (bool, error) {
		if thread.Spec.Abort {
			// we should abort aggressive in the task so that the next step in task won't continue
			if thread.Spec.WorkflowExecutionName != "" {
				cancel(fmt.Errorf("thread was aborted, cancelling run"))
				return true, nil
			}
			if err := i.gptClient.AbortRun(ctx, run); err != nil {
				return false, err
			}
			// cancel the context after 30 seconds in case the abort doesn't work
			go timeoutAfter(ctx, cancel, 30*time.Second)
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
