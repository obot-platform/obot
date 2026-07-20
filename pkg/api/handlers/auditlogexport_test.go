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
			name: "MCP requires source types",
			req: types.AuditLogExportCreateRequest{
				Name:      "export",
				Bucket:    "bucket",
				StartTime: types.Time{Time: now},
				EndTime:   types.Time{Time: now.Add(time.Hour)},
			},
			wantErr: "sourceTypes must include at least one",
		},
		{
			name:    "missing name",
			req:     types.AuditLogExportCreateRequest{Type: types.AuditLogTypeLLM},
			wantErr: "name is required",
		},
		{
			name: "LLM missing bucket",
			req: types.AuditLogExportCreateRequest{
				Name:      "export",
				Type:      types.AuditLogTypeLLM,
				StartTime: types.Time{Time: now},
				EndTime:   types.Time{Time: now.Add(time.Hour)},
			},
			wantErr: "bucket is required",
		},
		{
			name: "MCP missing bucket",
			req: types.AuditLogExportCreateRequest{
				Name:      "export",
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
				Bucket:    "bucket",
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
				Filters:   &types.AuditLogExportFilters{UserIDs: []string{"user-1"}},
			},
			wantErr: "filters can only be set for MCP",
		},
		{
			name: "MCP rejects LLM filters",
			req: types.AuditLogExportCreateRequest{
				Name:       "export",
				Bucket:     "bucket",
				StartTime:  types.Time{Time: now},
				EndTime:    types.Time{Time: now.Add(time.Hour)},
				LLMFilters: &types.LLMAuditLogExportFilters{UserIDs: []string{"user-1"}},
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
			LLMFilters: &types.LLMAuditLogExportFilters{UserIDs: []string{"user-1"}},
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
	if got.Type != types.AuditLogTypeLLM || got.LLMFilters == nil || got.LLMFilters.UserIDs[0] != "user-1" || got.State != string(types.AuditLogExportStateCompleted) || got.StorageProvider != types.StorageProviderS3 {
		t.Fatalf("unexpected status/filter fields: %#v", got)
	}
	if !got.StartedAt.Time.Equal(started.Time) || !got.CompletedAt.Time.Equal(completed.Time) {
		t.Fatalf("unexpected timestamps: started=%s completed=%s", got.StartedAt, got.CompletedAt)
	}
}

func TestValidateAuditLogExportFilters(t *testing.T) {
	tests := []struct {
		name    string
		filters types.AuditLogExportFilters
		wantErr bool
	}{
		{
			name:    "invalid source",
			filters: types.AuditLogExportFilters{SourceTypes: []types.AuditLogSourceType{"future"}},
			wantErr: true,
		},
		{
			name: "MCP filter without MCP source",
			filters: types.AuditLogExportFilters{
				SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeLocalAgentToolCall},
				MCPIDs:      []string{"mcp-1"},
			},
			wantErr: true,
		},
		{
			name: "local filter without local source",
			filters: types.AuditLogExportFilters{
				SourceTypes:    []types.AuditLogSourceType{types.AuditLogSourceTypeMCP},
				AgentProviders: []string{"codex"},
			},
			wantErr: true,
		},
		{
			name: "mixed source-specific groups",
			filters: types.AuditLogExportFilters{
				SourceTypes:    []types.AuditLogSourceType{types.AuditLogSourceTypeMCP, types.AuditLogSourceTypeLocalAgentToolCall},
				MCPIDs:         []string{"mcp-1"},
				AgentProviders: []string{"codex"},
			},
			wantErr: true,
		},
		{
			name: "multi-source rejects shared-column filters",
			filters: types.AuditLogExportFilters{
				SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeMCP, types.AuditLogSourceTypeLocalAgentToolCall},
				UserIDs:     []string{"user-1"},
			},
			wantErr: true,
		},
		{
			name: "common filter requires multiple sources",
			filters: types.AuditLogExportFilters{
				SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeMCP},
				Actors:      []string{"user-1"},
			},
			wantErr: true,
		},
		{
			name: "common filters cannot combine with source-specific",
			filters: types.AuditLogExportFilters{
				SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeMCP, types.AuditLogSourceTypeLocalAgentToolCall},
				Actors:      []string{"user-1"},
				MCPIDs:      []string{"mcp-1"},
			},
			wantErr: true,
		},
		{
			name: "valid multi-source common filters",
			filters: types.AuditLogExportFilters{
				SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeMCP, types.AuditLogSourceTypeLocalAgentToolCall},
				Actors:      []string{"user-1"},
				Tools:       []string{"Bash"},
				Outcomes:    []string{"denied"},
			},
		},
		{
			name: "valid single-source shared-column filters",
			filters: types.AuditLogExportFilters{
				SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeMCP},
				UserIDs:     []string{"user-1"},
				SessionIDs:  []string{"session-1"},
			},
		},
		{
			name: "valid local-agent filters",
			filters: types.AuditLogExportFilters{
				SourceTypes:    []types.AuditLogSourceType{types.AuditLogSourceTypeLocalAgentToolCall},
				AgentProviders: []string{"codex"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAuditLogExportFilters(&test.filters)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateAuditLogExportFiltersRequiresSourceTypes(t *testing.T) {
	for _, filters := range []*types.AuditLogExportFilters{
		nil,
		{},
		{SourceTypes: []types.AuditLogSourceType{}},
	} {
		if err := validateAuditLogExportFilters(filters); err == nil {
			t.Fatalf("expected empty sourceTypes to be rejected for filters %#v", filters)
		}
	}
}

func TestValidateScheduledMCPAuditLogExportRequiresSourceTypes(t *testing.T) {
	req := types.ScheduledAuditLogExportCreateRequest{
		Name:   "schedule",
		Bucket: "bucket",
		Schedule: types.Schedule{
			Interval: "daily",
		},
	}
	if err := (&AuditLogExportHandler{}).validateScheduledExportRequest(&req); err == nil {
		t.Fatal("expected an MCP schedule without sourceTypes to be rejected")
	}

	req.Filters = &types.AuditLogExportFilters{SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeMCP}}
	if err := (&AuditLogExportHandler{}).validateScheduledExportRequest(&req); err != nil {
		t.Fatalf("expected an MCP schedule with sourceTypes to be valid: %v", err)
	}
}
