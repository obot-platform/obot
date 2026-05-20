package threads

import (
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (t *Handler) CleanupEphemeralThreads(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Ephemeral ||
		thread.CreationTimestamp.After(time.Now().Add(-12*time.Hour)) {
		return nil
	}

	log.Infof("Deleting expired ephemeral thread: thread=%s createdAt=%s", thread.Name, thread.CreationTimestamp.Format(time.RFC3339))
	return kclient.IgnoreNotFound(req.Delete(thread))
}

// CleanupOldThreads deletes any threads with the system thread prefix. These threads were used for chat.
// Any threads that are still needed will have a system thread prefix to distinguish them.
func (t *Handler) CleanupOldThreads(req router.Request, _ router.Response) error {
	if strings.HasPrefix(req.Name, system.ThreadPrefix) {
		return kclient.IgnoreNotFound(req.Delete(req.Object))
	}
	return nil
}

func (t *Handler) RemoveOldFinalizers(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)

	finalizerCount := len(thread.Finalizers)
	thread.Finalizers = slices.DeleteFunc(thread.Finalizers, func(finalizer string) bool {
		return finalizer == v1.ThreadFinalizer+"-child-cleanup" || finalizer == v1.MCPServerFinalizer
	})

	if finalizerCount != len(thread.Finalizers) {
		log.Infof("Removing deprecated thread finalizers: thread=%s removed=%d", thread.Name, finalizerCount-len(thread.Finalizers))
		return req.Client.Update(req.Ctx, thread)
	}
	return nil
}
