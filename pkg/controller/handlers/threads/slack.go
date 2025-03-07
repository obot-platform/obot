package threads

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *Handler) SlackCapability(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Project {
		return nil
	}

	var (
		slackReceiver v1.SlackReceiver
		receiverName  string
		enabled       bool
	)
	if thread.Spec.ParentThreadName == "" {
		receiverName = system.SlackReceiverPrefix + thread.Name
	} else {
		receiverName = system.SlackReceiverPrefix + thread.Spec.ParentThreadName
	}

	if err := req.Get(&slackReceiver, thread.Namespace, receiverName); kclient.IgnoreNotFound(err) != nil {
		return err
	} else if err == nil {
		enabled = true
	}

	if thread.Spec.Capabilities.OnSlackMessage != enabled {
		thread.Spec.Capabilities.OnSlackMessage = enabled
		return req.Client.Update(req.Ctx, thread)
	}

	return nil
}
