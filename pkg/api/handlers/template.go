package handlers

import (
	"context"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type TemplateHandler struct{}

func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

func (h *TemplateHandler) CreateProjectTemplate(req api.Context) error {
	var (
		projectThread     v1.Thread
		projectID         = req.PathValue("project_id")
		projectThreadName = strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)
	)

	if err := req.Get(&projectThread, projectThreadName); err != nil {
		return err
	}

	for projectThread.Spec.ParentThreadName != "" {
		if err := req.Get(&projectThread, projectThread.Spec.ParentThreadName); err != nil {
			return err
		}
	}

	if !projectThread.Spec.Project || projectThread.Spec.Template {
		return types.NewErrBadRequest("invalid project %s", projectID)
	}

	// Attempt to get the project snapshot by name
	var (
		snapshotThread     v1.Thread
		snapshotThreadName = strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1) + "-snapshot"
	)
	if err := req.Get(&snapshotThread, snapshotThreadName); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		// No snapshot currently exists, create the initial snapshot
		snapshotThread = v1.Thread{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotThreadName,
				Namespace: projectThread.Namespace,
			},
			Spec: v1.ThreadSpec{
				AgentName:        projectThread.Spec.AgentName,
				SourceThreadName: projectThread.Name,
				UserID:           projectThread.Spec.UserID,
				Project:          true,
				Template:         true,
				UpgradeApproved:  true,
			},
		}
		if err := req.Create(&snapshotThread); err != nil {
			return err
		}

		return req.WriteCreated(convertTemplateThread(snapshotThread, nil))
	}

	// Found existing snapshot
	if !snapshotThread.Status.UpgradeAvailable || snapshotThread.Spec.UpgradeApproved {
		// The project hasn't diverged from the snapshot, return the current snapshot
		return req.Write(convertTemplateThread(snapshotThread, nil))
	}

	// The project has diverged from the snapshot.
	// Set the flag to trigger an upgrade and update the LastUpdated timestamp.
	snapshotThread.Spec.UpgradeApproved = true
	if err := req.Update(&snapshotThread); err != nil {
		return err
	}

	return req.Write(convertTemplateThread(snapshotThread, nil))
}

func (h *TemplateHandler) DeleteProjectTemplate(req api.Context) error {
	var (
		projectID         = req.PathValue("project_id")
		projectThreadName = strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)
	)

	// Find the template thread that was created from this project
	var templateThreadList v1.ThreadList
	if err := req.List(&templateThreadList, kclient.MatchingFields{
		"spec.sourceThreadName": projectThreadName,
	}); err != nil {
		return err
	}

	var templateThread *v1.Thread
	for _, thread := range templateThreadList.Items {
		if thread.Spec.Template {
			templateThread = &thread
			break
		}
	}

	if templateThread == nil {
		return types.NewErrNotFound("template not found for project %s", projectID)
	}

	return req.Delete(templateThread)
}

func (h *TemplateHandler) CopyTemplate(req api.Context) error {
	var (
		publicID          = req.PathValue("template_public_id")
		templateShareList v1.ThreadShareList
	)

	if err := req.List(&templateShareList, kclient.InNamespace(req.Namespace()), kclient.MatchingFields{
		"spec.publicID": publicID,
	}, kclient.Limit(1)); err != nil {
		return err
	}

	if len(templateShareList.Items) < 1 {
		return types.NewErrNotFound("template not found: %s", publicID)
	}

	var templateThread v1.Thread
	if err := req.Get(&templateThread, templateShareList.Items[0].Spec.ProjectThreadName); err != nil {
		return err
	}

	newProject := v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ThreadSpec{
			AgentName:        templateThread.Spec.AgentName,
			SourceThreadName: templateThread.Name,
			UserID:           req.User.GetUID(),
			Project:          true,
			UpgradeApproved:  true, // Approve the initial upgrade
		},
	}

	if err := req.Create(&newProject); err != nil {
		return err
	}

	return req.Write(convertProject(&newProject, nil))
}

func (h *TemplateHandler) GetProjectTemplate(req api.Context) error {
	var (
		projectID         = req.PathValue("project_id")
		projectThreadName = strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)
	)

	// Find the template thread that was created from this project
	var templateThreadList v1.ThreadList
	if err := req.List(&templateThreadList, kclient.MatchingFields{
		"spec.sourceThreadName": projectThreadName,
	}); err != nil {
		return err
	}

	var templateThread *v1.Thread
	for _, thread := range templateThreadList.Items {
		if thread.Spec.Template {
			templateThread = &thread
			break
		}
	}
	if templateThread == nil {
		return types.NewErrNotFound("template not found for project %s", projectID)
	}

	var templateShareList v1.ThreadShareList
	if err := req.List(&templateShareList, kclient.MatchingFields{
		"spec.projectThreadName": templateThread.Name,
	}); err != nil {
		return err
	}

	var templateShare *v1.ThreadShare
	for _, share := range templateShareList.Items {
		if share.Spec.Template {
			templateShare = &share
			break
		}
	}

	return req.Write(convertTemplateThread(*templateThread, templateShare))
}

func (h *TemplateHandler) GetTemplate(req api.Context) error {
	var (
		publicID          = req.PathValue("template_public_id")
		templateShareList v1.ThreadShareList
	)

	if err := req.List(&templateShareList, kclient.InNamespace(req.Namespace()), kclient.MatchingFields{
		"spec.publicID": publicID,
	}, kclient.Limit(1)); err != nil {
		return err
	}

	if len(templateShareList.Items) < 1 {
		return types.NewErrNotFound("template not found: %s", publicID)
	}

	var templateThread v1.Thread
	if err := req.Get(&templateThread, templateShareList.Items[0].Spec.ProjectThreadName); err != nil {
		return err
	}

	return req.Write(convertTemplateThread(templateThread, &templateShareList.Items[0]))
}

// getTemplateSet returns a set of template thread names for all templates
func getTemplateSet(ctx context.Context, c kclient.Client) (map[string]struct{}, error) {
	var templateThreadList v1.ThreadList
	if err := c.List(ctx, &templateThreadList, kclient.MatchingFields{
		"spec.template": "true",
	}); err != nil {
		return nil, err
	}

	templateSet := make(map[string]struct{}, len(templateThreadList.Items))
	for _, thread := range templateThreadList.Items {
		templateSet[thread.Name] = struct{}{}
	}

	return templateSet, nil
}
