package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/controller/handlers/workflow"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const fieldSelector = "spec.workflow"

type WorkflowHandler struct{}

func NewWorkflowHandler() *WorkflowHandler {
	return &WorkflowHandler{}
}

func (a *WorkflowHandler) Update(req api.Context) error {
	var (
		id       = req.PathValue("id")
		wf       v1.Workflow
		manifest types.WorkflowManifest
	)

	if err := req.Read(&manifest); err != nil {
		return err
	}

	manifest = workflow.PopulateIDs(manifest)

	if err := req.Get(&wf, id); err != nil {
		return err
	}

	wf.Spec.Manifest = manifest
	if err := req.Update(&wf); err != nil {
		return err
	}

	resp, err := convertWorkflow(wf)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
}

func (a *WorkflowHandler) Delete(req api.Context) error {
	var (
		id             = req.PathValue("id")
		deleteTriggers = req.URL.Query().Get("delete-triggers")
	)

	if deleteTriggers == "true" {
		listOptions := &kclient.ListOptions{
			FieldSelector: fields.SelectorFromSet(map[string]string{
				fieldSelector: id,
			}),
			Namespace: req.Namespace(),
		}

		var webhooks v1.WebhookList
		if err := req.List(&webhooks, listOptions); err != nil {
			return err
		}

		for _, webhook := range webhooks.Items {
			if err := req.Delete(&webhook); err != nil && !apierror.IsNotFound(err) {
				return err
			}
		}

		var cronjobs v1.CronJobList
		if err := req.List(&cronjobs, listOptions); err != nil {
			return err
		}

		for _, cronjob := range cronjobs.Items {
			if err := req.Delete(&cronjob); err != nil && !apierror.IsNotFound(err) {
				return err
			}
		}

		var emailReceivers v1.EmailReceiverList
		if err := req.List(&emailReceivers, listOptions); err != nil {
			return err
		}

		for _, emailReceiver := range emailReceivers.Items {
			if err := req.Delete(&emailReceiver); err != nil && !apierror.IsNotFound(err) {
				return err
			}
		}
	}

	return req.Delete(&v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func convertWorkflow(workflow v1.Workflow) (*types.Workflow, error) {
	return &types.Workflow{
		Metadata:         MetadataFrom(&workflow),
		WorkflowManifest: workflow.Spec.Manifest,
		ThreadID:         workflow.Spec.ThreadName,
	}, nil
}

func (a *WorkflowHandler) ByID(req api.Context) error {
	var (
		workflow v1.Workflow
		id       = req.PathValue("id")
	)

	if err := req.Get(&workflow, id); err != nil {
		return err
	}

	resp, err := convertWorkflow(workflow)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
}

func (a *WorkflowHandler) List(req api.Context) error {
	var workflowList v1.WorkflowList
	if err := req.List(&workflowList); err != nil {
		return err
	}

	resp := make([]types.Workflow, 0, len(workflowList.Items))
	for _, workflow := range workflowList.Items {
		convertedWorkflow, err := convertWorkflow(workflow)
		if err != nil {
			return err
		}

		resp = append(resp, *convertedWorkflow)
	}

	return req.Write(types.WorkflowList{Items: resp})
}
