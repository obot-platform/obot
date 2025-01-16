package knowledgesource

import (
	"fmt"
	"time"

	"github.com/adhocore/gronx"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (k *Handler) Reschedule(req router.Request, _ router.Response) error {
	source := req.Object.(*v1.KnowledgeSource)
	if source.Spec.Manifest.SyncSchedule == "" {
		// No schedule defined, nothing to do
		return nil
	}

	if source.Status.LastSyncEndTime.IsZero() {
		// No sync has been performed yet or is still in progress
		return nil
	}

	if source.Status.LastSyncStartTime.IsZero() {
		// No sync has been performed yet
		return nil
	}

	if source.Status.NextSyncTime.IsZero() {
		if !gronx.IsValid(source.Spec.Manifest.SyncSchedule) {
			source.Status.Error = fmt.Sprintf("invalid sync schedule: %s", source.Spec.Manifest.SyncSchedule)
			source.Status.SyncState = types.KnowledgeSourceStateError
			return nil
		}

		tick, err := gronx.NextTickAfter(source.Spec.Manifest.SyncSchedule, source.Status.LastSyncStartTime.Time, false)
		if err != nil {
			source.Status.Error = fmt.Sprintf("failed to calculate next sync time: %v", err)
			source.Status.SyncState = types.KnowledgeSourceStateError
			return nil
		}

		source.Status.NextSyncTime = metav1.NewTime(tick)
	} else if source.Status.NextSyncTime.Time.Before(time.Now()) {
		source.Status.NextSyncTime = metav1.Time{}
		source.Status.SyncState = types.KnowledgeSourceStatePending
	}

	return req.Client.Status().Update(req.Ctx, source)
}
