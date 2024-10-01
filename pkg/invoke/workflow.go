package invoke

import (
	"context"
	"fmt"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/otto/pkg/events"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type WorkflowOptions struct {
	ThreadName string
	Background bool
}

func (i *Invoker) Workflow(ctx context.Context, c kclient.WithWatch, wf *v1.Workflow, input string, opt WorkflowOptions) (*Response, error) {
	if opt.ThreadName != "" {
		agent, err := i.toAgent(ctx, c, wf, &v1.WorkflowStep{}, input, wf.Spec.Manifest)
		if err != nil {
			return nil, err
		}

		return i.Agent(ctx, c, &agent, input, Options{
			ThreadName: opt.ThreadName,
		})
	}

	wfe := &v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowExecutionPrefix,
			Namespace:    wf.Namespace,
			Finalizers:   []string{v1.WorkflowExecutionFinalizer},
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

		if wfe.Status.ThreadName != "" {
			var thread v1.Thread
			if err := c.Get(ctx, router.Key(wfe.Namespace, wfe.Status.ThreadName), &thread); err != nil {
				return nil, err
			}
			if opt.Background {
				return &Response{
					Thread: &thread,
				}, nil
			}

			resp, err := i.events.Watch(ctx, wfe.Namespace, events.WatchOptions{
				History:    true,
				ThreadName: wfe.Status.ThreadName,
				Follow:     true,
			})
			if err != nil {
				continue
			}
			return &Response{
				Thread: &thread,
				Events: resp,
			}, nil
		}
	}

	return nil, fmt.Errorf("workflow did not start")
}
