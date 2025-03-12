package threadshare

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

func CopyProjectInfo(req router.Request, _ router.Response) error {
	share := req.Object.(*v1.ThreadShare)
	if !share.Spec.Featured {
		return nil
	}

	var project v1.Thread
	if err := req.Get(&project, share.Namespace, share.Spec.ProjectThreadName); err != nil {
		return err
	}

	status := v1.ThreadShareStatus{
		Name:        project.Spec.Manifest.Name,
		Description: project.Spec.Manifest.Description,
		Icons:       project.Spec.Manifest.Icons,
		Tools:       project.Spec.Manifest.Tools,
	}

	if !equality.Semantic.DeepEqual(status, share.Status) {
		share.Status = status
		return req.Client.Status().Update(req.Ctx, share)
	}

	return nil
}
