package scheduledauditlogexport

import (
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateExportFromSchedule(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	next := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	scheduled := &v1.ScheduledAuditLogExport{
		ObjectMeta: metav1.ObjectMeta{Name: "scheduled", Namespace: "default"},
		Spec: v1.ScheduledAuditLogExportSpec{
			Name:                   "weekly",
			Type:                   types.AuditLogTypeLLM,
			Bucket:                 "bucket",
			KeyPrefix:              "prefix",
			RetentionPeriodInDays:  7,
			WithRequestAndResponse: true,
			LLMFilters:             types.LLMAuditLogExportFilters{ModelProviders: []string{"openai"}},
		},
		Status: v1.ScheduledAuditLogExportStatus{TotalExportsCreated: 2},
	}

	if err := (&Handler{}).createExportFromSchedule(router.Request{Ctx: t.Context(), Client: c}, scheduled, next); err != nil {
		t.Fatal(err)
	}

	var exports v1.AuditLogExportList
	if err := c.List(t.Context(), &exports, client.InNamespace("default")); err != nil {
		t.Fatal(err)
	}
	if len(exports.Items) != 1 {
		t.Fatalf("expected one export, got %d", len(exports.Items))
	}
	got := exports.Items[0]
	if got.Spec.Name != "weekly-3" || got.Spec.Bucket != "bucket" || got.Spec.KeyPrefix != "prefix" {
		t.Fatalf("unexpected export spec: %#v", got.Spec)
	}
	if !got.Spec.StartTime.Time.Equal(next.Add(-7*24*time.Hour)) || !got.Spec.EndTime.Time.Equal(next) {
		t.Fatalf("unexpected export range: %s - %s", got.Spec.StartTime.Time, got.Spec.EndTime.Time)
	}
	if got.Spec.Type != types.AuditLogTypeLLM || !got.Spec.WithRequestAndResponse || got.Spec.LLMFilters.ModelProviders[0] != "openai" || scheduled.Status.TotalExportsCreated != 3 {
		t.Fatalf("unexpected sensitive/filter/count fields: export=%#v scheduled=%#v", got.Spec, scheduled.Status)
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
			got, timezone := getScheduleAndTimezone(tt.in)
			if got != tt.want || timezone != "UTC" {
				t.Fatalf("expected %q/UTC, got %q/%q", tt.want, got, timezone)
			}
		})
	}
}

func TestCalculateNextRunTimeWithNilLastRunAt(t *testing.T) {
	scheduledExport := &v1.ScheduledAuditLogExport{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC))},
		Spec:       v1.ScheduledAuditLogExportSpec{Schedule: v1.Schedule{Interval: "hourly", Minute: 30, TimeZone: "UTC"}},
	}

	next, err := calculateNextRunTime(scheduledExport)
	if err != nil {
		t.Fatal(err)
	}

	want := time.Date(2026, 7, 1, 10, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected %s, got %s", want, next)
	}
}
