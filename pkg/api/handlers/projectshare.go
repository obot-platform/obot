package handlers

import (
	"strings"

	"github.com/google/uuid"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ProjectShareHandler struct {
}

func NewProjectShareHandler() *ProjectShareHandler {
	return &ProjectShareHandler{}
}

func (h *ProjectShareHandler) CreateShare(req api.Context) error {
	var (
		threadShareManifest types.ProjectShareManifest
		projectID           = req.PathValue("project_id")
		projectShareName    = h.getProjectShareName(req.User, projectID)
	)

	if err := req.Read(&threadShareManifest); err != nil {
		return err
	}

	threadShare := v1.ThreadShare{
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectShareName,
			Namespace: req.Namespace(),
		},
		Spec: v1.ThreadShareSpec{
			Manifest:          threadShareManifest,
			UserID:            req.User.GetUID(),
			PublicID:          strings.ReplaceAll(uuid.New().String(), "-", ""),
			ProjectThreadName: strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1),
		},
	}
	if err := req.Create(&threadShare); err != nil {
		return err
	}

	return req.WriteCreated(convertProjectShare(threadShare))
}

func (h *ProjectShareHandler) getProjectShareName(user user.Info, projectID string) string {
	return name.SafeHashConcatName(system.ThreadSharePrefix, user.GetUID(),
		strings.Replace(projectID, system.ThreadPrefix, system.ProjectPrefix, 1))
}

func (h *ProjectShareHandler) GetShare(req api.Context) error {
	var (
		threadShare      v1.ThreadShare
		projectID        = req.PathValue("project_id")
		projectShareName = h.getProjectShareName(req.User, projectID)
	)

	if err := req.Get(&threadShare, projectShareName); apierrors.IsNotFound(err) {
		return req.Write(convertProjectShare(v1.ThreadShare{
			Spec: v1.ThreadShareSpec{
				ProjectThreadName: projectID,
			},
		}))
	}

	return req.Write(convertProjectShare(threadShare))
}

func (h *ProjectShareHandler) ListFeatured(req api.Context) error {
	var (
		threadShareList v1.ThreadShareList
	)

	if err := req.List(&threadShareList, kclient.InNamespace(req.Namespace()), kclient.MatchingFields{
		"spec.featured": "true",
	}); err != nil {
		return err
	}

	projectShares := make([]types.ProjectShare, 0, len(threadShareList.Items))
	for _, threadShare := range threadShareList.Items {
		projectShares = append(projectShares, convertProjectShare(threadShare))
	}

	return req.Write(types.ProjectShareList{
		Items: projectShares,
	})
}

func (h *ProjectShareHandler) SetFeatured(req api.Context) error {
	var (
		threadShare      v1.ThreadShare
		projectID        = req.PathValue("project_id")
		projectShareName = h.getProjectShareName(req.User, projectID)
	)

	if err := req.Get(&threadShare, projectShareName); err != nil {
		return err
	}

	var featured struct {
		Featured bool `json:"featured"`
	}

	if err := req.Read(&featured); err != nil {
		return err
	}

	threadShare.Spec.Featured = featured.Featured
	if err := req.Update(&threadShare); err != nil {
		return err
	}

	return req.Write(convertProjectShare(threadShare))
}

func (h *ProjectShareHandler) UpdateShare(req api.Context) error {
	var (
		threadShare      v1.ThreadShare
		manifest         types.ProjectShareManifest
		projectID        = req.PathValue("project_id")
		projectShareName = h.getProjectShareName(req.User, projectID)
	)

	if err := req.Get(&threadShare, projectShareName); err != nil {
		return err
	}

	if err := req.Read(&manifest); err != nil {
		return err
	}

	threadShare.Spec.Manifest = manifest
	if err := req.Update(&threadShare); err != nil {
		return err
	}

	return req.Write(convertProjectShare(threadShare))
}

func (h *ProjectShareHandler) DeleteShare(req api.Context) error {
	var (
		projectID        = req.PathValue("project_id")
		projectShareName = h.getProjectShareName(req.User, projectID)
	)

	return req.Delete(&v1.ThreadShare{
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectShareName,
			Namespace: req.Namespace(),
		},
	})
}

func convertProjectShare(threadShare v1.ThreadShare) types.ProjectShare {
	return types.ProjectShare{
		Metadata:             MetadataFrom(&threadShare),
		ProjectShareManifest: threadShare.Spec.Manifest,
		PublicID:             threadShare.Spec.PublicID,
		Featured:             threadShare.Spec.Featured,
		ProjectID:            strings.Replace(threadShare.Spec.ProjectThreadName, system.ThreadPrefix, system.ProjectPrefix, 1),
		Name:                 threadShare.Status.Name,
		Description:          threadShare.Status.Description,
		Icons:                threadShare.Status.Icons,
	}
}

func (h *ProjectShareHandler) CreateProjectFromShare(req api.Context) error {
	var (
		shareID         = req.PathValue("share_public_id")
		threadShareList v1.ThreadShareList
		baseProject     v1.Thread
		id              = name.SafeHashConcatName(system.ThreadPrefix, req.User.GetUID(), shareID)
	)

	if err := req.Get(&baseProject, id); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil {
		return req.Write(convertProject(&baseProject))
	}

	if err := req.List(&threadShareList, kclient.InNamespace(req.Namespace()), kclient.MatchingFields{
		"spec.publicID": shareID,
	}); err != nil {
		return err
	}

	if len(threadShareList.Items) == 0 {
		return types.NewErrNotFound("share not found %s", shareID)
	}

	if err := req.Get(&baseProject, threadShareList.Items[0].Spec.ProjectThreadName); err != nil {
		return err
	}

	if baseProject.Spec.UserID == req.User.GetUID() {
		return req.Write(convertProject(&baseProject))
	}

	newProject, err := invoke.CreateProjectFromProject(req.Context(), req.Storage, &baseProject, id, req.User.GetUID())
	if apierrors.IsAlreadyExists(err) {
		if err := req.Get(&baseProject, id); err != nil {
			return err
		}
		return req.Write(convertProject(&baseProject))
	} else if err != nil {
		return err
	}

	return req.Write(convertProject(newProject))
}
