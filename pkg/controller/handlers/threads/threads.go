package threads

import (
	"fmt"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/otto/pkg/aihelper"
	"github.com/gptscript-ai/otto/pkg/knowledge"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	wclient "github.com/thedadams/workspace-provider/pkg/client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type ThreadHandler struct {
	workspace *wclient.Client
	ingester  *knowledge.Ingester
	aihelper  *aihelper.AIHelper
}

func New(workspace *wclient.Client, ingester *knowledge.Ingester, aihelper *aihelper.AIHelper) *ThreadHandler {
	return &ThreadHandler{
		workspace: workspace,
		ingester:  ingester,
		aihelper:  aihelper,
	}
}

func (t *ThreadHandler) Cleanup(req router.Request, resp router.Response) error {
	thread := req.Object.(*v1.Thread)

	if thread.Spec.AgentName != "" {
		var agent v1.Agent
		if err := req.Get(&agent, thread.Namespace, thread.Spec.AgentName); apierrors.IsNotFound(err) {
			return req.Delete(thread)
		} else if err != nil {
			return err
		}
	}

	if thread.Spec.WorkflowName != "" {
		var wf v1.Workflow
		if err := req.Get(&wf, thread.Namespace, thread.Spec.WorkflowName); apierrors.IsNotFound(err) {
			return req.Delete(thread)
		} else if err != nil {
			return err
		}
	}

	if thread.Spec.WorkflowStepName != "" {
		var step v1.WorkflowStep
		if err := req.Get(&step, thread.Namespace, thread.Spec.WorkflowStepName); apierrors.IsNotFound(err) {
			return req.Delete(thread)
		} else if err != nil {
			return err
		}
	}

	if thread.Spec.WorkflowExecutionName != "" {
		var we v1.WorkflowExecution
		if err := req.Get(&we, thread.Namespace, thread.Spec.WorkflowExecutionName); apierrors.IsNotFound(err) {
			return req.Delete(thread)
		} else if err != nil {
			return err
		}
	}

	return nil
}

func (t *ThreadHandler) Description(req router.Request, resp router.Response) error {
	thread := req.Object.(*v1.Thread)

	if thread.Status.Description != "" || thread.Status.LastRunName == "" {
		return nil
	}

	var run v1.Run
	if err := req.Get(&run, thread.Namespace, thread.Status.LastRunName); err != nil {
		return err
	}

	for run.Spec.PreviousRunName != "" {
		var prevRun v1.Run
		if err := req.Get(&prevRun, thread.Namespace, run.Spec.PreviousRunName); err != nil {
			return err
		}
		if prevRun.Spec.ThreadName == thread.Name {
			run = prevRun
		} else {
			break
		}
	}

	return t.aihelper.GenerateObject(req.Ctx, &thread.Status.Description,
		"Given the following start of a conversation, generate a short title of the conversation",
		fmt.Sprintf(`User: %s\n
Assistant: %s\n`, run.Spec.Input, run.Status.Output))
}
