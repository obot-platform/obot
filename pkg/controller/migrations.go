package controller

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func setWorkflowAdditionalCredentialContexts(req router.Request, _ router.Response) error {
	wf := req.Object.(*v1.Workflow)

	if len(wf.Spec.AdditionalCredentialContexts) != 0 || wf.Spec.ThreadName == "" {
		return nil
	}

	var thread v1.Thread
	if err := req.Client.Get(req.Ctx, kclient.ObjectKey{Namespace: wf.Namespace, Name: wf.Spec.ThreadName}, &thread); err != nil {
		return err
	}

	if thread.Spec.AgentName == "" {
		return nil
	}

	wf.Spec.AdditionalCredentialContexts = []string{thread.Spec.AgentName}
	if err := req.Client.Update(req.Ctx, wf); err != nil {
		return err
	}

	return nil
}
