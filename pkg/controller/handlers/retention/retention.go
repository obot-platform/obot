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

func RunRetention(policy time.Duration) func(req router.Request, resp router.Response) error {
	if policy == 0 {
		log.Infof("retention policy: disabled")
	} else {
		log.Infof("retention policy: %s", policy)
	}

	return func(req router.Request, resp router.Response) error {
		if policy == 0 {
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
			if err := req.Client.List(req.Ctx, shares, kclient.InNamespace(thread.Namespace), &kclient.ListOptions{
				FieldSelector: fields.SelectorFromSet(map[string]string{
					"spec.projectThreadName": thread.Name,
				}),
			}); err != nil {
				return err
			}

			for _, share := range shares.Items {
				if share.Spec.Featured {
					log.Infof("retention: skipping thread %s because it is a featured Obot", thread.Name)
					return nil
				}
			}
		}

		if !thread.Status.LastUsedTime.IsZero() && time.Since(thread.Status.LastUsedTime.Time) > policy {
			log.Infof("retention: deleting thread %s/%s", thread.Namespace, thread.Name)
			return req.Client.Delete(req.Ctx, thread)
		}

		if since := time.Since(thread.Status.LastUsedTime.Time); policy-since < 10*time.Hour {
			resp.RetryAfter(time.Until(thread.Status.LastUsedTime.Time.Add(policy)))
		}

		return nil
	}
}
