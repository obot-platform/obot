package workflow

import (
	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/acorn-io/nah/pkg/router"
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
