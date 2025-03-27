package projects

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) CopyProjectInfo(req router.Request, _ router.Response) error {
	projectThread := req.Object.(*v1.Thread)
	if !projectThread.Spec.Project || projectThread.Spec.ParentThreadName == "" {
		return nil
	}

	var parentThread v1.Thread
	if err := req.Get(&parentThread, projectThread.Namespace, projectThread.Spec.ParentThreadName); err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(projectThread.Spec.Manifest.ThreadManifestManagedFields, parentThread.Spec.Manifest.ThreadManifestManagedFields) {
		projectThread.Spec.Manifest.ThreadManifestManagedFields = parentThread.Spec.Manifest.ThreadManifestManagedFields
		return req.Client.Update(req.Ctx, projectThread)
	}

	return nil
}
