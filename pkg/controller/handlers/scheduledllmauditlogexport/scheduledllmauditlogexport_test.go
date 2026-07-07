package scheduledllmauditlogexport

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
	scheduled := &v1.ScheduledLLMAuditLogExport{
		ObjectMeta: metav1.ObjectMeta{Name: "scheduled", Namespace: "default"},
		Spec: v1.ScheduledLLMAuditLogExportSpec{
			Name:                  "weekly",
			Bucket:                "bucket",
			KeyPrefix:             "prefix",
			RetentionPeriodInDays: 7,
			WithSensitiveFields:   true,
			Filters:               types.LLMAuditLogExportFilters{ModelProviders: []string{"openai"}},
		},
		Status: v1.ScheduledLLMAuditLogExportStatus{TotalExportsCreated: 2},
	}

	if err := (&Handler{}).createExportFromSchedule(router.Request{Ctx: t.Context(), Client: c}, scheduled, next); err != nil {
		t.Fatal(err)
	}

	var exports v1.LLMAuditLogExportList
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
	if !got.Spec.WithSensitiveFields || got.Spec.Filters.ModelProviders[0] != "openai" || scheduled.Status.TotalExportsCreated != 3 {
		t.Fatalf("unexpected sensitive/filter/count fields: export=%#v scheduled=%#v", got.Spec, scheduled.Status)
	}
}

func TestCalculateNextRunTimeWithNilLastRunAt(t *testing.T) {
	scheduled := &v1.ScheduledLLMAuditLogExport{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.NewTime(time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)),
		},
		Spec: v1.ScheduledLLMAuditLogExportSpec{
			Schedule: v1.Schedule{Interval: "hourly", Minute: 30, TimeZone: "UTC"},
		},
	}

	if _, err := calculateNextRunTime(scheduled); err != nil {
		t.Fatal(err)
	}
}
