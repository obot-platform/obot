package handlers

import (
	"strings"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
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

	templateThread := v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix,
			Namespace:    projectThread.Namespace,
		},
		Spec: v1.ThreadSpec{
			Manifest:         projectThread.Spec.Manifest,
			AgentName:        projectThread.Spec.AgentName,
			SourceThreadName: projectThread.Name,
			UserID:           projectThread.Spec.UserID,
			Project:          true,
			Template:         true,
		},
	}

	if err := req.Create(&templateThread); err != nil {
		return err
	}

	return req.WriteCreated(convertTemplateThread(templateThread, nil))
}

func (h *TemplateHandler) UpdateProjectTemplate(req api.Context) error {
	var (
		templateID         = req.PathValue("template_id")
		templateThreadName = strings.Replace(templateID, system.ProjectPrefix, system.ThreadPrefix, 1)
		templateManifest   types.ProjectTemplateManifest
	)

	if err := req.Read(&templateManifest); err != nil {
		return err
	}

	if templateManifest.Featured {
		if !req.UserIsAdmin() {
			return types.NewErrForbidden("only admins can set a template to featured")
		}
		if !templateManifest.Public {
			return types.NewErrBadRequest("featured templates must be public")
		}
	}

	var templateThread v1.Thread
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var thread v1.Thread
		if err := req.Get(&thread, templateThreadName); err != nil {
			return err
		}

		if templateManifest.Name != "" && thread.Spec.Manifest.Name != templateManifest.Name {
			thread.Spec.Manifest.Name = templateManifest.Name
			if err := req.Update(&thread); err != nil {
				return err
			}
		}

		templateThread = thread
		return nil
	}); err != nil {
		return err
	}

	var templateThreadShare v1.ThreadShare
	if err := req.Get(&templateThreadShare, templateThreadName); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		templateThreadShare = v1.ThreadShare{
			ObjectMeta: metav1.ObjectMeta{
				Name:      templateThreadName,
				Namespace: templateThread.Namespace,
			},
		}
	}

	updated := templateThreadShare.DeepCopy()
	updated.Spec.UserID = templateThread.Spec.UserID
	updated.Spec.ProjectThreadName = templateThread.Name
	updated.Spec.Featured = templateManifest.Featured
	updated.Spec.Template = true
	updated.Spec.Manifest = types.ProjectShareManifest{
		Public: templateManifest.Public,
	}

	if updated.Spec.Manifest.Public && updated.Spec.PublicID == "" {
		updated.Spec.PublicID = strings.ReplaceAll(uuid.New().String(), "-", "")
	} else if !updated.Spec.Manifest.Public && updated.Spec.PublicID != "" {
		updated.Spec.PublicID = ""
	}

	var err error
	switch {
	case updated.CreationTimestamp.IsZero():
		err = req.Create(updated)
	case !equality.Semantic.DeepEqual(templateThreadShare.Spec, updated.Spec):
		err = req.Update(updated)
	}
	if err != nil {
		return err
	}

	return req.Write(convertTemplateThread(templateThread, updated))
}

func (h *TemplateHandler) DeleteProjectTemplate(req api.Context) error {
	var (
		templateID         = req.PathValue("template_id")
		templateThreadName = strings.Replace(templateID, system.ProjectPrefix, system.ThreadPrefix, 1)
	)

	var templateThread v1.Thread
	if err := req.Get(&templateThread, templateThreadName); err != nil {
		return err
	}

	return req.Delete(&templateThread)
}

func (h *TemplateHandler) CopyTemplate(req api.Context) error {
	var (
		publicID          = req.PathValue("template_public_id")
		templateShareList v1.ThreadShareList
	)

	if err := req.List(&templateShareList, kclient.InNamespace(req.Namespace()), kclient.MatchingFields{
		"spec.publicID": publicID,
		"spec.template": "true",
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
			Manifest:         templateThread.Spec.Manifest,
			AgentName:        templateThread.Spec.AgentName,
			SourceThreadName: templateThread.Name,
			UserID:           req.User.GetUID(),
			Project:          true,
		},
	}

	if err := req.Create(&newProject); err != nil {
		return err
	}

	return req.Write(convertProject(&newProject, nil))
}

func (h *TemplateHandler) GetProjectTemplate(req api.Context) error {
	var (
		templateID         = req.PathValue("template_id")
		templateThreadName = strings.Replace(templateID, system.ProjectPrefix, system.ThreadPrefix, 1)
	)

	var templateThread v1.Thread
	if err := req.Get(&templateThread, templateThreadName); err != nil {
		return err
	}

	var templateShareList v1.ThreadShareList
	if err := req.List(&templateShareList, kclient.MatchingFields{
		"spec.template":          "true",
		"spec.projectThreadName": templateThread.Name,
	}, kclient.Limit(1)); err != nil {
		return err
	}

	var templateShare *v1.ThreadShare
	if len(templateShareList.Items) > 0 {
		templateShare = &templateShareList.Items[0]
	}

	return req.Write(convertTemplateThread(templateThread, templateShare))
}

func (h *TemplateHandler) ListProjectTemplates(req api.Context) error {
	var (
		sourceProjectID  = req.PathValue("project_id")
		sourceThreadName = strings.Replace(sourceProjectID, system.ProjectPrefix, system.ThreadPrefix, 1)
		templateList     types.ProjectTemplateList
	)

	var templateThreadList v1.ThreadList
	if err := req.List(&templateThreadList, kclient.MatchingFields{
		"spec.template":         "true",
		"spec.sourceThreadName": sourceThreadName,
	}); err != nil {
		return err
	}

	if len(templateThreadList.Items) < 1 {
		return req.Write(templateList)
	}

	var templateShareList v1.ThreadShareList
	if err := req.List(&templateShareList, kclient.MatchingFields{
		"spec.template": "true",
	}); err != nil {
		return err
	}

	templateShares := make(map[string]v1.ThreadShare, len(templateShareList.Items))
	for _, templateShare := range templateShareList.Items {
		templateShares[templateShare.Spec.ProjectThreadName] = templateShare
	}

	for _, templateThread := range templateThreadList.Items {
		var templateShare *v1.ThreadShare
		if ts, ok := templateShares[templateThread.Name]; ok {
			templateShare = &ts
		}

		templateList.Items = append(templateList.Items, convertTemplateThread(templateThread, templateShare))
	}

	return req.Write(templateList)
}

func (h *TemplateHandler) ListTemplates(req api.Context) error {
	var (
		all          = req.UserIsAdmin() && req.URL.Query().Get("all") == "true"
		templateList types.ProjectTemplateList
	)

	shareSelector := kclient.MatchingFields{
		"spec.template": "true",
	}
	if !all {
		shareSelector["spec.public"] = "true"
	}

	var templateShareList v1.ThreadShareList
	if err := req.List(&templateShareList, shareSelector); err != nil {
		return err
	}

	templateShares := make(map[string]v1.ThreadShare, len(templateShareList.Items))
	for _, templateShare := range templateShareList.Items {
		templateShares[templateShare.Spec.ProjectThreadName] = templateShare
	}

	var templateThreadList v1.ThreadList
	if err := req.List(&templateThreadList, kclient.MatchingFields{
		"spec.template": "true",
	}); err != nil {
		return err
	}

	for _, templateThread := range templateThreadList.Items {
		var threadShare *v1.ThreadShare
		if ts, ok := templateShares[templateThread.Name]; ok {
			threadShare = &ts
		}

		if !all && threadShare == nil {
			continue
		}

		templateList.Items = append(templateList.Items, convertTemplateThread(templateThread, threadShare))
	}

	return req.Write(templateList)
}

func (h *TemplateHandler) GetTemplate(req api.Context) error {
	var (
		publicID          = req.PathValue("template_public_id")
		templateShareList v1.ThreadShareList
	)

	if err := req.List(&templateShareList, kclient.InNamespace(req.Namespace()), kclient.MatchingFields{
		"spec.publicID": publicID,
		"spec.template": "true",
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
