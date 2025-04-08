package retention

import (
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Manager struct {
	c      kclient.Client
	policy time.Duration
}

func NewRetentionManager(c kclient.Client, policy time.Duration) *Manager {
	log.Infof("retention policy: %s", policy)

	return &Manager{
		c:      c,
		policy: policy,
	}
}

func (m *Manager) Run(req router.Request, resp router.Response) error {
	thread := req.Object.(*v1.Thread)
	if thread.Spec.SystemTask {
		return nil
	}

	if !thread.Status.LastUsedTime.IsZero() && time.Since(thread.Status.LastUsedTime.Time) > m.policy {
		log.Infof("retention: deleting thread %s/%s", thread.Namespace, thread.Name)
		return m.c.Delete(req.Ctx, thread)
	}

	if since := time.Since(thread.Status.LastUsedTime.Time); m.policy-since < 10*time.Hour {
		resp.RetryAfter(time.Until(thread.Status.LastUsedTime.Time.Add(m.policy)))
	}

	return nil
}
