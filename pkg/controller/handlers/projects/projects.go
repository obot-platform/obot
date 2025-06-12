package projects

import (
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (*Handler) CopyProjectInfo(req router.Request, _ router.Response) error {
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

func (*Handler) CleanupChatbots(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Project || thread.Spec.ParentThreadName == "" {
		return nil
	}

	// Get the parent thread
	var parentThread v1.Thread
	if err := req.Get(&parentThread, thread.Namespace, thread.Spec.ParentThreadName); apierrors.IsNotFound(err) {
		// Bail out early if the parent thread doesn't exist, this project will get cleaned up anyway.
		return nil
	} else if err != nil {
		return err
	}

	// Check if the project share exists
	var (
		parentProjectID  = strings.Replace(parentThread.Name, system.ThreadPrefix, system.ProjectPrefix, 1)
		projectShareName = system.GetProjectShareName(parentThread.Spec.UserID, parentProjectID)
		threadShare      v1.ThreadShare
	)

	if err := req.Get(&threadShare, thread.Namespace, projectShareName); apierrors.IsNotFound(err) {
		// Project share doesn't exist, delete the child project thread
		return kclient.IgnoreNotFound(req.Delete(thread))
	} else if err != nil {
		return err
	}

	return nil
}
