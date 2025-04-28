package cronjob

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCalculateNextRunTime(t *testing.T) {
	t.Run("cronjob with empty LastRunStartedAt", func(t *testing.T) {
		creationTime := time.Date(2025, 4, 26, 9, 0, 0, 0, time.UTC)
		cronJob := v1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: creationTime},
			},
			Status: v1.CronJobStatus{
				LastRunStartedAt: &metav1.Time{},
			},
			Spec: v1.CronJobSpec{
				CronJobManifest: types.CronJobManifest{
					TaskSchedule: &types.Schedule{
						Interval: "daily",
						Hour:     10,
						TimeZone: "America/Phoenix",
					},
				},
			},
		}

		nextRun, err := calculateNextRunTime(cronJob)
		require.NoError(t, err)
		loc, err := time.LoadLocation("America/Phoenix")
		require.NoError(t, err)
		expectedNextRun := creationTime.In(loc).Add(8 * time.Hour)
		require.Equal(t, expectedNextRun, nextRun)
	})

	t.Run("cronjob with timezone specified", func(t *testing.T) {
		// Setup
		creationTimeUTC := time.Date(2025, 4, 26, 9, 0, 0, 0, time.UTC)

		cronJob := v1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: creationTimeUTC},
			},
			Status: v1.CronJobStatus{
				LastRunStartedAt: &metav1.Time{Time: creationTimeUTC},
			},
			Spec: v1.CronJobSpec{
				CronJobManifest: types.CronJobManifest{
					TaskSchedule: &types.Schedule{
						Interval: "daily",
						Hour:     10,
						TimeZone: "America/Phoenix",
					},
				},
			},
		}

		nextRun, err := calculateNextRunTime(cronJob)
		require.NoError(t, err)

		loc, err := time.LoadLocation("America/Phoenix")
		require.NoError(t, err)
		expectedNextRun := cronJob.Status.LastRunStartedAt.In(loc).Add(8 * time.Hour)
		require.Equal(t, expectedNextRun, nextRun)
	})
}
