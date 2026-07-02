package handlers

import (
	"net/url"
	"testing"
	"time"
)

func TestParseLLMAuditLogOptsDefaultsStartTimeToLast30Days(t *testing.T) {
	before := time.Now().UTC().AddDate(0, 0, -30).Add(-time.Second)
	opts := parseLLMAuditLogOpts(url.Values{})
	after := time.Now().UTC().AddDate(0, 0, -30).Add(time.Second)

	if opts.StartTime.Before(before) || opts.StartTime.After(after) {
		t.Fatalf("expected start time around 30 days ago, got %s", opts.StartTime)
	}
}

func TestParseLLMAuditLogOptsUsesProvidedStartTime(t *testing.T) {
	want := time.Date(2026, 6, 1, 2, 3, 4, 0, time.UTC)
	opts := parseLLMAuditLogOpts(url.Values{"start_time": {want.Format(time.RFC3339)}})

	if !opts.StartTime.Equal(want) {
		t.Fatalf("expected start time %s, got %s", want, opts.StartTime)
	}
}
