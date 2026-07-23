package client

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func sampleEnforcementDecision(decision, agent string) types.EnforcementDecisionLog {
	return types.EnforcementDecisionLog{
		CreatedAt:            time.Now().UTC(),
		MDMConfigurationID:   7,
		DeviceID:             "device-1",
		ClientIP:             "203.0.113.10",
		Agent:                agent,
		Tool:                 "search",
		Kind:                 "mcp",
		ServerName:           "docs",
		Decision:             decision,
		Reason:               "test reason",
		ServerURL:            "https://internal.example.com/docs",
		ServerHostname:       "internal.example.com",
		ServerPackageSource:  "npm",
		ServerPackageName:    "@acme/docs",
		ServerPackageVersion: "1.2.3",
	}
}

func TestEnforcementDecisionBufferedInsertReturnsServerIdentity(t *testing.T) {
	c := newTestClient(t)

	c.LogEnforcementDecision(sampleEnforcementDecision(types.EnforcementDecisionDeny, "claude_code"))
	if err := c.persistEnforcementDecisions(); err != nil {
		t.Fatalf("persist enforcement decisions: %v", err)
	}

	// The list view returns the full row, including the resolved server identity.
	logs, total, err := c.GetEnforcementDecisions(t.Context(), EnforcementDecisionOptions{})
	if err != nil {
		t.Fatalf("list enforcement decisions: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected one row, got total=%d len=%d", total, len(logs))
	}
	if logs[0].ServerURL != "https://internal.example.com/docs" ||
		logs[0].ServerHostname != "internal.example.com" ||
		logs[0].ServerPackageName != "@acme/docs" {
		t.Fatalf("expected list row to include the server identity, got %#v", logs[0])
	}
	if logs[0].Decision != types.EnforcementDecisionDeny {
		t.Fatalf("list decision = %q, want deny", logs[0].Decision)
	}

	// The detail view returns the same server identity.
	detail, err := c.GetEnforcementDecision(t.Context(), logs[0].ID)
	if err != nil {
		t.Fatalf("get enforcement decision detail: %v", err)
	}
	if detail.ServerURL != "https://internal.example.com/docs" {
		t.Fatalf("detail server URL = %q", detail.ServerURL)
	}
	if detail.ServerHostname != "internal.example.com" || detail.ServerPackageName != "@acme/docs" || detail.ServerPackageVersion != "1.2.3" {
		t.Fatalf("unexpected detail server identity: %#v", detail)
	}
}

func TestEnforcementDecisionQuerySearchesServerIdentity(t *testing.T) {
	c := newTestClient(t)

	c.LogEnforcementDecision(sampleEnforcementDecision(types.EnforcementDecisionDeny, "claude_code"))
	if err := c.persistEnforcementDecisions(); err != nil {
		t.Fatalf("persist enforcement decisions: %v", err)
	}

	logs, total, err := c.GetEnforcementDecisions(t.Context(), EnforcementDecisionOptions{
		Query: "internal.example.com",
	})
	if err != nil {
		t.Fatalf("query list: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected query to match the server hostname, got total=%d len=%d", total, len(logs))
	}
}

func TestEnforcementDecisionFilteredRead(t *testing.T) {
	c := newTestClient(t)

	c.LogEnforcementDecision(sampleEnforcementDecision(types.EnforcementDecisionAllow, "codex"))
	c.LogEnforcementDecision(sampleEnforcementDecision(types.EnforcementDecisionDeny, "claude_code"))
	if err := c.persistEnforcementDecisions(); err != nil {
		t.Fatalf("persist enforcement decisions: %v", err)
	}

	// Filter by decision.
	denies, total, err := c.GetEnforcementDecisions(t.Context(), EnforcementDecisionOptions{
		Decision: []string{types.EnforcementDecisionDeny},
	})
	if err != nil {
		t.Fatalf("filtered list: %v", err)
	}
	if total != 1 || len(denies) != 1 || denies[0].Agent != "claude_code" {
		t.Fatalf("expected one deny row for claude_code, got total=%d %#v", total, denies)
	}

	// Filter by agent.
	byAgent, total, err := c.GetEnforcementDecisions(t.Context(), EnforcementDecisionOptions{
		Agent: []string{"codex"},
	})
	if err != nil {
		t.Fatalf("filter by agent: %v", err)
	}
	if total != 1 || len(byAgent) != 1 || byAgent[0].Decision != types.EnforcementDecisionAllow {
		t.Fatalf("expected one allow row for codex, got total=%d %#v", total, byAgent)
	}

	// Filter options for the agent dimension include both agents.
	options, err := c.GetEnforcementDecisionFilterOptions(t.Context(), "agent", EnforcementDecisionOptions{})
	if err != nil {
		t.Fatalf("filter options: %v", err)
	}
	if len(options) != 2 {
		t.Fatalf("expected two agent options, got %v", options)
	}

	// An unknown filter option is rejected.
	if _, err := c.GetEnforcementDecisionFilterOptions(t.Context(), "bogus", EnforcementDecisionOptions{}); err == nil {
		t.Fatal("expected unknown filter option to be rejected")
	}

	// An invalid sort key is rejected.
	if _, _, err := c.GetEnforcementDecisions(t.Context(), EnforcementDecisionOptions{SortBy: "reason"}); err == nil {
		t.Fatal("expected invalid sort key to be rejected")
	}
}
