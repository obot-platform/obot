package workflow

import (
	"context"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func SetAdditionalCredentialContexts(ctx context.Context, client kclient.Client) error {
	var workflows v1.WorkflowList
	if err := client.List(ctx, &workflows); err != nil {
		return err
	}

	for _, wf := range workflows.Items {
		if len(wf.Spec.AdditionalCredentialContexts) != 0 {
			continue
		}

		var thread v1.Thread
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: wf.Namespace, Name: wf.Spec.ThreadName}, &thread); err != nil || thread.Spec.AgentName == "" {
			return err
		}

		wf.Spec.AdditionalCredentialContexts = []string{thread.Spec.AgentName}
		if err := client.Update(ctx, &wf); err != nil {
			return err
		}
	}
	return nil
}
