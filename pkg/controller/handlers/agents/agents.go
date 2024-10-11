package agents

import (
	"github.com/acorn-io/baaah/pkg/router"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func WorkspaceObjects(req router.Request, _ router.Response) error {
	agent := req.Object.(*v1.Agent)
	if agent.Status.WorkspaceName == "" {
		ws := &v1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    req.Namespace,
				GenerateName: system.WorkspacePrefix,
			},
			Spec: v1.WorkspaceSpec{
				AgentName: agent.Name,
			},
		}
		if err := req.Client.Create(req.Ctx, ws); err != nil {
			return err
		}

		agent.Status.WorkspaceName = ws.Name
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		ws := &v1.KnowledgeSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    req.Namespace,
				GenerateName: system.KnowledgeSetPrefix,
			},
			Spec: v1.KnowledgeSetSpec{
				AgentName: agent.Name,
			},
		}
		if err := req.Client.Create(req.Ctx, ws); err != nil {
			return err
		}

		agent.Status.KnowledgeSetNames = append(agent.Status.KnowledgeSetNames, ws.Name)
		if err := req.Client.Status().Update(req.Ctx, agent); err != nil {
			return err
		}
	}

	return nil
}
