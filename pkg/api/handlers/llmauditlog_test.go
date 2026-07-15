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

func TestParseLLMAuditLogOptsParsesFilterFields(t *testing.T) {
	opts := parseLLMAuditLogOpts(url.Values{
		"target_model":             {"model-a,model-b"},
		"client_session_id":        {"session-1"},
		"message_policy_triggered": {"true,false"},
	})

	if got, want := opts.TargetModel, []string{"model-a", "model-b"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected target models %v, got %v", want, got)
	}
	if got := opts.ClientSessionID; len(got) != 1 || got[0] != "session-1" {
		t.Fatalf("expected client session ID, got %v", got)
	}
	if got := opts.MessagePolicyTriggered; len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("expected input policy trigger values [true false], got %v", got)
	}
}
