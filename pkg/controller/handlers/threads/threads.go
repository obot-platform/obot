package threads

import (
	"fmt"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/otto/pkg/aihelper"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ThreadHandler struct {
	aihelper *aihelper.AIHelper
}

func New(aihelper *aihelper.AIHelper) *ThreadHandler {
	return &ThreadHandler{
		aihelper: aihelper,
	}
}

func (t *ThreadHandler) Description(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)

	if thread.Spec.Manifest.Description != "" || thread.Status.LastRunName == "" {
		return nil
	}

	var run v1.Run
	if err := req.Get(&run, thread.Namespace, thread.Status.LastRunName); err != nil {
		return err
	}

	for run.Spec.PreviousRunName != "" {
		var prevRun v1.Run
		if err := req.Get(&prevRun, thread.Namespace, run.Spec.PreviousRunName); err != nil {
			return err
		}
		if prevRun.Spec.ThreadName == thread.Name {
			run = prevRun
		} else {
			break
		}
	}

	var desc string
	err := t.aihelper.GenerateObject(req.Ctx, &desc,
		`Given the following start of a conversation, generate a short title of the conversation.
Output just your suggested title, without quotes or any other text.`,
		fmt.Sprintf(`User: %s\n
Assistant: %s\n`, run.Spec.Input, run.Status.Output))
	if err != nil {
		return err
	}

	thread.Spec.Manifest.Description = desc
	return req.Client.Update(req.Ctx, thread)
}

func (t *ThreadHandler) CreateWorkspaces(req router.Request, resp router.Response) error {
	thread := req.Object.(*v1.Thread)
	resp.Objects(
		&v1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: thread.Namespace,
				Name:      system.WorkspacePrefix + thread.Name,
			},
			Spec: v1.WorkspaceSpec{
				ThreadName:  thread.Name,
				WorkspaceID: thread.Spec.WorkspaceID,
			},
		},
		&v1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: thread.Namespace,
				Name:      system.WorkspacePrefix + "knowledge" + thread.Name,
			},
			Spec: v1.WorkspaceSpec{
				ThreadName:  thread.Name,
				WorkspaceID: thread.Spec.KnowledgeWorkspaceID,
				IsKnowledge: true,
			},
		},
	)

	return nil
}
