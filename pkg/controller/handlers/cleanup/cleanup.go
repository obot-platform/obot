package cleanup

import (
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/acorn-io/baaah/pkg/uncached"
	"github.com/otto8-ai/otto8/logger"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type refs interface {
	DeleteRefs() []v1.Ref
}

var log = logger.Package()

func Cleanup(req router.Request, _ router.Response) error {
	toDelete := req.Object.(refs)

	for _, ref := range toDelete.DeleteRefs() {
		if ref.Name == "" {
			continue
		}

		if _, ok := ref.ObjType.(*v1.Workflow); ok {
			if !system.IsWorkflowID(ref.Name) {
				ref.ObjType = new(v1.Reference)
			}
		} else if _, ok = ref.ObjType.(*v1.Agent); ok {
			if !system.IsAgentID(ref.Name) {
				ref.ObjType = new(v1.Reference)
			}
		}
		if err := req.Get(ref.ObjType, req.Namespace, ref.Name); apierrors.IsNotFound(err) {
			if err := req.Get(uncached.Get(ref.ObjType), req.Namespace, ref.Name); apierrors.IsNotFound(err) {
				log.Infof("Deleting %s/%s due to missing %s", req.Namespace, req.Name, ref.Name)
				return req.Delete(req.Object)
			}
		}
	}

	return nil
}
