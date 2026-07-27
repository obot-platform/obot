package auditlogexport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auditlog"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLLMAuditLogOptionsFromExport(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	export := &v1.AuditLogExport{Spec: v1.AuditLogExportSpec{
		Type:                   types.AuditLogTypeLLM,
		StartTime:              metav1.NewTime(start),
		EndTime:                metav1.NewTime(end),
		WithRequestAndResponse: true,
		LLMFilters: &types.LLMAuditLogExportFilters{
			UserIDs:                []string{"user-1"},
			ModelProviders:         []string{"openai"},
			TargetModels:           []string{"gpt-4o"},
			RequestPaths:           []string{"/v1/chat/completions"},
			ResponseStatuses:       []int{200, 429},
			Outcomes:               []string{gatewaytypes.LLMAuditOutcomeSuccess},
			UserAgents:             []string{"obot/1.0"},
			ClientSessionIDs:       []string{"session-1"},
			MessagePolicyTriggered: []bool{true, false},
			Query:                  "needle",
		},
	}}

	got := llmAuditLogOptionsFromExport(export, 100, 200)
	if !got.StartTime.Equal(start) || !got.EndTime.Equal(end) || got.Limit != 100 || got.Offset != 200 || !got.WithSensitiveFields {
		t.Fatalf("unexpected scalar options: %#v", got)
	}
	if got.SortBy != "created_at" || got.SortOrder != "asc" {
		t.Fatalf("unexpected sort: %#v", got)
	}
	if !reflect.DeepEqual(got.UserID, export.Spec.LLMFilters.UserIDs) || !reflect.DeepEqual(got.ResponseStatus, export.Spec.LLMFilters.ResponseStatuses) || got.Query != "needle" {
		t.Fatalf("filters were not mapped: %#v", got)
	}
	if !reflect.DeepEqual(got.MessagePolicyTriggered, export.Spec.LLMFilters.MessagePolicyTriggered) {
		t.Fatalf("message policy filter was not mapped: %#v", got)
	}
	if !reflect.DeepEqual(got.UserAgent, export.Spec.LLMFilters.UserAgents) {
		t.Fatalf("user agent filter was not mapped: %#v", got)
	}
}

func TestAuditLogOptionsFromExportWithoutFilters(t *testing.T) {
	export := &v1.AuditLogExport{}

	mcpOptions := mcpAuditLogOptionsFromExport(export, 100, 200)
	if mcpOptions.Limit != 100 || mcpOptions.Offset != 200 {
		t.Fatalf("unexpected MCP options: %#v", mcpOptions)
	}

	llmOptions := llmAuditLogOptionsFromExport(export, 100, 200)
	if llmOptions.Limit != 100 || llmOptions.Offset != 200 {
		t.Fatalf("unexpected LLM options: %#v", llmOptions)
	}
}

type testStorageProvider struct {
	bucket string
	key    string
	data   string
	err    error
}

func (t *testStorageProvider) Test(context.Context, types.StorageConfig) error {
	return nil
}

func (t *testStorageProvider) Upload(_ context.Context, _ types.StorageConfig, bucket, key string, data io.Reader) error {
	b, err := io.ReadAll(data)
	t.err = err
	if err != nil {
		return err
	}
	t.bucket = bucket
	t.key = key
	t.data = string(b)
	return nil
}

type failingStorageProvider struct {
	err error
}

func (f failingStorageProvider) Test(context.Context, types.StorageConfig) error {
	return nil
}

func (f failingStorageProvider) Upload(context.Context, types.StorageConfig, string, string, io.Reader) error {
	return f.err
}

type failingAfterReadStorageProvider struct {
	data string
	err  error
}

func (f *failingAfterReadStorageProvider) Test(context.Context, types.StorageConfig) error {
	return nil
}

func (f *failingAfterReadStorageProvider) Upload(_ context.Context, _ types.StorageConfig, _, _ string, data io.Reader) error {
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	f.data = string(b)
	return f.err
}

type successWithoutReadStorageProvider struct{}

func (s successWithoutReadStorageProvider) Test(context.Context, types.StorageConfig) error {
	return nil
}

func (s successWithoutReadStorageProvider) Upload(context.Context, types.StorageConfig, string, string, io.Reader) error {
	return nil
}

func TestFormatLogsWritesJSONLines(t *testing.T) {
	type logEntry struct {
		ID     string
		Status int
	}

	data, err := formatLogs([]logEntry{{ID: "log-1", Status: 200}}, func(log logEntry) map[string]any {
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

func TestStreamingExportFetchesFormatsAndUploadsBatches(t *testing.T) {
	storage := &testStorageProvider{}
	var calls []int

	size, err := streamingExport(t.Context(), types.StorageConfig{}, storage, &v1.AuditLogExport{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ *v1.AuditLogExport, limit, offset int) ([]int, error) {
		if limit != batchSize {
			t.Fatalf("expected batch size %d, got %d", batchSize, limit)
		}
		calls = append(calls, offset)
		switch offset {
		case 0:
			return []int{1, 2}, nil
		case 2:
			return []int{3}, nil
		case 3:
			return nil, nil
		default:
			t.Fatalf("unexpected offset %d", offset)
			return nil, nil
		}
	}, func(v int) map[string]int {
		return map[string]int{"value": v}
	})
	if err != nil {
		t.Fatal(err)
	}

	wantData := "{\"value\":1}\n{\"value\":2}\n{\"value\":3}\n"
	if storage.bucket != "bucket" || storage.key != "prefix/export.jsonl" || storage.data != wantData {
		t.Fatalf("unexpected upload: bucket=%q key=%q data=%q", storage.bucket, storage.key, storage.data)
	}
	if size != int64(len(wantData)) {
		t.Fatalf("expected size %d, got %d", len(wantData), size)
	}
	if len(calls) != 3 || calls[0] != 0 || calls[1] != 2 || calls[2] != 3 {
		t.Fatalf("unexpected offsets: %v", calls)
	}
}

func TestStreamingExportClosesPipeWithFetchError(t *testing.T) {
	storage := &testStorageProvider{}
	fetchErr := errors.New("fetch failed")

	_, err := streamingExport(t.Context(), types.StorageConfig{}, storage, &v1.AuditLogExport{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ *v1.AuditLogExport, _ int, offset int) ([]int, error) {
		switch offset {
		case 0:
			return []int{1}, nil
		case 1:
			return nil, fetchErr
		default:
			t.Fatalf("unexpected offset %d", offset)
			return nil, nil
		}
	}, func(v int) map[string]int {
		return map[string]int{"value": v}
	})
	if err == nil || !strings.Contains(err.Error(), "failed to get audit logs batch 1") {
		t.Fatalf("expected fetch error, got %v", err)
	}
	if !errors.Is(storage.err, fetchErr) {
		t.Fatalf("expected upload reader to receive fetch error, got %v", storage.err)
	}
}

func TestStreamingExportClosesPipeWithFormatError(t *testing.T) {
	storage := &testStorageProvider{}

	_, err := streamingExport(t.Context(), types.StorageConfig{}, storage, &v1.AuditLogExport{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ *v1.AuditLogExport, _ int, offset int) ([]int, error) {
		if offset > 0 {
			return nil, nil
		}
		return []int{1}, nil
	}, func(int) any {
		return func() {}
	})
	if err == nil || !strings.Contains(err.Error(), "failed to format logs batch 0") {
		t.Fatalf("expected format error, got %v", err)
	}
	if storage.err == nil || !strings.Contains(storage.err.Error(), "unsupported type: func()") {
		t.Fatalf("expected upload reader to receive format error, got %v", storage.err)
	}
}

func TestStreamingExportUnblocksOnUploadError(t *testing.T) {
	uploadErr := errors.New("upload failed")

	_, err := streamingExport(t.Context(), types.StorageConfig{}, failingStorageProvider{err: uploadErr}, &v1.AuditLogExport{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ *v1.AuditLogExport, _ int, offset int) ([]int, error) {
		if offset > 0 {
			return nil, nil
		}
		return []int{1}, nil
	}, func(v int) map[string]int {
		return map[string]int{"value": v}
	})
	if err == nil || !strings.Contains(err.Error(), "failed to write to pipe") {
		t.Fatalf("expected write error after upload failure, got %v", err)
	}
}

func TestStreamingExportUnblocksWhenUploadReturnsWithoutReading(t *testing.T) {
	_, err := streamingExport(t.Context(), types.StorageConfig{}, successWithoutReadStorageProvider{}, &v1.AuditLogExport{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ *v1.AuditLogExport, _ int, offset int) ([]int, error) {
		if offset > 0 {
			return nil, nil
		}
		return []int{1}, nil
	}, func(v int) map[string]int {
		return map[string]int{"value": v}
	})
	if err == nil || !strings.Contains(err.Error(), "failed to write to pipe") {
		t.Fatalf("expected write error after upload returned early, got %v", err)
	}
}

func TestStreamingExportReturnsUploadErrorAfterDataWritten(t *testing.T) {
	uploadErr := errors.New("upload failed")
	storage := &failingAfterReadStorageProvider{err: uploadErr}

	size, err := streamingExport(t.Context(), types.StorageConfig{}, storage, &v1.AuditLogExport{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ *v1.AuditLogExport, _ int, offset int) ([]int, error) {
		switch offset {
		case 0:
			return []int{1}, nil
		case 1:
			return nil, nil
		default:
			t.Fatalf("unexpected offset %d", offset)
			return nil, nil
		}
	}, func(v int) map[string]int {
		return map[string]int{"value": v}
	})
	if !errors.Is(err, uploadErr) {
		t.Fatalf("expected upload error, got %v", err)
	}
	if storage.data != "{\"value\":1}\n" {
		t.Fatalf("unexpected uploaded data: %q", storage.data)
	}
	if size != int64(len(storage.data)) {
		t.Fatalf("expected size %d, got %d", len(storage.data), size)
	}
}

func TestGenerateExportPath(t *testing.T) {
	withDefault := generateExportPath("daily", "", "llm-audit-logs")
	if !strings.HasPrefix(withDefault, "llm-audit-logs/") || !strings.HasSuffix(withDefault, ".jsonl") || !strings.Contains(withDefault, "/daily-") {
		t.Fatalf("unexpected default export path: %q", withDefault)
	}

	withPrefix := generateExportPath("daily", "custom/prefix", "llm-audit-logs")
	if !strings.HasPrefix(withPrefix, "custom/prefix/daily-") || !strings.HasSuffix(withPrefix, ".jsonl") {
		t.Fatalf("unexpected custom export path: %q", withPrefix)
	}
}

func newExportTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()

	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("failed to create storage services: %v", err)
	}

	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate gateway db: %v", err)
	}

	// Use a short persistence interval so LogMCPAuditEntry rows flush to the DB quickly.
	c := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, 10*time.Millisecond, 10, 90, 90, true)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func exportWithFilters(filters types.AuditLogExportFilters, withPayload bool) *v1.AuditLogExport {
	return &v1.AuditLogExport{
		Spec: v1.AuditLogExportSpec{
			StartTime:              metav1.NewTime(time.Now().Add(-24 * time.Hour)),
			EndTime:                metav1.NewTime(time.Now().Add(24 * time.Hour)),
			Filters:                &filters,
			WithRequestAndResponse: withPayload,
		},
	}
}

func validLocalAgentManifest(occurredAt time.Time, idempotencyKey string) gatewaytypes.LocalAgentToolCallAuditLogFields {
	return gatewaytypes.LocalAgentToolCallAuditLogFields{
		OccurredAt:     occurredAt,
		ActorType:      types.AuditLogActorTypeDevice,
		ActorID:        "device-1",
		ActionName:     "mcp__server__tool",
		ActionKind:     "mcp",
		TargetType:     types.AuditLogTargetTypeMCPTool,
		TargetName:     "tool",
		OutcomeStatus:  types.AuditLogOutcomeStatusSuccess,
		AgentProvider:  types.LocalAgentProviderCodex,
		CLIVersion:     "1.2.3",
		IdempotencyKey: idempotencyKey,
		DeviceID:       "device-1",
		CWD:            "/Users/alice/project",
		RequestBody:    json.RawMessage(`{"arg":true}`),
		ResponseBody:   json.RawMessage(`{"ok":true}`),
		RawEvent:       json.RawMessage(`{"native":true}`),
	}
}

func formatPresentedAuditLogs(logs []gatewaytypes.MCPAuditLog, opts auditlog.PresentOptions) ([]byte, error) {
	return formatLogs(logs, func(log gatewaytypes.MCPAuditLog) types.AuditLogEvent {
		return auditlog.Present(log, opts)
	})
}

// TestAuditLogOptionsDefaultsToMCPOnly proves an export with no SourceTypes filter keeps the
// historical MCP-only default and does not opt into local-agent logs.
func TestAuditLogOptionsDefaultsToMCPOnly(t *testing.T) {
	opts := mcpAuditLogOptionsFromExport(exportWithFilters(types.AuditLogExportFilters{}, false), 100, 0)
	if len(opts.SourceTypes) != 1 || opts.SourceTypes[0] != types.AuditLogSourceTypeMCP {
		t.Fatalf("expected MCP source type, got %v", opts.SourceTypes)
	}
	if opts.WithRequestAndResponse {
		t.Fatal("expected WithRequestAndResponse to be false for non-auditor export")
	}
}

// TestAuditLogOptionsMapsLocalAgentFilters proves local-agent source type and filters are passed
// through to the gateway query when a caller explicitly opts in.
func TestAuditLogOptionsMapsLocalAgentFilters(t *testing.T) {
	opts := mcpAuditLogOptionsFromExport(exportWithFilters(types.AuditLogExportFilters{
		SourceTypes:    []types.AuditLogSourceType{types.AuditLogSourceTypeLocalAgentToolCall},
		AgentProviders: []string{string(types.LocalAgentProviderClaudeCode)},
		Statuses:       []string{string(types.AuditLogOutcomeStatusFailure)},
		ToolNames:      []string{"mcp__server__tool"},
		ToolKinds:      []string{"mcp"},
		DeviceIDs:      []string{"device-1"},
	}, true), 100, 0)

	if len(opts.SourceTypes) != 1 || opts.SourceTypes[0] != types.AuditLogSourceTypeLocalAgentToolCall {
		t.Fatalf("expected local-agent source type, got %v", opts.SourceTypes)
	}
	if len(opts.AgentProvider) != 1 || opts.AgentProvider[0] != string(types.LocalAgentProviderClaudeCode) {
		t.Fatalf("expected agent provider filter to pass through, got %v", opts.AgentProvider)
	}
	if len(opts.Status) != 1 || len(opts.ToolName) != 1 || len(opts.ToolKind) != 1 || len(opts.DeviceID) != 1 {
		t.Fatalf("expected local-agent filters to pass through, got %#v", opts)
	}
	if !opts.WithRequestAndResponse {
		t.Fatal("expected WithRequestAndResponse to be true for auditor export")
	}
}

// TestFormatLogsExportPayloadRequiresAuditor proves that only Auditor-role exports
// (WithRequestAndResponse=true) include decrypted payload fields; otherwise sensitive fields are
// blanked.
func TestFormatLogsExportPayloadRequiresAuditor(t *testing.T) {
	c := newExportTestGatewayClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	local := validLocalAgentManifest(now, "payload-entry")
	if err := c.InsertLocalAgentAuditLogs(ctx, []gatewaytypes.MCPAuditLog{{
		CreatedAt:                now,
		SourceType:               types.AuditLogSourceTypeLocalAgentToolCall,
		UserID:                   "user-1",
		LocalAgentToolCallFields: &local,
	}}); err != nil {
		t.Fatalf("insert local-agent audit log: %v", err)
	}

	assertPayload := func(withPayload bool, wantPayload bool) {
		logs, _, err := c.GetMCPAuditLogs(ctx, mcpAuditLogOptionsFromExport(exportWithFilters(types.AuditLogExportFilters{
			SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeLocalAgentToolCall},
		}, withPayload), 100, 0))
		if err != nil {
			t.Fatalf("get local-agent audit logs (withPayload=%v): %v", withPayload, err)
		}
		data, err := formatPresentedAuditLogs(logs, auditlog.PresentOptions{
			IncludeDetails:  true,
			PayloadRedacted: !withPayload,
		})
		if err != nil {
			t.Fatalf("format logs (withPayload=%v): %v", withPayload, err)
		}
		hasPayload := strings.Contains(string(data), `"arg":true`) &&
			strings.Contains(string(data), `/Users/alice/project`)
		if hasPayload != wantPayload {
			t.Fatalf("withPayload=%v: expected hasPayload=%v, got %v (%s)", withPayload, wantPayload, hasPayload, data)
		}
	}

	assertPayload(false, false)
	assertPayload(true, true)
}

func TestExportAllSourcesIncludesBothMCPAndLocal(t *testing.T) {
	c := newExportTestGatewayClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	c.LogMCPAuditEntry(gatewaytypes.MCPAuditLog{
		CreatedAt:  now,
		SourceType: types.AuditLogSourceTypeMCP,
		UserID:     "user-1",
		MCPFields: &gatewaytypes.MCPAuditLogFields{
			MCPID:    "mcp-1",
			CallType: "tools/call",
		},
	})
	waitForMCPAuditLog(t, c)

	local := validLocalAgentManifest(now, "local-entry")
	if err := c.InsertLocalAgentAuditLogs(ctx, []gatewaytypes.MCPAuditLog{{
		CreatedAt:                now,
		SourceType:               types.AuditLogSourceTypeLocalAgentToolCall,
		UserID:                   "user-1",
		LocalAgentToolCallFields: &local,
	}}); err != nil {
		t.Fatalf("insert local-agent audit log: %v", err)
	}

	export := exportWithFilters(types.AuditLogExportFilters{SourceTypes: []types.AuditLogSourceType{
		types.AuditLogSourceTypeMCP,
		types.AuditLogSourceTypeLocalAgentToolCall,
	}}, true)

	var (
		mcpRows   int
		localRows int
	)
	logs, _, err := c.GetMCPAuditLogs(ctx, mcpAuditLogOptionsFromExport(export, 100, 0))
	if err != nil {
		t.Fatalf("get mixed audit logs: %v", err)
	}
	data, err := formatPresentedAuditLogs(logs, auditlog.PresentOptions{IncludeDetails: true})
	if err != nil {
		t.Fatalf("format mixed logs: %v", err)
	}
	if strings.Contains(string(data), "mcpFields") || strings.Contains(string(data), "localAgentToolCallFields") {
		t.Fatalf("legacy keys found in normalized export: %s", data)
	}
	for _, line := range splitNonEmptyLines(string(data)) {
		var row types.AuditLogEvent
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("unmarshal exported line: %v", err)
		}
		switch row.EventType {
		case types.AuditLogEventTypeMCPCall:
			mcpRows++
		case types.AuditLogEventTypeLocalAgentToolCall:
			localRows++
		}
	}

	if mcpRows != 1 || localRows != 1 {
		t.Fatalf("expected 1 MCP row and 1 local row in an all-sources export, got mcp=%d local=%d", mcpRows, localRows)
	}
}

func splitNonEmptyLines(s string) []string {
	var out []string
	for line := range strings.SplitSeq(strings.TrimSpace(s), "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func waitForMCPAuditLog(t *testing.T, c *gatewayclient.Client) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		logs, total, err := c.GetMCPAuditLogs(t.Context(), gatewayclient.MCPAuditLogOptions{
			SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeMCP},
			Limit:       1,
		})
		if err != nil {
			t.Fatalf("list MCP audit logs: %v", err)
		}
		if total > 0 && len(logs) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for MCP audit log to persist")
}
