package auditlogexportcommon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adhocore/gronx"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func FormatLogs[T any, U any](logs []T, convert func(T) U) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

	for _, log := range logs {
		if err := enc.Encode(convert(log)); err != nil {
			return nil, fmt.Errorf("failed to marshal log entry: %w", err)
		}
	}

	return buf.Bytes(), nil
}

func GenerateExportPath(name, keyPrefix, defaultPrefix string) string {
	now := time.Now()
	filename := fmt.Sprintf("%s-%s.jsonl", name, now.Format(time.RFC3339))

	if keyPrefix == "" {
		keyPrefix = fmt.Sprintf("%s/%04d/%02d/%02d", defaultPrefix, now.Year(), now.Month(), now.Day())
	}
	if keyPrefix != "" && !strings.HasSuffix(keyPrefix, "/") {
		keyPrefix += "/"
	}

	return keyPrefix + filename
}

func GetScheduleAndTimezone(schedule v1.Schedule) (string, string) {
	cron := ""
	switch schedule.Interval {
	case "hourly":
		cron = fmt.Sprintf("%d * * * *", schedule.Minute)
	case "daily":
		cron = fmt.Sprintf("%d %d * * *", schedule.Minute, schedule.Hour)
	case "weekly":
		cron = fmt.Sprintf("%d %d * * %d", schedule.Minute, schedule.Hour, schedule.Weekday)
	case "monthly":
		if schedule.Day < 0 {
			// The day being -1 means the last day of the month. The cron parsing package we use uses `L` for this.
			cron = fmt.Sprintf("%d %d L * *", schedule.Minute, schedule.Hour)
		} else if schedule.Day == 0 {
			cron = fmt.Sprintf("%d %d 1 * *", schedule.Minute, schedule.Hour)
		} else {
			cron = fmt.Sprintf("%d %d %d * *", schedule.Minute, schedule.Hour, schedule.Day)
		}
	}

	return cron, schedule.TimeZone
}

func CalculateNextRunTime(schedule v1.Schedule, lastRun *metav1.Time, createdAt metav1.Time) (time.Time, error) {
	if lastRun == nil || lastRun.IsZero() {
		lastRun = &metav1.Time{Time: createdAt.Time}
	}

	cron, timezone := GetScheduleAndTimezone(schedule)
	var location *time.Location
	if timezone != "" {
		loc, err := time.LoadLocation(timezone)
		if err == nil {
			location = loc
		}
	}
	if location != nil {
		lastRun = &metav1.Time{Time: lastRun.In(location)}
	}

	next, err := gronx.NextTickAfter(cron, lastRun.Time, false)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse schedule: %w", err)
	}

	return next, nil
}
