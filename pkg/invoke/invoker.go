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

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/gz"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

const (
	ephemeralRunPrefix = "ephemeral-run"
	runOutputMaxLength = 2000
)

type Invoker struct {
	gptClient         *gptscript.GPTScript
	uncached          kclient.WithWatch
	gatewayClient     *client.Client
	tokenService      *persistent.TokenService
	serverURL         string
	internalServerURL string
}

func NewInvoker(c kclient.WithWatch, gatewayClient *client.Client, serverURL string, serverPort int, tokenService *persistent.TokenService, gptClient *gptscript.GPTScript) *Invoker {
	return &Invoker{
		uncached:          c,
		gatewayClient:     gatewayClient,
		tokenService:      tokenService,
		serverURL:         serverURL,
		internalServerURL: fmt.Sprintf("http://localhost:%d", serverPort),
		gptClient:         gptClient,
	}
}

type Response struct {
	Run     *v1.Run
	Thread  *v1.Thread
	Message string

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
		panic("runState doesn't match")
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

	var (
		notFoundCount int
		notFoundErr   error
		notFoundLimit = 3
	)
	for notFoundCount < notFoundLimit {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			r, err := c.RunState(ctx, run.Namespace, run.Name)
			if apierror.IsNotFound(err) {
				notFoundErr = err
				notFoundCount++
				continue
			} else if err != nil {
				return nil, err
			}
			if stop, err := done(r); err != nil {
				return nil, err
			} else if stop {
				return r, nil
			}
		}
	}

	return nil, fmt.Errorf("run state not found after %d attempts: %w", notFoundLimit, notFoundErr)
}

type Options struct {
	Synchronous          bool
	EphemeralThread      bool
	Thread               *v1.Thread
	ThreadName           string
	PreviousRunName      string
	ForceNoResume        bool
	CreateThread         bool
	CredentialContextIDs []string
	UserUID              string
	GenerateName         string
	ExtraEnv             []string
}

func (i *Invoker) getChatState(ctx context.Context, c kclient.Client, run *v1.Run) (result string, _ error) {
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

func unAbortThread(ctx context.Context, c kclient.Client, thread *v1.Thread) error {
	if thread.Spec.Abort {
		thread.Spec.Abort = false
		return c.Update(ctx, thread)
	}
	return nil
}

type runOptions struct {
	Env                  []string
	CredentialContextIDs []string
	Timeout              time.Duration
}

func isEphemeral(run *v1.Run) bool {
	return strings.HasPrefix(run.Name, ephemeralRunPrefix)
}

func (i *Invoker) createRun(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, tool any, input string, opts runOptions) (*Response, error) {
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
			ThreadName:           thread.Name,
			PreviousRunName:      thread.Status.LastRunName,
			Input:                input,
			Tool:                 string(toolData),
			Env:                  opts.Env,
			CredentialContextIDs: opts.CredentialContextIDs,
			Timeout:              metav1.Duration{Duration: opts.Timeout},
		},
	}

	if err := c.Create(ctx, &run); err != nil {
		return nil, err
	}
	log.Infof("Created run resource: run=%s thread=%s previousRun=%s", run.Name, thread.Name, thread.Status.LastRunName)

	resp := &Response{
		Run:    &run,
		Thread: thread,
	}

	ctx, cancel := context.WithCancel(ctx)
	resp.uncached = i.uncached
	resp.gatewayClient = i.gatewayClient
	resp.cancel = cancel
	go func() {
		if err := i.Resume(ctx, c, thread, &run); err != nil {
			log.Errorf("run failed: run=%s thread=%s error=%s", run.Name, thread.Name, err)
		}
	}()

	return resp, nil
}

func (i *Invoker) Resume(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, run *v1.Run) (err error) {
	input := run.Spec.Input

	chatState, err := i.getChatState(ctx, c, run)
	if err != nil {
		return fmt.Errorf("failed to get chat state: %w", err)
	}

	now := time.Now()
	token, err := i.tokenService.NewToken(ctx, persistent.TokenContext{
		Audience:  i.serverURL,
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour * 24),
		Namespace: run.Namespace,
		Scope:     thread.Namespace,
		TokenType: persistent.TokenTypeRun,
	})
	if err != nil {
		return err
	}

	options := gptscript.Options{
		GlobalOptions: gptscript.GlobalOptions{
			Env: append(run.Spec.Env,
				"OBOT_SERVER_PUBLIC_URL="+i.serverURL,
				"OBOT_SERVER_URL="+i.internalServerURL,
				"OBOT_TOKEN="+token,
				"OBOT_RUN_ID="+run.Name,
				"OBOT_THREAD_ID="+thread.Name,
				"GPTSCRIPT_HTTP_ENV=OBOT_TOKEN,OBOT_RUN_ID,OBOT_THREAD_ID,OBOT_PROJECT_ID,OBOT_WORKFLOW_ID,OBOT_WORKFLOW_STEP_ID,OBOT_AGENT_ID",
			),
		},
		Input:              input,
		CredentialContexts: run.Spec.CredentialContextIDs,
		ChatState:          chatState,
		ForceSequential:    true,
	}
	log.Infof("Executing run with resolved invocation settings: run=%s thread=%s", run.Name, thread.Name)

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
	for range 3 {
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

func (i *Invoker) doSaveState(ctx context.Context, c kclient.Client, _ *v1.Thread, run *v1.Run, runResp *gptscript.Run, retErr error) error {
	var (
		runStateSpec gtypes.RunState
		runChanged   bool
		err          error
		prevState    = run.Status.State
	)

	runStateSpec.Name = run.Name
	runStateSpec.Namespace = run.Namespace
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

	runState, err := i.gatewayClient.RunState(ctx, run.Namespace, run.Name)
	if apierror.IsNotFound(err) {
		runState = &gtypes.RunState{
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
	case v1.Continue, v1.Finished:
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
			runChanged = true
			run.Status.Output = shortText
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
		if run.Status.EndTime.IsZero() && final {
			run.Status.EndTime = metav1.Now()
		}
		if err := c.Status().Update(ctx, run); err != nil {
			return err
		}
		log.Infof(
			"Persisted run status update: run=%s thread=%s previousState=%v newState=%v final=%v hasError=%v",
			run.Name,
			run.Spec.ThreadName,
			prevState,
			run.Status.State,
			final,
			run.Status.Error != "",
		)
	}

	return nil
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

	wg.Go(func() {
		for {
			select {
			case <-saveCtx.Done():
				return
			case <-time.After(time.Second):
				_ = i.saveState(ctx, c, thread, run, runResp, nil)
			}
		}
	})

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
		go i.watchThreadAbort(runCtx, c, thread, cancelRun)
	}

	for {
		select {
		case <-runCtx.Done():
			return context.Cause(runCtx)
		case _, ok := <-runEvent:
			if !ok {
				if errOut := runResp.ErrorOutput(); errOut != "" {
					return errors.New(errOut)
				}
				return runResp.Err()
			}
		}
	}
}

func (i *Invoker) watchThreadAbort(ctx context.Context, c kclient.WithWatch, thread *v1.Thread, cancelRun context.CancelCauseFunc) {
	_, _ = wait.For(ctx, c, thread, func(thread *v1.Thread) (bool, error) {
		if thread.Spec.Abort {
			// Abort aggressively so that:
			// 1. If this is a task, the next step doesn't run
			// 2. If this a chat thread, unconfirmed tool calls don't block abort and are removed from the chat history
			cancelRun(fmt.Errorf("thread was aborted, cancelling run"))
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
