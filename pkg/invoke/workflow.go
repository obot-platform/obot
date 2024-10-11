package invoke

import (
	"context"
	"fmt"
	"strings"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/events"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type WorkflowOptions struct {
	ThreadName string
	StepID     string
	Events     bool
}

func (i *Invoker) startWorkflow(ctx context.Context, c kclient.WithWatch, wf *v1.Workflow, input string) (*v1.Thread, error) {
	wfe := &v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowExecutionPrefix,
			Namespace:    wf.Namespace,
		},
		Spec: v1.WorkflowExecutionSpec{
			Input:        input,
			WorkflowName: wf.Name,
		},
	}

	if err := c.Create(ctx, wfe); err != nil {
		return nil, err
	}

	w, err := c.Watch(ctx, &v1.WorkflowExecutionList{}, kclient.InNamespace(wfe.Namespace), kclient.MatchingFields{"metadata.name": wfe.Name})
	if err != nil {
		return nil, err
	}

	defer func() {
		w.Stop()
		for range w.ResultChan() {
		}
	}()

	for event := range w.ResultChan() {
		wfe, ok := event.Object.(*v1.WorkflowExecution)
		if !ok {
			continue
		}

		if wfe.Status.State == types.WorkflowStateError {
			return nil, fmt.Errorf("workflow failed: %s", wfe.Status.Error)
		}

		if wfe.Status.ThreadName != "" {
			var thread v1.Thread
			return &thread, c.Get(ctx, router.Key(wfe.Namespace, wfe.Status.ThreadName), &thread)
		}
	}

	return nil, fmt.Errorf("workflow did not start")
}

func (i *Invoker) Workflow(ctx context.Context, c kclient.WithWatch, wf *v1.Workflow, input string, opt WorkflowOptions) (*Response, error) {
	var (
		thread *v1.Thread
		err    error
	)
	if opt.ThreadName != "" {
		thread, err = i.rerunThread(ctx, c, wf, opt.ThreadName, opt.StepID)
		if err != nil {
			return nil, err
		}
	} else {
		thread, err = i.startWorkflow(ctx, c, wf, input)
		if err != nil {
			return nil, err
		}
	}

	if !opt.Events {
		closedChan := make(chan types.Progress)
		close(closedChan)
		return &Response{
			Thread: thread,
			Events: closedChan,
		}, nil
	}

	run, prg, err := i.events.Watch(ctx, thread.Namespace, events.WatchOptions{
		History:               true,
		ThreadName:            thread.Name,
		ThreadResourceVersion: thread.ResourceVersion,
		Follow:                true,
	})
	if err != nil {
		return nil, err
	}

	return &Response{
		Thread: thread,
		Run:    run,
		Events: prg,
	}, nil
}

func (i *Invoker) rerunThread(ctx context.Context, c kclient.WithWatch, wf *v1.Workflow, threadName, stepID string) (*v1.Thread, error) {
	var (
		thread v1.Thread
		wfe    v1.WorkflowExecution
	)

	if err := c.Get(ctx, router.Key(wf.Namespace, threadName), &thread); err != nil {
		return nil, err
	}

	if thread.Spec.WorkflowName != wf.Name {
		return nil, fmt.Errorf("thread does not belong to workflow: %s", wf.Name)
	}

	if thread.Spec.WorkflowExecutionName == "" {
		return nil, fmt.Errorf("thread does not have a workflow execution")
	}

	if err := c.Get(ctx, router.Key(wf.Namespace, thread.Spec.WorkflowExecutionName), &wfe); err != nil {
		return nil, err
	}

	if stepID != "" {
		step, _ := types.FindStep(&wf.Spec.Manifest, stepID)
		if step == nil {
			return nil, fmt.Errorf("step not found: %s", stepID)
		}

		if err := i.deleteSteps(ctx, c, thread, stepID); err != nil {
			return nil, err
		}
	}

	if thread.Status.CurrentRunName != "" || thread.Status.LastRunName != "" {
		thread.Status.CurrentRunName = ""
		thread.Status.LastRunName = ""
		if err := c.Status().Update(ctx, &thread); err != nil {
			return nil, err
		}
	}

	wfe.Spec.WorkflowGeneration++
	wfe.Spec.RunUntilStep = stepID
	return &thread, c.Update(ctx, &wfe)
}

func (i *Invoker) deleteSteps(ctx context.Context, c kclient.Client, thread v1.Thread, stepID string) error {
	var (
		steps v1.WorkflowStepList
	)

	if err := c.List(ctx, &steps, kclient.InNamespace(thread.Namespace)); err != nil {
		return err
	}

	if len(steps.Items) == 0 {
		return types.NewErrNotFound("step not found: %s", stepID)
	}

	var deleted bool
	for _, step := range steps.Items {
		if step.Status.State == types.WorkflowStateError ||
			step.Spec.WorkflowExecutionName == thread.Spec.WorkflowExecutionName && stepMatches(step.Spec.Step.ID, stepID) {
			if err := c.Delete(ctx, &step); kclient.IgnoreNotFound(err) != nil {
				return err
			}
			deleted = true
		}
	}

	if !deleted {
		return types.NewErrNotFound("step not found: %s", stepID)
	}

	return nil
}

func stepMatches(left, right string) bool {
	return stepLookupID(left) == stepLookupID(right)
}

func stepLookupID(stepID string) string {
	id, _, _ := strings.Cut(stepID, "{")
	return id
}
