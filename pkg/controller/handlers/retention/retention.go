package retention

import (
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Manager struct {
	c      kclient.Client
	policy time.Duration
}

func NewRetentionManager(c kclient.Client, policy time.Duration) *Manager {
	if policy == 0 {
		log.Infof("retention policy: disabled")
	} else {
		log.Infof("retention policy: %s", policy)
	}

	return &Manager{
		c:      c,
		policy: policy,
	}
}

func (m *Manager) Run(req router.Request, resp router.Response) error {
	if m.policy == 0 {
		return nil
	}

	thread := req.Object.(*v1.Thread)
	if thread.Spec.SystemTask {
		return nil
	}

	if thread.Spec.Project {
		// If this thread is a project, there is a chance it is a featured Obot.
		// We do not want to clean up featured Obots. Check the thread shares to see if it is one.
		shares := &v1.ThreadShareList{}
		if err := m.c.List(req.Ctx, shares, kclient.InNamespace(thread.Namespace), &kclient.ListOptions{
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.featured":          "true",
				"spec.projectThreadName": thread.Name,
			}),
		}); err != nil {
			return err
		}

		if len(shares.Items) > 0 {
			log.Infof("retention: skipping thread %s because it is a featured Obot", thread.Name)
			return nil
		}
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
