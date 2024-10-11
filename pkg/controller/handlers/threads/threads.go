package threads

import (
	"time"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/otto8-ai/otto8/pkg/invoke"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func WorkflowState(req router.Request, _ router.Response) error {
	var (
		thread = req.Object.(*v1.Thread)
		wfe    v1.WorkflowExecution
	)

	if thread.Spec.WorkflowExecutionName != "" {
		if err := req.Get(&wfe, thread.Namespace, thread.Spec.WorkflowExecutionName); err != nil {
			return err
		}
		thread.Status.WorkflowState = wfe.Status.State
	}

	return nil
}

func PurgeSystemThread(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if thread.Labels[invoke.SystemThreadLabel] != "true" || !thread.Status.LastRunState.IsTerminal() {
		return nil
	}

	// Delete if the thread is older than the TTL
	if thread.CreationTimestamp.Add(invoke.SystemThreadTTL).Before(time.Now()) {
		return req.Delete(thread)
	}

	return nil
}

func CreateWorkspaces(req router.Request, resp router.Response) error {
	thread := req.Object.(*v1.Thread)
	resp.Objects(
		&v1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: thread.Namespace,
				Name:      system.WorkspacePrefix + thread.Name,
			},
			Spec: v1.WorkspaceSpec{
				ThreadName:  thread.Name,
				WorkspaceID: thread.Spec.WorkspaceID,
			},
		},
	)

	return nil
}
