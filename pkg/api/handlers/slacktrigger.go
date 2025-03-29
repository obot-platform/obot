package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SlackTriggerHandler struct{}

func NewSlackTriggerHandler() *SlackTriggerHandler {
	return &SlackTriggerHandler{}
}

func (h *SlackTriggerHandler) Create(req api.Context) error {
	var manifest types.SlackTriggerManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	// Validate manifest
	if manifest.WorkflowName == "" {
		return types.NewErrBadRequest("workflowName is required")
	}
	if manifest.ThreadName == "" {
		return types.NewErrBadRequest("threadName is required")
	}

	var existingTriggers v1.SlackTriggerList
	if err := req.List(&existingTriggers, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.threadName": manifest.ThreadName,
		}),
	}); err != nil {
		return err
	}

	if len(existingTriggers.Items) > 0 {
		return types.NewErrBadRequest("a slack trigger already exists for this project")
	}

	trigger := &v1.SlackTrigger{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.SlackTriggerPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.SlackTriggerSpec{
			WorkflowName: manifest.WorkflowName,
			ThreadName:   manifest.ThreadName,
		},
	}

	if err := req.Create(trigger); err != nil {
		return err
	}

	return req.WriteCreated(convertSlackTrigger(*trigger))
}

func (h *SlackTriggerHandler) Delete(req api.Context) error {
	var (
		id = req.PathValue("id")
	)

	return req.Delete(&v1.SlackTrigger{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (h *SlackTriggerHandler) List(req api.Context) error {
	var (
		threadName = req.Request.URL.Query().Get("threadName")
	)

	listOptions := &client.ListOptions{
		Namespace: req.Namespace(),
	}
	if threadName != "" {
		listOptions.FieldSelector = fields.SelectorFromSet(map[string]string{
			"spec.threadName": threadName,
		})
	}

	var list v1.SlackTriggerList
	if err := req.List(&list, listOptions); err != nil {
		return err
	}

	result := make([]types.SlackTrigger, 0, len(list.Items))
	for _, item := range list.Items {
		result = append(result, convertSlackTrigger(item))
	}

	return req.Write(types.SlackTriggerList{Items: result})
}

func (h *SlackTriggerHandler) ByID(req api.Context) error {
	var (
		id      = req.PathValue("id")
		trigger v1.SlackTrigger
	)

	if err := req.Get(&trigger, id); err != nil {
		return err
	}

	return req.Write(convertSlackTrigger(trigger))
}

func convertSlackTrigger(trigger v1.SlackTrigger) types.SlackTrigger {
	return types.SlackTrigger{
		Metadata: MetadataFrom(&trigger),
		SlackTriggerManifest: types.SlackTriggerManifest{
			WorkflowName: trigger.Spec.WorkflowName,
			ThreadName:   trigger.Spec.ThreadName,
		},
	}
}
