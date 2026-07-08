package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateLLMExportRequest(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		req     types.LLMAuditLogExportCreateRequest
		wantErr string
	}{
		{
			name: "valid",
			req: types.LLMAuditLogExportCreateRequest{
				Name:      "export",
				Bucket:    "bucket",
				StartTime: types.Time{Time: now},
				EndTime:   types.Time{Time: now.Add(time.Hour)},
			},
		},
		{
			name:    "missing name",
			req:     types.LLMAuditLogExportCreateRequest{},
			wantErr: "name is required",
		},
		{
			name: "missing bucket",
			req: types.LLMAuditLogExportCreateRequest{
				Name:      "export",
				StartTime: types.Time{Time: now},
				EndTime:   types.Time{Time: now.Add(time.Hour)},
			},
			wantErr: "bucket is required",
		},
		{
			name: "inverted time range",
			req: types.LLMAuditLogExportCreateRequest{
				Name:      "export",
				Bucket:    "bucket",
				StartTime: types.Time{Time: now.Add(time.Hour)},
				EndTime:   types.Time{Time: now},
			},
			wantErr: "start time must be before end time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLLMExportRequest(&tt.req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid request: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
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
		Status: v1.AuditLogExportStatus{
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
