package uploads

import (
	"github.com/acorn-io/baaah/pkg/router"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (u *UploadHandler) CleanupSyncRequests(req router.Request, _ router.Response) error {
	reSyncUploadRequest := req.Object.(*v1.SyncUploadRequest)

	var upload v1.RemoteKnowledgeSource
	// Use !Before here for checking time because the lastReSyncStarted time and the sync request maybe the at the same second.
	if err := req.Get(&upload, reSyncUploadRequest.Namespace, reSyncUploadRequest.Spec.RemoteKnowledgeSourceName); apierrors.IsNotFound(err) || err == nil && !upload.Status.LastReSyncStarted.Before(&reSyncUploadRequest.CreationTimestamp) {
		return kclient.IgnoreNotFound(req.Client.Delete(req.Ctx, reSyncUploadRequest))
	} else if err != nil {
		return err
	}

	return nil
}
