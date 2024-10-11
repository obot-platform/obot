package workflow

import (
	"github.com/acorn-io/baaah/pkg/router"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func EnsureIDs(req router.Request, resp router.Response) error {
	wf := req.Object.(*v1.Workflow)
	manifestWithIDS := PopulateIDs(wf.Spec.Manifest)
	if !equality.Semantic.DeepEqual(wf.Spec.Manifest, manifestWithIDS) {
		wf.Spec.Manifest = manifestWithIDS
		return req.Client.Update(req.Ctx, wf)
	}
	return nil
}

func WorkspaceObjects(req router.Request, _ router.Response) error {
	workflow := req.Object.(*v1.Workflow)
	if workflow.Status.WorkspaceName == "" {
		ws := &v1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    req.Namespace,
				GenerateName: system.WorkspacePrefix,
			},
			Spec: v1.WorkspaceSpec{
				WorkflowName: workflow.Name,
			},
		}
		if err := req.Client.Create(req.Ctx, ws); err != nil {
			return err
		}

		workflow.Status.WorkspaceName = ws.Name
	}

	return nil
}
