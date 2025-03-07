package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	var workflow v1.Workflow
	if err := req.Get(&workflow, manifest.WorkflowName); err != nil {
		return err
	}

	var thread v1.Thread
	if err := req.Get(&thread, workflow.Spec.ThreadName); err != nil {
		return err
	}

	if manifest.TeamID != thread.Status.SlackConfiguration.Teams.ID {
		return types.NewErrBadRequest("teamID does not match thread teamID")
	}

	// Check if trigger already exists for this workflow
	var existingTriggers v1.SlackTriggerList
	if err := req.List(&existingTriggers); err != nil {
		return err
	}

	for _, t := range existingTriggers.Items {
		if t.Spec.WorkflowName == manifest.WorkflowName {
			return types.NewErrBadRequest("slack trigger already exists for this workflow")
		}
		if t.Spec.TeamID == manifest.TeamID {
			return types.NewErrBadRequest("slack trigger already exists for this team ID")
		}
	}

	trigger := &v1.SlackTrigger{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.SlackTriggerPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.SlackTriggerSpec{
			WorkflowName: manifest.WorkflowName,
			TeamID:       manifest.TeamID,
		},
	}

	if err := req.Create(trigger); err != nil {
		return err
	}

	return req.WriteCreated(convertSlackTrigger(*trigger))
}

func (h *SlackTriggerHandler) Update(req api.Context) error {
	var (
		id      = req.PathValue("id")
		trigger v1.SlackTrigger
	)

	if err := req.Get(&trigger, id); err != nil {
		return err
	}

	var manifest types.SlackTriggerManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	var existingTriggers v1.SlackTriggerList
	if err := req.List(&existingTriggers); err != nil {
		return err
	}

	for _, t := range existingTriggers.Items {
		if t.Spec.TeamID == manifest.TeamID && t.Name != id {
			return types.NewErrBadRequest("slack trigger already exists for this team ID")
		}
	}

	trigger.Spec.WorkflowName = manifest.WorkflowName
	trigger.Spec.TeamID = manifest.TeamID

	if err := req.Update(&trigger); err != nil {
		return err
	}

	return req.Write(convertSlackTrigger(trigger))
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
	var list v1.SlackTriggerList
	if err := req.List(&list); err != nil {
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
			TeamID:       trigger.Spec.TeamID,
		},
	}
}
