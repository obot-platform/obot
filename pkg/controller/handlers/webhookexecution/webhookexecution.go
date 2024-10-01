package webhookexecution

import (
	"strings"

	"github.com/acorn-io/baaah/pkg/apply"
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/otto/pkg/invoke"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/system"
	wclient "github.com/thedadams/workspace-provider/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	workspaceClient *wclient.Client
	invoker         *invoke.Invoker
}

func New(wc *wclient.Client, invoker *invoke.Invoker) *Handler {
	return &Handler{
		workspaceClient: wc,
		invoker:         invoker,
	}
}

func (h *Handler) Run(req router.Request, resp router.Response) error {
	we := req.Object.(*v1.WebhookExecution)

	if we.Status.State == v1.WebhookStateComplete || we.Status.State == v1.WebhookStateError {
		// The webhook has already run
		return nil
	}

	var wfe v1.WorkflowExecution
	if err := kclient.IgnoreNotFound(req.Get(&wfe, we.Namespace, system.WebHookExecutionPrefix+we.Name)); err != nil {
		return err
	}

	we.Status.Output = wfe.Status.Output
	we.Status.State = v1.WebhookState(wfe.Status.State)
	// If the workflow execution object is not found, then this if-block will be skipped.
	if we.Status.State == v1.WebhookStateComplete || we.Status.State == v1.WebhookStateError {
		return nil
	}

	var wh v1.Webhook
	if err := req.Get(&wh, we.Namespace, we.Spec.WebhookName); err != nil {
		return err
	}

	var input strings.Builder
	_, _ = input.WriteString("You are being called from a webhook.\n\n")
	if we.Spec.Payload != "" {
		_, _ = input.WriteString("Here is the payload of the webhook:\n")
		_, _ = input.WriteString(we.Spec.Payload)
	}
	if len(we.Spec.Headers) > 0 {
		_, _ = input.WriteString("\nHere are the headers of the webhook:\n")
		for k, v := range we.Spec.Headers {
			input.WriteString("\n")
			input.WriteString(k)
			input.WriteString(": ")
			input.WriteString(v)
		}
	}

	resp.Objects(&v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.WorkflowExecutionPrefix + we.Name,
			Namespace: we.Namespace,
			Annotations: map[string]string{
				// Don't prune the workflow execution object because we want it to live as long as the webhook exists.
				apply.AnnotationPrune: "false",
			},
		},
		Spec: v1.WorkflowExecutionSpec{
			Input:                 input.String(),
			WorkflowName:          wh.Spec.WorkflowName,
			WebhookExecutionName:  we.Name,
			AfterWorkflowStepName: wh.Spec.AfterWorkflowStepName,
		},
	})

	we.Status.State = v1.WebhookStateRunning
	return nil
}
