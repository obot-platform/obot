package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateAuditLogExportRequest(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		req      types.AuditLogExportCreateRequest
		wantErr  string
		wantType types.AuditLogType
	}{
		{
			name: "valid LLM",
			req: types.AuditLogExportCreateRequest{
				Name:      "export",
				Type:      types.AuditLogTypeLLM,
				Bucket:    "bucket",
				StartTime: types.Time{Time: now},
				EndTime:   types.Time{Time: now.Add(time.Hour)},
			},
			wantType: types.AuditLogTypeLLM,
		},
		{
			name: "missing type defaults to MCP",
			req: types.AuditLogExportCreateRequest{
				Name:      "export",
				StartTime: types.Time{Time: now},
				EndTime:   types.Time{Time: now.Add(time.Hour)},
			},
			wantType: types.AuditLogTypeMCP,
		},
		{
			name:    "missing name",
			req:     types.AuditLogExportCreateRequest{Type: types.AuditLogTypeLLM},
			wantErr: "name is required",
		},
		{
			name: "missing bucket",
			req: types.AuditLogExportCreateRequest{
				Name:      "export",
				Type:      types.AuditLogTypeLLM,
				StartTime: types.Time{Time: now},
				EndTime:   types.Time{Time: now.Add(time.Hour)},
			},
			wantErr: "bucket is required",
		},
		{
			name: "inverted time range",
			req: types.AuditLogExportCreateRequest{
				Name:      "export",
				Type:      types.AuditLogTypeLLM,
				Bucket:    "bucket",
				StartTime: types.Time{Time: now.Add(time.Hour)},
				EndTime:   types.Time{Time: now},
			},
			wantErr: "start time must be before end time",
		},
		{
			name: "invalid type",
			req: types.AuditLogExportCreateRequest{
				Name:      "export",
				Type:      "other",
				StartTime: types.Time{Time: now},
				EndTime:   types.Time{Time: now.Add(time.Hour)},
			},
			wantErr: "must be",
		},
		{
			name: "LLM rejects MCP filters",
			req: types.AuditLogExportCreateRequest{
				Name:      "export",
				Type:      types.AuditLogTypeLLM,
				Bucket:    "bucket",
				StartTime: types.Time{Time: now},
				EndTime:   types.Time{Time: now.Add(time.Hour)},
				Filters:   types.AuditLogExportFilters{UserIDs: []string{"user-1"}},
			},
			wantErr: "filters can only be set for MCP",
		},
		{
			name: "MCP rejects LLM filters",
			req: types.AuditLogExportCreateRequest{
				Name:       "export",
				StartTime:  types.Time{Time: now},
				EndTime:    types.Time{Time: now.Add(time.Hour)},
				LLMFilters: types.LLMAuditLogExportFilters{UserIDs: []string{"user-1"}},
			},
			wantErr: "llmFilters can only be set for LLM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&AuditLogExportHandler{}).validateExportRequest(&tt.req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid request: %v", err)
				}
				if tt.req.Type != tt.wantType {
					t.Fatalf("expected normalized type %q, got %q", tt.wantType, tt.req.Type)
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
	export := &v1.AuditLogExport{
		ObjectMeta: metav1.ObjectMeta{Name: "ael1abc", CreationTimestamp: metav1.NewTime(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))},
		Spec: v1.AuditLogExportSpec{
			Name:       "daily",
			Type:       types.AuditLogTypeLLM,
			Bucket:     "bucket",
			KeyPrefix:  "prefix",
			StartTime:  metav1.NewTime(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)),
			EndTime:    metav1.NewTime(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)),
			LLMFilters: types.LLMAuditLogExportFilters{UserIDs: []string{"user-1"}},
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

	got := (&AuditLogExportHandler{}).convertExportToAPI(export)
	if got.ID != export.Name || got.Name != export.Spec.Name || got.Bucket != "bucket" || got.ExportSize != 123 {
		t.Fatalf("unexpected response: %#v", got)
	}
	if got.Type != types.AuditLogTypeLLM || got.LLMFilters.UserIDs[0] != "user-1" || got.State != string(types.AuditLogExportStateCompleted) || got.StorageProvider != types.StorageProviderS3 {
		t.Fatalf("unexpected status/filter fields: %#v", got)
	}
	if !got.StartedAt.Time.Equal(started.Time) || !got.CompletedAt.Time.Equal(completed.Time) {
		t.Fatalf("unexpected timestamps: started=%s completed=%s", got.StartedAt, got.CompletedAt)
	}
}
