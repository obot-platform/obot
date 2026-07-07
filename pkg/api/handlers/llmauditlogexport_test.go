package handlers

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateLLMExportRequest(t *testing.T) {
	now := time.Now()

	if err := validateLLMExportRequest(&types.LLMAuditLogExportCreateRequest{Name: "export", StartTime: types.Time{Time: now}, EndTime: types.Time{Time: now.Add(time.Hour)}}); err != nil {
		t.Fatalf("expected valid request: %v", err)
	}
	if err := validateLLMExportRequest(&types.LLMAuditLogExportCreateRequest{}); err == nil {
		t.Fatal("expected missing name to fail")
	}
	if err := validateLLMExportRequest(&types.LLMAuditLogExportCreateRequest{Name: "export", StartTime: types.Time{Time: now.Add(time.Hour)}, EndTime: types.Time{Time: now}}); err == nil {
		t.Fatal("expected inverted time range to fail")
	}
}

func TestConvertLLMExportToAPI(t *testing.T) {
	started := metav1.NewTime(time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC))
	completed := metav1.NewTime(time.Date(2026, 7, 1, 1, 5, 0, 0, time.UTC))
	export := &v1.LLMAuditLogExport{
		ObjectMeta: metav1.ObjectMeta{Name: "lael1abc", CreationTimestamp: metav1.NewTime(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))},
		Spec: v1.LLMAuditLogExportSpec{
			Name:      "daily",
			Bucket:    "bucket",
			KeyPrefix: "prefix",
			StartTime: metav1.NewTime(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)),
			EndTime:   metav1.NewTime(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)),
			Filters:   types.LLMAuditLogExportFilters{UserIDs: []string{"user-1"}},
		},
		Status: v1.LLMAuditLogExportStatus{
			State:           types.AuditLogExportStateCompleted,
			ExportSize:      123,
			ExportPath:      "prefix/daily.jsonl",
			StartedAt:       &started,
			CompletedAt:     &completed,
			StorageProvider: types.StorageProviderS3,
		},
	}

	got := convertLLMExportToAPI(export)
	if got.ID != export.Name || got.Name != export.Spec.Name || got.Bucket != "bucket" || got.ExportSize != 123 {
		t.Fatalf("unexpected response: %#v", got)
	}
	if got.Filters.UserIDs[0] != "user-1" || got.State != string(types.AuditLogExportStateCompleted) || got.StorageProvider != types.StorageProviderS3 {
		t.Fatalf("unexpected status/filter fields: %#v", got)
	}
	if !got.StartedAt.Time.Equal(started.Time) || !got.CompletedAt.Time.Equal(completed.Time) {
		t.Fatalf("unexpected timestamps: started=%s completed=%s", got.StartedAt, got.CompletedAt)
	}
}
