package webhook

import (
	"github.com/acorn-io/acorn/apiclient/types"
	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/acorn-io/nah/pkg/router"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) SetSuccessRunTime(req router.Request, _ router.Response) error {
	wh := req.Object.(*v1.Webhook)

	var workflowExecutions v1.WorkflowExecutionList
	if err := req.List(&workflowExecutions, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{"spec.webhookName": wh.Name}),
		Namespace:     wh.Namespace,
	}); err != nil {
		return err
	}

	for _, trigger := range workflowExecutions.Items {
		if trigger.Status.State == types.WorkflowStateComplete && (wh.Status.LastSuccessfulRunCompleted == nil || wh.Status.LastSuccessfulRunCompleted.Before(trigger.Status.EndTime)) {
			wh.Status.LastSuccessfulRunCompleted = trigger.Status.EndTime
		}
	}

	return nil
}
