package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/openai"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/typed"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gz"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/wait"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Emitter struct {
	client        kclient.WithWatch
	gatewayClient *gclient.Client
	liveStates    map[string][]liveState
	liveStateLock sync.RWMutex
	liveBroadcast *sync.Cond
}

func NewEmitter(client kclient.WithWatch, gatewayClient *gclient.Client) *Emitter {
	e := &Emitter{
		client:        client,
		gatewayClient: gatewayClient,
		liveStates:    map[string][]liveState{},
	}
	e.liveBroadcast = sync.NewCond(&e.liveStateLock)
	return e
}

type liveState struct {
	Prg      *gptscript.Program
	Frames   *gptscript.CallFrames
	Progress *types.Progress
	Done     bool
}

type WatchOptions struct {
	History                  bool
	LastRunName              string
	MaxRuns                  int
	After                    bool
	ThreadName               string
	ThreadResourceVersion    string
	Follow                   bool
	FollowWorkflowExecutions bool
	Run                      *v1.Run
	WaitForThread            bool
}

type callFramePrintState struct {
	Outputs                []gptscript.Output
	InputPrinted           bool
	InputTranslatedPrinted bool
}

type printState struct {
	frames          map[string]callFramePrintState
	toolCalls       map[string]string
	lastStepPrinted string
}

func newPrintState(oldState *printState) *printState {
	if oldState != nil && oldState.toolCalls != nil {
		// carry over tool call state
		return &printState{
			frames:    map[string]callFramePrintState{},
			toolCalls: oldState.toolCalls,
		}
	}
	return &printState{
		frames:    map[string]callFramePrintState{},
		toolCalls: map[string]string{},
	}
}

func (e *Emitter) Submit(run *v1.Run, prg *gptscript.Program, frames gptscript.CallFrames) {
	e.liveStateLock.Lock()
	defer e.liveStateLock.Unlock()

	e.liveStates[run.Name] = append(e.liveStates[run.Name], liveState{Prg: prg, Frames: &frames})
	for i, state := range e.liveStates[run.Name] {
		// This is to save memory until we remove this liveState hack
		if state.Frames != nil {
			e.liveStates[run.Name][i].Frames = &frames
		}
	}
	e.liveBroadcast.Broadcast()
}

func (e *Emitter) Done(run *v1.Run) {
	e.liveStateLock.Lock()
	defer e.liveStateLock.Unlock()

	e.liveStates[run.Name] = append(e.liveStates[run.Name], liveState{Done: true})
	e.liveBroadcast.Broadcast()
}

func (e *Emitter) ClearProgress(run *v1.Run) {
	e.liveStateLock.Lock()
	defer e.liveStateLock.Unlock()

	delete(e.liveStates, run.Name)
	e.liveBroadcast.Broadcast()
}

func (e *Emitter) SubmitProgress(run *v1.Run, progress types.Progress) {
	e.liveStateLock.Lock()
	defer e.liveStateLock.Unlock()

	e.liveStates[run.Name] = append(e.liveStates[run.Name], liveState{Progress: &progress})
	e.liveBroadcast.Broadcast()
}

func (e *Emitter) findRunByThreadName(ctx context.Context, threadNamespace, threadName, resourceVersion string) (*v1.Run, error) {
	var run v1.Run

	w, err := e.client.Watch(ctx, &v1.ThreadList{}, kclient.InNamespace(threadNamespace),
		kclient.MatchingFields{"metadata.name": threadName}, &kclient.ListOptions{
			Raw: &metav1.ListOptions{
				ResourceVersion: resourceVersion,
			},
		})
	if err != nil {
		return nil, err
	}
	defer func() {
		w.Stop()
		//nolint:revive
		for range w.ResultChan() {
		}
	}()

	for event := range w.ResultChan() {
		if thread, ok := event.Object.(*v1.Thread); ok {
			if thread.Status.CurrentRunName != "" {
				if err := e.client.Get(ctx, router.Key(thread.Namespace, thread.Status.CurrentRunName), &run); err != nil && !apierrors.IsNotFound(err) {
					return nil, err
				} else if err == nil {
					return &run, nil
				}
			}
			if thread.Status.LastRunName != "" {
				if err := e.client.Get(ctx, router.Key(thread.Namespace, thread.Status.LastRunName), &run); err != nil && !apierrors.IsNotFound(err) {
					return nil, err
				} else if err == nil {
					return &run, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no run found for thread: %s", threadName)
}

func (e *Emitter) getThread(ctx context.Context, namespace, name string, wait bool) (*v1.Thread, error) {
	var thread v1.Thread
	if err := e.client.Get(ctx, router.Key(namespace, name), &thread); apierrors.IsNotFound(err) && wait {
		w, err := e.client.Watch(ctx, &v1.ThreadList{}, kclient.MatchingFields{"metadata.name": name}, kclient.InNamespace(namespace))
		if err != nil {
			return nil, err
		}
		defer func() {
			w.Stop()
			//nolint:revive
			for range w.ResultChan() {
			}
		}()
		for event := range w.ResultChan() {
			if thread, ok := event.Object.(*v1.Thread); ok {
				return thread, nil
			}
		}
		return nil, fmt.Errorf("failed to find thread %s", name)
	} else if err != nil {
		return nil, err
	}
	return &thread, nil
}

func (e *Emitter) Watch(ctx context.Context, namespace string, opts WatchOptions) (*v1.Run, chan types.Progress, error) {
	var (
		run v1.Run
	)

	if opts.Run != nil {
		run = *opts.Run
	} else if opts.LastRunName != "" {
		if err := e.client.Get(ctx, router.Key(namespace, opts.LastRunName), &run); err != nil {
			return nil, nil, err
		}
		if opts.ThreadName != "" && run.Spec.ThreadName != opts.ThreadName {
			return nil, nil, fmt.Errorf("run %s is not associated with thread %s", opts.LastRunName, opts.ThreadName)
		}
	} else if opts.ThreadName != "" {
		thread, err := e.getThread(ctx, namespace, opts.ThreadName, opts.WaitForThread)
		if err != nil {
			return nil, nil, err
		}
		if thread.Status.LastRunName == "" {
			runForThread, err := e.findRunByThreadName(ctx, namespace, opts.ThreadName, opts.ThreadResourceVersion)
			if err != nil {
				return nil, nil, err
			}
			run = *runForThread
		} else if err := e.client.Get(ctx, router.Key(namespace, thread.Status.LastRunName), &run); err != nil {
			return nil, nil, err
		}
	}

	result := make(chan types.Progress)

	if run.Name == "" {
		close(result)
		return &run, result, nil
	}

	go func() {
		// error is ignored because it's internally sent to progress channel
		_ = e.streamEvents(ctx, run, opts, result)
	}()

	return &run, result, nil
}

func (e *Emitter) printRun(ctx context.Context, state *printState, run v1.Run, result chan types.Progress, historical bool) error {
	var (
		liveIndex    int
		broadcast    = make(chan struct{}, 1)
		done, cancel = context.WithCancel(ctx)
	)
	defer cancel()

	defer func() {
		result <- types.Progress{
			RunID:       run.Name,
			Time:        types.NewTime(time.Now()),
			RunComplete: true,
		}
	}()

	if run.Spec.WorkflowStepID != "" && run.Spec.WorkflowExecutionName != "" && state.lastStepPrinted != run.Spec.WorkflowStepID {
		var wfe v1.WorkflowExecution
		if err := e.client.Get(ctx, router.Key(run.Namespace, run.Spec.WorkflowExecutionName), &wfe); err != nil {
			return err
		}
		step, _ := types.FindStep(wfe.Status.WorkflowManifest, run.Spec.WorkflowStepID)
		if run.Spec.WorkflowStepID != "" && step == nil {
			step = &types.Step{
				ID: run.Spec.WorkflowStepID,
			}
		}
		result <- types.Progress{
			RunID:       run.Name,
			ParentRunID: run.Spec.PreviousRunName,
			Time:        types.NewTime(wfe.CreationTimestamp.Time),
			Step:        step,
		}
		state.lastStepPrinted = run.Spec.WorkflowStepID
	}

	go func() {
		e.liveStateLock.Lock()
		defer e.liveStateLock.Unlock()
		for {
			select {
			case broadcast <- struct{}{}:
			default:
			}

			select {
			case <-done.Done():
				return
			default:
			}

			e.liveBroadcast.Wait()
		}
	}()

	var (
		tick   = immediately()
		ticker *time.Ticker
	)

	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-broadcast:
			var notSeen []liveState
			e.liveStateLock.RLock()
			liveStateLen := len(e.liveStates[run.Name])
			if liveIndex < liveStateLen {
				notSeen = e.liveStates[run.Name][liveIndex:]
				liveIndex = liveStateLen
			}
			e.liveStateLock.RUnlock()
			if liveStateLen < liveIndex {
				_, err := e.gatewayClient.RunState(ctx, run.Namespace, run.Name)
				if apierrors.IsNotFound(err) {
					return nil
				} else if err != nil {
					return err
				}
				liveIndex = liveStateLen
				continue
			}
			for _, toPrint := range notSeen {
				if toPrint.Done {
					runState, err := e.gatewayClient.RunState(ctx, run.Namespace, run.Name)
					if err == nil && runState.Done {
						return nil
					} else if apierrors.IsNotFound(err) {
						// ephemeral tasks this won't exist
						return nil
					} else if err != nil {
						return err
					}
					continue
				}

				if toPrint.Progress != nil {
					result <- *toPrint.Progress
				} else {
					if err := e.callToEvents(run, toPrint.Prg, *toPrint.Frames, state, result); err != nil {
						return err
					}
				}
			}
		case <-tick:
			if ticker == nil {
				// now wait every second for new events
				ticker = time.NewTicker(time.Second)
				tick = ticker.C
			}
			runState, err := e.gatewayClient.RunState(ctx, run.Namespace, run.Name)
			// There was a previous bug that made it possible to have run states leftover from previous runs.
			// If this run happens to coincide with an old run state, then wait for the run state to be updated.
			// We know this is an old run state if the updated time is not zero and the updated time is before the run was created.
			if apierrors.IsNotFound(err) || (runState != nil && !runState.UpdatedAt.IsZero() && runState.UpdatedAt.Before(run.CreationTimestamp.Time)) {
				var checkRun v1.Run
				if err := e.client.Get(ctx, router.Key(run.Namespace, run.Name), &checkRun); err == nil {
					if checkRun.Status.Error != "" {
						result <- types.Progress{
							RunID: run.Name,
							Time:  types.NewTime(time.Now()),
							Error: checkRun.Status.Error,
						}
						return nil
					}
				}
				continue
			} else if err != nil {
				return err
			}
			var (
				prg        gptscript.Program
				callFrames = gptscript.CallFrames{}
			)
			if len(runState.Program) != 0 {
				if err := gz.Decompress(&prg, runState.Program); err != nil {
					return err
				}
			}
			if len(runState.CallFrame) != 0 {
				if err := gz.Decompress(&callFrames, runState.CallFrame); err != nil {
					return err
				}
			}

			// Don't log historical runs that have errored
			if runState.Done && runState.Error != "" && historical {
				return nil
			}

			if err := e.callToEvents(run, &prg, callFrames, state, result); err != nil {
				return err
			}

			if runState.Done {
				if runState.Error != "" {
					result <- types.Progress{
						RunID: run.Name,
						Time:  types.NewTime(time.Now()),
						Error: runState.Error,
					}
				}
				return nil
			}
		}
	}
}

func immediately() <-chan time.Time {
	ch := make(chan time.Time)
	close(ch)
	return ch
}

func (e *Emitter) printParent(ctx context.Context, remaining int, state *printState, run v1.Run, result chan types.Progress) error {
	if remaining <= 0 {
		return nil
	}

	if run.Spec.PreviousRunName == "" {
		return nil
	}

	var (
		parent      v1.Run
		errNotFound error
	)
	if err := e.client.Get(ctx, kclient.ObjectKey{Namespace: run.Namespace, Name: run.Spec.PreviousRunName}, &parent); err != nil {
		return err
	}

	if parent.Spec.ThreadName != "" && run.Spec.ThreadName != "" && parent.Spec.ThreadName != run.Spec.ThreadName {
		return nil
	}
	if err := e.printParent(ctx, remaining-1, state, parent, result); apierrors.IsNotFound(err) {
		errNotFound = err
	} else if err != nil {
		return err
	}

	return errors.Join(errNotFound, e.printRun(ctx, state, parent, result, true))
}

func (e *Emitter) streamEvents(ctx context.Context, run v1.Run, opts WatchOptions, result chan types.Progress) (retErr error) {
	defer close(result)
	defer func() {
		if retErr != nil {
			result <- types.Progress{
				Time:  types.NewTime(time.Now()),
				Error: retErr.Error(),
			}
		}
	}()

	if opts.After {
		opts.History = false
	}

	var (
		state              *printState
		replayCompleteSent bool
	)
	for {
		state = newPrintState(state)

		if opts.History {
			if err := e.printParent(ctx, opts.MaxRuns-1, state, run, result); !apierrors.IsNotFound(err) && err != nil {
				return err
			}
			if run.Status.EndTime.IsZero() || run.Status.State == v1.Waiting {
				replayCompleteSent = true
				result <- types.Progress{
					ReplayComplete: true,
					ThreadID:       run.Spec.ThreadName,
				}
			}
		} else if !replayCompleteSent {
			replayCompleteSent = true
			result <- types.Progress{
				ReplayComplete: true,
				ThreadID:       run.Spec.ThreadName,
			}
		}

		if opts.After {
			opts.After = false
		} else {
			if err := e.printRun(ctx, state, run, result, false); err != nil {
				return err
			}
		}

		if opts.History && !run.Status.EndTime.IsZero() {
			result <- types.Progress{
				ReplayComplete: true,
				ThreadID:       run.Spec.ThreadName,
			}
		}

		nextRun, err := e.findNextRun(ctx, run, opts)
		if err != nil {
			return err
		}
		if nextRun == nil {
			return nil
		}

		// don't tail history again
		opts.History = false
		run = *nextRun
	}
}

func (e *Emitter) getNextWorkflowRun(ctx context.Context, run v1.Run) (*v1.Run, error) {
	var runName string
	_, err := wait.For(ctx, e.client, &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: run.Namespace,
			Name:      run.Spec.ThreadName,
		},
	}, func(thread *v1.Thread) (bool, error) {
		if thread.Status.CurrentRunName != "" && thread.Status.CurrentRunName != run.Name {
			runName = thread.Status.CurrentRunName
			return true, nil
		}
		if thread.Status.LastRunName != "" && thread.Status.LastRunName != run.Name {
			runName = thread.Status.LastRunName
			return true, nil
		}
		return false, nil
	}, wait.Option{
		Timeout: 15 * time.Minute,
	})
	if err != nil {
		return nil, err
	}

	var nextRun v1.Run
	if err := e.client.Get(ctx, router.Key(run.Namespace, runName), &nextRun); err != nil {
		return nil, err
	}
	return &nextRun, nil
}

func (e *Emitter) isWorkflowDone(ctx context.Context, run v1.Run, opts WatchOptions) (<-chan *v1.Run, func(), error) {
	if run.Spec.WorkflowExecutionName == "" {
		return nil, func() {}, nil
	}
	w, err := e.client.Watch(ctx, &v1.WorkflowExecutionList{}, kclient.InNamespace(run.Namespace), &kclient.MatchingFields{
		"metadata.name": run.Spec.WorkflowExecutionName,
	})
	if err != nil {
		return nil, nil, err
	}

	result := make(chan *v1.Run, 1)
	cancel := func() {
		w.Stop()
		go func() {
			//nolint:revive
			for range w.ResultChan() {
			}
		}()
	}

	go func() {
		defer close(result)
		defer cancel()
		for event := range w.ResultChan() {
			if wfe, ok := event.Object.(*v1.WorkflowExecution); ok {
				if wfe.Status.State.IsTerminal() || wfe.Status.State.IsBlocked() {
					if opts.FollowWorkflowExecutions {
						next, err := e.getNextWorkflowRun(ctx, run)
						if err != nil {
							if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
								log.Errorf("failed to get next workflow run for last run %q: %v", run.Name, err)
							}
						} else {
							result <- next
						}
					}
					return
				}
			}
		}
	}()

	return result, cancel, nil
}

func (e *Emitter) findNextRun(ctx context.Context, run v1.Run, opts WatchOptions) (*v1.Run, error) {
	var (
		runs     v1.RunList
		criteria = []kclient.ListOption{
			kclient.InNamespace(run.Namespace),
			kclient.MatchingFields{"spec.previousRunName": run.Name},
		}
	)

	if !opts.Follow {
		return nil, nil
	}

	if err := e.client.List(ctx, &runs, criteria...); err != nil {
		return nil, err
	}
	if len(runs.Items) > 0 {
		return &runs.Items[0], nil
	}
	w, err := e.client.Watch(ctx, &v1.RunList{}, append(criteria, &kclient.ListOptions{
		Raw: &metav1.ListOptions{
			ResourceVersion: runs.ResourceVersion,
			TimeoutSeconds:  typed.Pointer(int64(15 * 60)),
		},
	})...)
	if err != nil {
		return nil, err
	}
	defer func() {
		w.Stop()
		//nolint:revive
		for range w.ResultChan() {
		}
	}()

	isWorkflowDone, cancel, err := e.isWorkflowDone(ctx, run, opts)
	if err != nil {
		return nil, err
	}
	defer cancel()

	for {
		select {
		case event, ok := <-w.ResultChan():
			if !ok {
				return nil, nil
			}
			if run, ok := event.Object.(*v1.Run); ok {
				return run, nil
			}
		case run := <-isWorkflowDone:
			return run, nil
		}
	}
}

func (e *Emitter) callToEvents(run v1.Run, prg *gptscript.Program, frames gptscript.CallFrames, printed *printState, out chan types.Progress) error {
	parent := frames.ParentCallFrame()
	if parent.ID == "" || parent.Start.IsZero() {
		return nil
	}

	return e.printCall(run, prg, &parent, frames, printed, out)
}

func getStepTemplateInvoke(prg *gptscript.Program, call *gptscript.CallFrame, frames gptscript.CallFrames) *types.StepTemplateInvoke {
	if len(call.Tool.InputFilters) == 0 {
		return nil
	}

	toolIDs := call.Tool.ToolMapping[call.Tool.InputFilters[0]]
	if len(toolIDs) == 0 {
		return nil
	}

	tool := prg.ToolSet[toolIDs[0].ToolID]

	for _, frame := range frames {
		if frame.Tool.ID == toolIDs[0].ToolID && frame.ParentID == call.ID && frame.ToolCategory == gptscript.InputToolCategory {
			for _, output := range frame.Output {
				if output.Content != "" {
					args := map[string]string{}
					_ = json.Unmarshal([]byte(frame.Input), &args)
					return &types.StepTemplateInvoke{
						Name:        tool.Name,
						Description: tool.Description,
						Args:        args,
						Result:      output.Content,
					}
				}
			}
		}
	}

	return nil
}

func (e *Emitter) printCall(run v1.Run, prg *gptscript.Program, call *gptscript.CallFrame, frames gptscript.CallFrames, lastPrint *printState, out chan types.Progress) error {
	printed := lastPrint.frames[call.ID]
	lastOutputs := printed.Outputs

	if run.Spec.Input != "" && !printed.InputPrinted {
		out <- types.Progress{
			RunID:                    run.Name,
			Time:                     types.NewTime(call.Start),
			Content:                  "\n",
			Input:                    run.Spec.Input,
			InputIsStepTemplateInput: len(call.Tool.InputFilters) > 0,
		}
		printed.InputPrinted = true
	}

	if !printed.InputTranslatedPrinted {
		if translated := getStepTemplateInvoke(prg, call, frames); translated != nil {
			out <- types.Progress{
				RunID:              run.Name,
				Time:               types.NewTime(call.Start),
				StepTemplateInvoke: translated,
			}
			printed.InputTranslatedPrinted = true
		}
	}

	llmRequest, _ := call.LLMRequest.(map[string]any)
	toolMapping, _ := llmRequest["toolMapping"].(map[string]any)

	for i, currentOutput := range call.Output {
		for i >= len(lastOutputs) {
			lastOutputs = append(lastOutputs, gptscript.Output{})
		}
		last := lastOutputs[i]

		if last.Content != currentOutput.Content {
			currentOutput.Content = printString(prg, call.Start, run.Name, toolMapping, i, out, last.Content, currentOutput.Content)
		}

		for _, callID := range slices.Sorted(maps.Keys(currentOutput.SubCalls)) {
			subCall := currentOutput.SubCalls[callID]
			output := getToolCallOutput(frames, callID)
			taskID, taskRunID := getTaskRunID(frames, callID)
			if _, ok := last.SubCalls[callID]; !ok || (lastPrint.toolCalls[callID] != output && output != "") {
				if lastOutput, seenTool := lastPrint.toolCalls[callID]; !seenTool || lastOutput != output {
					if tool, ok := prg.ToolSet[subCall.ToolID]; ok {
						tc := &types.ToolCall{
							Name:        tool.Name,
							Description: tool.Description,
							TaskID:      taskID,
							TaskRunID:   taskRunID,
							Input:       subCall.Input,
							Output:      output,
							Metadata:    tool.MetaData,
						}
						out <- types.Progress{
							RunID:     run.Name,
							ContentID: callID,
							Time:      types.NewTime(call.Start),
							ToolCall:  tc,
						}
					}
					lastPrint.toolCalls[callID] = output
				}
			}
		}

		lastOutputs[i] = currentOutput
	}

	printed.Outputs = lastOutputs
	lastPrint.frames[call.ID] = printed

	return nil
}

func getTaskRunID(frames gptscript.CallFrames, callID string) (string, string) {
	frame := frames[callID]
	var (
		resume v1.ExternalCallResume
		call   v1.ExternalCall
	)

	if err := json.Unmarshal([]byte(frame.Input), &resume); err == nil && resume.Type == "obotExternalCallResume" {
		call = resume.Call
	} else if len(frame.Output) == 1 {
		if err := json.Unmarshal([]byte(frame.Output[0].Content), &call); err != nil && call.Type != "obotExternalCall" {
			return "", ""
		}
	}

	callData := struct {
		TaskID string `json:"taskID"`
	}{}
	if err := json.Unmarshal([]byte(call.Data), &callData); err == nil && callData.TaskID != "" {
		return callData.TaskID, call.ID
	}

	return "", ""
}

func getToolCallOutput(frames gptscript.CallFrames, callID string) string {
	frame := frames[callID]
	out := frame.Output
	if len(out) == 1 && (frame.Type == gptscript.EventTypeCallFinish || frame.Type == gptscript.EventTypeChat) {
		var call v1.ExternalCall
		if err := json.Unmarshal([]byte(out[0].Content), &call); err == nil && call.Type == "obotExternalCall" {
			return ""
		}
		return strings.TrimPrefix(out[0].Content, "CHAT FINISH: ")
	}
	return ""
}

func printString(prg *gptscript.Program, time time.Time, runID string, toolMapping map[string]any, outputIndex int, out chan types.Progress, last, current string) string {
	if hasRolledBack(last, current) {
		return last
	}

	lastParts := strings.Split(last, "<tool call> ")
	currentParts := strings.Split(current, "<tool call> ")

	for i, part := range currentParts {
		var (
			lastPart    string
			currentPart = part
		)
		if len(lastParts) > i {
			lastPart = lastParts[i]
		}
		if i > 0 {
			lastPart = "<tool call> " + lastPart
			currentPart = "<tool call> " + currentPart
		}
		if currentPart == "" {
			continue
		}
		printSubString(prg, time, runID, toolMapping, outputIndex, i, out, lastPart, currentPart)
	}
	return current
}

func hasRolledBack(last string, current string) bool {
	// Because we get events from two sources (live in memory, and persisted RunStates), it's possible we can
	// get an old message. Typically, this is fine because we would have already printed all the old messages and we
	// save that state. The issue is when we are still streaming the response of a message, and we get and old content of
	// content.

	// First detect if we are getting the "Waiting for model" message after we have already printed some content
	if len(last) > 0 && current == openai.WaitingMessage {
		return true
	}

	// Then check if we are current an old substring we have already printed.
	if len(last) > len(current) && strings.HasPrefix(last, current) {
		return true
	}
	return false
}

func printSubString(prg *gptscript.Program, time time.Time, runID string, toolMapping map[string]any, outputIndex, contentSuffixIndex int, out chan types.Progress, last, current string) string {
	toPrint := current
	if strings.HasPrefix(current, last) {
		toPrint = current[len(last):]
	} else if hasRolledBack(last, current) {
		return last
	}

	var (
		toolName  string
		toolInput *types.ToolInput
	)

	toPrint, waitingOnModel := strings.CutPrefix(toPrint, openai.WaitingMessage)
	toPrint, toolPrint, isToolCall := strings.Cut(toPrint, "<tool call> ")

	if isToolCall {
		toolName = strings.Split(toolPrint, " ->")[0]
	} else {
		_, wasToolPrint, wasToolCall := strings.Cut(current, "<tool call> ")
		if wasToolCall {
			isToolCall = true
			toolName = strings.Split(wasToolPrint, " ->")[0]
			toolPrint = toPrint
			toPrint = ""
		}
	}

	toolPrint = strings.TrimPrefix(toolPrint, toolName+" -> ")

	if isToolCall {
		if v, ok := toolMapping[toolName]; ok {
			toolName = fmt.Sprint(v)
		}
		tool := prg.ToolSet[toolName]
		toolInput = &types.ToolInput{
			Name:        tool.Name,
			Description: tool.Description,
			Input:       toolPrint,
			Metadata:    tool.MetaData,
		}
	}

	out <- types.Progress{
		RunID:          runID,
		Time:           types.NewTime(time),
		Content:        toPrint,
		ContentID:      fmt.Sprintf("%s-%d-%d", runID, outputIndex, contentSuffixIndex),
		ToolInput:      toolInput,
		WaitingOnModel: waitingOnModel,
	}

	return current
}
