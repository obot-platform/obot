package auditlogexportcommon

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

	size, err := streamingExport(t.Context(), types.StorageConfig{}, storage, struct{}{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ struct{}, limit, offset int) ([]int, error) {
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

	_, err := streamingExport(t.Context(), types.StorageConfig{}, storage, struct{}{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ struct{}, _ int, offset int) ([]int, error) {
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

	_, err := streamingExport(t.Context(), types.StorageConfig{}, storage, struct{}{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ struct{}, _ int, offset int) ([]int, error) {
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

	_, err := streamingExport(t.Context(), types.StorageConfig{}, failingStorageProvider{err: uploadErr}, struct{}{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ struct{}, _ int, offset int) ([]int, error) {
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
	_, err := streamingExport(t.Context(), types.StorageConfig{}, successWithoutReadStorageProvider{}, struct{}{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ struct{}, _ int, offset int) ([]int, error) {
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

	size, err := streamingExport(t.Context(), types.StorageConfig{}, storage, struct{}{}, "bucket", "prefix/export.jsonl", func(_ context.Context, _ struct{}, _ int, offset int) ([]int, error) {
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
