package llmauditlogexport

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatLogsWritesJSONLines(t *testing.T) {
	data, err := formatLogs([]gatewaytypes.LLMAuditLog{
		{
			ID:             "log-1",
			CreatedAt:      time.Date(2026, 7, 2, 1, 2, 3, 0, time.UTC),
			UserID:         "user-1",
			TargetModel:    "gpt-4o",
			ResponseStatus: 200,
			Outcome:        gatewaytypes.LLMAuditOutcomeSuccess,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	line := string(data)
	if !strings.HasSuffix(line, "\n") {
		t.Fatalf("expected trailing newline, got %q", line)
	}
	for _, want := range []string{`"id":"log-1"`, `"userID":"user-1"`, `"targetModel":"gpt-4o"`, `"responseStatus":200`} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected %q in %s", want, line)
		}
	}
}

func TestLLMAuditLogOptionsFromExport(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	export := &v1.LLMAuditLogExport{Spec: v1.LLMAuditLogExportSpec{
		StartTime:           metav1.NewTime(start),
		EndTime:             metav1.NewTime(end),
		WithSensitiveFields: true,
		Filters: types.LLMAuditLogExportFilters{
			UserIDs:          []string{"user-1"},
			ModelProviders:   []string{"openai"},
			TargetModels:     []string{"gpt-4o"},
			RequestPaths:     []string{"/v1/chat/completions"},
			ResponseStatuses: []int{200, 429},
			Outcomes:         []string{gatewaytypes.LLMAuditOutcomeSuccess},
			Clients:          []string{"obot"},
			ClientSessionIDs: []string{"session-1"},
			Query:            "needle",
		},
	}}

	got := llmAuditLogOptionsFromExport(export, 100, 200)
	if !got.StartTime.Equal(start) || !got.EndTime.Equal(end) || got.Limit != 100 || got.Offset != 200 || !got.WithSensitiveFields {
		t.Fatalf("unexpected scalar options: %#v", got)
	}
	if got.SortBy != "created_at" || got.SortOrder != "asc" {
		t.Fatalf("unexpected sort: %#v", got)
	}
	if !reflect.DeepEqual(got.UserID, export.Spec.Filters.UserIDs) || !reflect.DeepEqual(got.ResponseStatus, export.Spec.Filters.ResponseStatuses) || got.Query != "needle" {
		t.Fatalf("filters were not mapped: %#v", got)
	}
}
