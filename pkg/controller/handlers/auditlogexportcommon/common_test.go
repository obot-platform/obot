package auditlogexportcommon

import (
	"strings"
	"testing"
	"time"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatLogsWritesJSONLines(t *testing.T) {
	type logEntry struct {
		ID     string
		Status int
	}

	data, err := FormatLogs([]logEntry{{ID: "log-1", Status: 200}}, func(log logEntry) map[string]any {
		return map[string]any{
			"id":     log.ID,
			"status": log.Status,
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	line := string(data)
	if !strings.HasSuffix(line, "\n") {
		t.Fatalf("expected trailing newline, got %q", line)
	}
	for _, want := range []string{`"id":"log-1"`, `"status":200`} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected %q in %s", want, line)
		}
	}
}

func TestGenerateExportPath(t *testing.T) {
	withDefault := GenerateExportPath("daily", "", "llm-audit-logs")
	if !strings.HasPrefix(withDefault, "llm-audit-logs/") || !strings.HasSuffix(withDefault, ".jsonl") || !strings.Contains(withDefault, "/daily-") {
		t.Fatalf("unexpected default export path: %q", withDefault)
	}

	withPrefix := GenerateExportPath("daily", "custom/prefix", "llm-audit-logs")
	if !strings.HasPrefix(withPrefix, "custom/prefix/daily-") || !strings.HasSuffix(withPrefix, ".jsonl") {
		t.Fatalf("unexpected custom export path: %q", withPrefix)
	}
}

func TestGetScheduleAndTimezone(t *testing.T) {
	tests := []struct {
		name string
		in   v1.Schedule
		want string
	}{
		{name: "hourly", in: v1.Schedule{Interval: "hourly", Minute: 15}, want: "15 * * * *"},
		{name: "daily", in: v1.Schedule{Interval: "daily", Hour: 2, Minute: 30}, want: "30 2 * * *"},
		{name: "weekly", in: v1.Schedule{Interval: "weekly", Hour: 3, Minute: 45, Weekday: 1}, want: "45 3 * * 1"},
		{name: "monthly first day", in: v1.Schedule{Interval: "monthly", Hour: 4, Minute: 5, Day: 0}, want: "5 4 1 * *"},
		{name: "monthly last day", in: v1.Schedule{Interval: "monthly", Hour: 4, Minute: 5, Day: -1}, want: "5 4 L * *"},
		{name: "monthly specific day", in: v1.Schedule{Interval: "monthly", Hour: 4, Minute: 5, Day: 12}, want: "5 4 12 * *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.TimeZone = "UTC"
			got, timezone := GetScheduleAndTimezone(tt.in)
			if got != tt.want || timezone != "UTC" {
				t.Fatalf("expected %q/UTC, got %q/%q", tt.want, got, timezone)
			}
		})
	}
}

func TestCalculateNextRunTimeWithNilLastRunAt(t *testing.T) {
	createdAt := metav1.NewTime(time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC))
	schedule := v1.Schedule{Interval: "hourly", Minute: 30, TimeZone: "UTC"}

	next, err := CalculateNextRunTime(schedule, nil, createdAt)
	if err != nil {
		t.Fatal(err)
	}

	want := time.Date(2026, 7, 1, 10, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected %s, got %s", want, next)
	}
}
