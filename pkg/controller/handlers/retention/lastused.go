package retention

import (
	"context"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SetLastUsedTime sets the last used time for a thread and its parent thread if it has one.
// It does not call Update() on the thread, so the caller is responsible for calling Update() if needed.
func SetLastUsedTime(ctx context.Context, c kclient.Client, thread *v1.Thread) error {
	thread.Status.LastUsedTime = metav1.Now()
	if thread.Spec.ParentThreadName != "" {
		return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			var parentThread v1.Thread
			if err := c.Get(ctx, router.Key(thread.Namespace, thread.Spec.ParentThreadName), &parentThread); err != nil {
				return err
			}

			if parentThread.Status.LastUsedTime.IsZero() || parentThread.Status.LastUsedTime.Time.Before(thread.Status.LastUsedTime.Time) {
				parentThread.Status.LastUsedTime = thread.Status.LastUsedTime
				return c.Status().Update(ctx, &parentThread)
			}
			return nil
		})
	}

	return nil
}
