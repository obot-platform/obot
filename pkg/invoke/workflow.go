package invoke

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/handlers/retention"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type WorkflowOptions struct {
	StepID                string
	WorkflowExecutionName string
}

func (i *Invoker) startWorkflow(ctx context.Context, c kclient.WithWatch, wf *v1.Workflow, input string, opt WorkflowOptions) (*v1.WorkflowExecution, *v1.Thread, error) {
	wfe := &v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowExecutionPrefix,
			Name:         opt.WorkflowExecutionName,
			Namespace:    wf.Namespace,
		},
		Spec: v1.WorkflowExecutionSpec{
			Input:        input,
			ThreadName:   wf.Spec.ThreadName,
			WorkflowName: wf.Name,
		},
	}

	if err := c.Create(ctx, wfe); err != nil {
		return nil, nil, err
	}

	wfe, err := wait.For(ctx, c, wfe, func(wfe *v1.WorkflowExecution) (bool, error) {
		if wfe.Status.State == types.WorkflowStateError {
			return false, fmt.Errorf("workflow failed: %s", wfe.Status.Error)
		}
		return wfe.Status.ThreadName != "", nil
	})
	if err != nil {
		return nil, nil, err
	}

	var thread v1.Thread
	return wfe, &thread, c.Get(ctx, router.Key(wfe.Namespace, wfe.Status.ThreadName), &thread)
}

func (i *Invoker) Workflow(ctx context.Context, c kclient.WithWatch, wf *v1.Workflow, input string, opt WorkflowOptions) (*Response, error) {
	var (
		thread *v1.Thread
		err    error
		rerun  bool
		wfe    = &v1.WorkflowExecution{}
	)

	if opt.WorkflowExecutionName != "" {
		if err := c.Get(ctx, router.Key(wf.Namespace, opt.WorkflowExecutionName), wfe); err != nil && !apierror.IsNotFound(err) {
			return nil, err
		} else if err == nil {
			wfe, err = wait.For(ctx, c, wfe, func(wfe *v1.WorkflowExecution) (bool, error) {
				return wfe.Status.ThreadName != "", nil
			})
			if err != nil {
				return nil, err
			}
			rerun = true
		}
	}

	if rerun {
		wfe, thread, err = i.rerunThreadWithRetry(ctx, c, wf, wfe.Status.ThreadName, opt.StepID, input)
		if err != nil {
			return nil, err
		}
	} else {
		wfe, thread, err = i.startWorkflow(ctx, c, wf, input, opt)
		if err != nil {
			return nil, err
		}
	}

	closedChan := make(chan types.Progress)
	close(closedChan)
	return &Response{
		cancel:            func() {},
		Thread:            thread,
		WorkflowExecution: wfe,
		Events:            closedChan,
	}, nil
}

func (i *Invoker) rerunThreadWithRetry(ctx context.Context, c kclient.WithWatch, wf *v1.Workflow, threadName, stepID, input string) (*v1.WorkflowExecution, *v1.Thread, error) {
	var (
		thread *v1.Thread
		wfe    *v1.WorkflowExecution
		err    error
	)
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		wfe, thread, err = i.rerunThread(ctx, c, wf, threadName, stepID, input)
		return err
	})
	return wfe, thread, err
}

func (i *Invoker) rerunThread(ctx context.Context, c kclient.WithWatch, wf *v1.Workflow, threadName, stepID, input string) (*v1.WorkflowExecution, *v1.Thread, error) {
	var (
		thread v1.Thread
		wfe    v1.WorkflowExecution
	)

	if err := c.Get(ctx, router.Key(wf.Namespace, threadName), &thread); err != nil {
		return nil, nil, err
	}

	if thread.Spec.WorkflowName != wf.Name {
		return nil, nil, fmt.Errorf("thread does not belong to workflow: %s", wf.Name)
	}

	if thread.Spec.WorkflowExecutionName == "" {
		return nil, nil, fmt.Errorf("thread does not have a workflow execution")
	}

	if err := unAbortThread(ctx, c, &thread); err != nil {
		return nil, nil, err
	}

	if err := c.Get(ctx, router.Key(wf.Namespace, thread.Spec.WorkflowExecutionName), &wfe); err != nil {
		return nil, nil, err
	}

	if wfe.Spec.Input != input {
		if stepID == "" {
			// If input doesn't match, delete all steps and rerun
			stepID = "*"
		}
		wfe.Spec.Input = input
	}

	if stepID != "*" {
		step, _ := types.FindStep(&wf.Spec.Manifest, stepID)
		if step == nil {
			return nil, nil, fmt.Errorf("step not found: %s", stepID)
		}
	}

	if stepID != "" {
		if err := i.deleteSteps(ctx, c, thread, stepID); err != nil {
			return nil, nil, err
		}
	}

	if thread.Status.CurrentRunName != "" || thread.Status.LastRunName != "" {
		thread.Status.CurrentRunName = ""
		thread.Status.LastRunName = ""
		if err := retention.SetLastUsedTime(ctx, c, &thread); err != nil {
			return nil, nil, err
		}
		if err := c.Status().Update(ctx, &thread); err != nil {
			return nil, nil, err
		}
	}

	wfe.Spec.WorkflowGeneration++
	wfe.Spec.RunUntilStep = stepID
	return &wfe, &thread, c.Update(ctx, &wfe)
}

func (i *Invoker) deleteSteps(ctx context.Context, c kclient.Client, thread v1.Thread, stepID string) error {
	var (
		steps v1.WorkflowStepList
	)

	if err := c.List(ctx, &steps, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.workflowExecutionName": thread.Spec.WorkflowExecutionName,
	}); err != nil {
		return err
	}

	if len(steps.Items) == 0 {
		return nil
	}

	for _, step := range steps.Items {
		if step.Status.State == types.WorkflowStateError || stepMatches(step.Spec.Step.ID, stepID) {
			if err := c.Delete(ctx, &step); kclient.IgnoreNotFound(err) != nil {
				return err
			}
		}
	}

	return nil
}

func stepMatches(left, right string) bool {
	if right == "*" {
		return true
	}
	return stepLookupID(left) == stepLookupID(right)
}

func stepLookupID(stepID string) string {
	id, _, _ := strings.Cut(stepID, "{")
	return id
}
