package workflow

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

func EnsureIDs(req router.Request, _ router.Response) error {
	wf := req.Object.(*v1.Workflow)
	manifestWithIDs := PopulateIDs(wf.Spec.Manifest)
	if !equality.Semantic.DeepEqual(wf.Spec.Manifest, manifestWithIDs) {
		wf.Spec.Manifest = manifestWithIDs
		return req.Client.Update(req.Ctx, wf)
	}
	return nil
}
