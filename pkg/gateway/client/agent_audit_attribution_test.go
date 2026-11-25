package client

import (
	"slices"
	"strconv"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auditlog"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func attributionTestCaller(uid string, kind principal.Kind) kuser.Info {
	extra := map[string][]string{}
	principal.RecordKind(extra, kind)
	return &kuser.DefaultInfo{UID: uid, Extra: extra}
}

// attributionTestActors are a user and an agent whose numeric IDs are the same, the case that must stay apart.
func attributionTestActors(t *testing.T, c *Client) (user types.Actor, agent types.Actor, agentID uint) {
	t.Helper()

	created := createTestAgent(t, c, "Nightly builder")
	agentID = created.ID
	user = types.ActorForCaller(attributionTestCaller(decimal(created.ID), principal.KindUser))
	agent = types.ActorForCaller(attributionTestCaller(principal.Agent(created.ID).String(), principal.KindAgent))
	return user, agent, agentID
}

func decimal(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

func TestAgentActivityAttributionOnSQLite(t *testing.T) {
	testAgentActivityAttribution(t, newLifecycleTestClient(t))
}

func TestAgentActivityAttributionOnPostgres(t *testing.T) {
	testAgentActivityAttribution(t, newPostgresLifecycleTestClient(t))
}

// testAgentActivityAttribution checks that the rows of an agent and of a user with the same numeric ID are told
// apart in LLM audit logs, MCP audit logs, token usage and budget usage: by their filters, their normalized read
// model and their totals, and that an agent's rows stay readable, with its name, after the agent is deleted.
func testAgentActivityAttribution(t *testing.T, c *Client) {
	t.Helper()

	ctx := t.Context()
	user, agent, agentID := attributionTestActors(t, c)
	agentOwner := principal.Agent(agentID).String()
	now := time.Now().UTC().Truncate(time.Second)
	agentKey := createTestAgentAPIKey(t, c, agentID, "ci key")

	if agent.UserID != "" || agent.PrincipalKind != "agent" || agent.PrincipalID != decimal(agentID) {
		t.Fatalf("agent attribution = %+v", agent)
	}
	if user.UserID != decimal(agentID) || user.PrincipalKind != "user" {
		t.Fatalf("user attribution = %+v", user)
	}

	// LLM audit logs.
	for _, log := range []types.LLMAuditLog{
		{ID: "llm-user", CreatedAt: now, UserID: user.UserID, PrincipalKind: user.PrincipalKind, PrincipalID: user.PrincipalID, ModelProvider: "openai"},
		{ID: "llm-agent", CreatedAt: now.Add(time.Second), UserID: agent.UserID, PrincipalKind: agent.PrincipalKind, PrincipalID: agent.PrincipalID,
			APIKeyID: &agentKey.ID, APIKeyName: agentKey.Name, ModelProvider: "openai"},
		// A row from before principals.
		{ID: "llm-legacy", CreatedAt: now.Add(-time.Second), UserID: user.UserID, ModelProvider: "openai"},
	} {
		if err := c.InsertLLMAuditLog(ctx, &log); err != nil {
			t.Fatalf("failed to insert LLM audit log %s: %v", log.ID, err)
		}
	}

	// MCP audit logs.
	for _, log := range []types.MCPAuditLog{
		{CreatedAt: now, SourceType: apitypes.AuditLogSourceTypeMCP, UserID: user.UserID, PrincipalKind: user.PrincipalKind, PrincipalID: user.PrincipalID,
			MCPFields: &types.MCPAuditLogFields{MCPID: "mcp-1", CallType: "tools/call", CallIdentifier: "user-tool"}},
		{CreatedAt: now.Add(time.Second), SourceType: apitypes.AuditLogSourceTypeMCP, UserID: agent.UserID, PrincipalKind: agent.PrincipalKind, PrincipalID: agent.PrincipalID,
			APIKeyID: &agentKey.ID, APIKeyName: agentKey.Name,
			MCPFields: &types.MCPAuditLogFields{MCPID: "mcp-1", CallType: "tools/call", CallIdentifier: "agent-tool"}},
	} {
		if err := c.insertMCPAuditLogs(ctx, []types.MCPAuditLog{log}); err != nil {
			t.Fatalf("failed to insert MCP audit log %s: %v", log.MCP().CallIdentifier, err)
		}
	}

	// Token usage and budget usage.
	for _, actor := range []types.Actor{user, agent} {
		if err := c.InsertTokenUsage(ctx, &types.RunTokenActivity{
			UserID: actor.UserID, PrincipalKind: actor.PrincipalKind, PrincipalID: actor.PrincipalID,
			Model: "openai/gpt-5", Usage: types.TokenUsage{InputTokens: 10, TotalTokens: 10, TotalSpend: 1},
		}); err != nil {
			t.Fatalf("failed to insert token usage: %v", err)
		}
	}
	if err := c.RecordLLMGatewayBudgetUsage(ctx, []string{"budget"}, decimal(agentID), "openai/gpt-5", types.TokenUsage{TotalTokens: 10}); err != nil {
		t.Fatalf("failed to record the user's budget usage: %v", err)
	}
	if err := c.RecordLLMGatewayBudgetUsage(ctx, []string{"budget"}, agentOwner, "openai/gpt-5", types.TokenUsage{TotalTokens: 20}); err != nil {
		t.Fatalf("failed to record the agent's budget usage: %v", err)
	}

	assertLLMLogs := func(t *testing.T, opts LLMAuditLogOptions, want ...string) {
		t.Helper()
		logs, total, err := c.GetLLMAuditLogs(ctx, opts)
		if err != nil {
			t.Fatalf("failed to get LLM audit logs: %v", err)
		}
		var got []string
		for _, log := range logs {
			got = append(got, log.ID)
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) || total != int64(len(want)) {
			t.Errorf("LLM audit logs for %+v = %v (total %d), want %v", opts, got, total, want)
		}
	}
	assertMCPTools := func(t *testing.T, opts MCPAuditLogOptions, want ...string) {
		t.Helper()
		opts.SourceTypes = []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP}
		logs, _, err := c.GetMCPAuditLogs(ctx, opts)
		if err != nil {
			t.Fatalf("failed to get MCP audit logs: %v", err)
		}
		var got []string
		for _, log := range logs {
			got = append(got, log.MCP().CallIdentifier)
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("MCP audit logs for %+v = %v, want %v", opts, got, want)
		}
	}

	agentIDString := decimal(agentID)
	deleted := false
	check := func(t *testing.T) {
		t.Helper()

		t.Run("llm audit log filters", func(t *testing.T) {
			assertLLMLogs(t, LLMAuditLogOptions{}, "llm-user", "llm-agent", "llm-legacy")
			assertLLMLogs(t, LLMAuditLogOptions{ActorKind: []string{"agent"}}, "llm-agent")
			assertLLMLogs(t, LLMAuditLogOptions{ActorKind: []string{"user"}}, "llm-user", "llm-legacy")
			assertLLMLogs(t, LLMAuditLogOptions{UserID: []string{agentOwner}}, "llm-agent")
			// The user with the agent's numeric ID is not the agent, and the legacy row is theirs.
			assertLLMLogs(t, LLMAuditLogOptions{UserID: []string{agentIDString}}, "llm-user", "llm-legacy")
			assertLLMLogs(t, LLMAuditLogOptions{UserID: []string{principal.Agent(9999).String()}})
			assertLLMLogs(t, LLMAuditLogOptions{APIKeyID: []uint{agentKey.ID}}, "llm-agent")
			assertLLMLogs(t, LLMAuditLogOptions{Query: "Nightly"}, "llm-agent")
		})

		t.Run("llm audit log read model", func(t *testing.T) {
			logs, _, err := c.GetLLMAuditLogs(t.Context(), LLMAuditLogOptions{})
			if err != nil {
				t.Fatalf("failed to get LLM audit logs: %v", err)
			}
			byID := map[string]apitypes.LLMAuditLog{}
			for _, log := range logs {
				byID[log.ID] = types.ConvertLLMAuditLog(log)
			}
			wantAgent := apitypes.PrincipalRef{Kind: apitypes.PrincipalKindAgent, ID: agentIDString}
			wantUser := apitypes.PrincipalRef{Kind: apitypes.PrincipalKindUser, ID: agentIDString}
			if got := byID["llm-agent"]; got.Principal != wantAgent || got.UserID != "" || got.AgentDisplayName != "Nightly builder" ||
				got.APIKeyID == nil || *got.APIKeyID != agentKey.ID {
				t.Errorf("agent LLM audit log = %+v", got)
			}
			for _, id := range []string{"llm-user", "llm-legacy"} {
				if got := byID[id]; got.Principal != wantUser || got.UserID != agentIDString || got.AgentDisplayName != "" {
					t.Errorf("user LLM audit log %s = %+v", id, got)
				}
			}
			// An agent's option is its storage encoding, which the user_id filter takes.
			options, err := c.GetLLMAuditLogFilterOptions(t.Context(), "user_id", LLMAuditLogOptions{}, "")
			if err != nil || !slices.Equal(options, []string{agentOwner, agentIDString}) && !slices.Equal(options, []string{agentIDString, agentOwner}) {
				t.Errorf("user_id options = %v, %v", options, err)
			}
		})

		t.Run("mcp audit log filters", func(t *testing.T) {
			assertMCPTools(t, MCPAuditLogOptions{}, "user-tool", "agent-tool")
			assertMCPTools(t, MCPAuditLogOptions{ActorKind: []string{"agent"}}, "agent-tool")
			assertMCPTools(t, MCPAuditLogOptions{ActorKind: []string{"user"}}, "user-tool")
			assertMCPTools(t, MCPAuditLogOptions{UserID: []string{agentOwner}}, "agent-tool")
			assertMCPTools(t, MCPAuditLogOptions{UserID: []string{agentIDString}}, "user-tool")
			assertMCPTools(t, MCPAuditLogOptions{Actor: []string{agentOwner}}, "agent-tool")
			assertMCPTools(t, MCPAuditLogOptions{Actor: []string{agentIDString}}, "user-tool")
			assertMCPTools(t, MCPAuditLogOptions{Query: "nightly"}, "agent-tool")

			actors, err := c.GetAuditLogFilterOptions(t.Context(), "actor", MCPAuditLogOptions{
				SourceTypes: []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP},
			})
			if err != nil || !slices.Equal(actors, []string{agentOwner, agentIDString}) && !slices.Equal(actors, []string{agentIDString, agentOwner}) {
				t.Errorf("actor options = %v, %v", actors, err)
			}
		})

		t.Run("mcp audit log read model", func(t *testing.T) {
			logs, _, err := c.GetMCPAuditLogs(t.Context(), MCPAuditLogOptions{
				SourceTypes: []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP},
			})
			if err != nil {
				t.Fatalf("failed to get MCP audit logs: %v", err)
			}
			for _, log := range logs {
				event := auditlog.Present(log, auditlog.PresentOptions{})
				switch log.MCP().CallIdentifier {
				case "agent-tool":
					wantKey := "ci key (oak1-" + agentIDString + "-" + decimal(agentKey.ID) + "-*****)"
					want := apitypes.AuditLogActor{
						ActorType:    apitypes.AuditLogActorTypeAgent,
						ID:           agentIDString,
						DisplayName:  "Nightly builder",
						CredentialID: wantKey,
						// Deleting an agent revokes its keys.
						APIKeyRevoked: deleted,
					}
					if event.Actor != want {
						t.Errorf("agent actor = %+v, want %+v", event.Actor, want)
					}
				case "user-tool":
					want := apitypes.AuditLogActor{ActorType: apitypes.AuditLogActorTypeUser, ID: agentIDString}
					if event.Actor != want {
						t.Errorf("user actor = %+v, want %+v", event.Actor, want)
					}
				}
			}

			stats, err := c.GetMCPUsageStats(t.Context(), MCPUsageStatsOptions{StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour)})
			if err != nil {
				t.Fatalf("failed to get MCP usage stats: %v", err)
			}
			// The user and the agent with the same numeric ID are two callers.
			if stats.UniqueUsers != 2 {
				t.Errorf("unique callers = %d, want 2", stats.UniqueUsers)
			}
		})

		t.Run("token usage", func(t *testing.T) {
			byOwner := map[string]types.RunTokenActivity{}
			usage, err := c.TokenUsageByUser(t.Context(), now.Add(-time.Hour), now.Add(time.Hour))
			if err != nil {
				t.Fatalf("failed to get token usage: %v", err)
			}
			for _, u := range usage {
				byOwner[u.PrincipalKind+":"+u.PrincipalID+":"+u.UserID] = u
			}
			if len(usage) != 2 || byOwner["agent:"+agentIDString+":"].Usage.TotalTokens != 10 || byOwner["user::"+agentIDString].Usage.TotalTokens != 10 {
				t.Errorf("token usage by principal = %+v", usage)
			}

			agentUsage, err := c.TotalTokenUsageForUser(t.Context(), agentOwner, now.Add(-time.Hour), now.Add(time.Hour))
			if err != nil || agentUsage.Usage.TotalTokens != 10 || agentUsage.UserID != "" || agentUsage.PrincipalKind != "agent" {
				t.Errorf("agent total token usage = %+v, %v", agentUsage, err)
			}
			if ref := types.ConvertTokenActivity(agentUsage).Principal; ref != (apitypes.PrincipalRef{Kind: apitypes.PrincipalKindAgent, ID: agentIDString}) {
				t.Errorf("agent token usage principal = %+v", ref)
			}
			records, err := c.TokenUsageForUser(t.Context(), agentOwner, now.Add(-time.Hour), now.Add(time.Hour), false, nil)
			if err != nil || len(records) != 1 || records[0].PrincipalKind != "agent" {
				t.Errorf("agent token usage records = %+v, %v", records, err)
			}
			empty, err := c.TotalTokenUsageForUser(t.Context(), principal.Agent(9999).String(), now.Add(-time.Hour), now.Add(time.Hour))
			if err != nil || empty.Usage.TotalTokens != 0 || empty.PrincipalKind != "agent" || empty.PrincipalID != "9999" {
				t.Errorf("unknown agent token usage = %+v, %v", empty, err)
			}

			stats, err := c.TokenUsageStats(t.Context(), TokenUsageStatsOptions{Start: now.Add(-time.Hour), End: now.Add(time.Hour), UserIDs: []string{agentOwner}})
			if err != nil || stats.Totals.Requests != 1 {
				t.Errorf("agent token usage stats = %+v, %v", stats.Totals, err)
			}
			stats, err = c.TokenUsageStats(t.Context(), TokenUsageStatsOptions{Start: now.Add(-time.Hour), End: now.Add(time.Hour), UserIDs: []string{agentIDString}})
			if err != nil || stats.Totals.Requests != 1 {
				t.Errorf("user token usage stats = %+v, %v", stats.Totals, err)
			}
			stats, err = c.TokenUsageStats(t.Context(), TokenUsageStatsOptions{Start: now.Add(-time.Hour), End: now.Add(time.Hour), ActorKinds: []string{"agent", "user"}})
			if err != nil || stats.Totals.Requests != 2 {
				t.Errorf("token usage stats of both kinds = %+v, %v", stats.Totals, err)
			}
		})

		t.Run("budget usage", func(t *testing.T) {
			entries, err := c.LLMGatewayBudgetUsageReport(t.Context(), &types.LLMGatewayBudget{
				ID:      "budget",
				PerUser: true,
				Limits:  []types.LLMGatewayBudgetLimit{{Metric: types.LLMGatewayBudgetMetricTokens, Amount: 100, Window: "1d"}},
			})
			if err != nil {
				t.Fatalf("failed to report budget usage: %v", err)
			}
			got := map[apitypes.PrincipalRef]float64{}
			for _, entry := range entries {
				got[entry.Principal] = entry.Used[0]
			}
			want := map[apitypes.PrincipalRef]float64{
				{Kind: apitypes.PrincipalKindUser, ID: agentIDString}:  10,
				{Kind: apitypes.PrincipalKindAgent, ID: agentIDString}: 20,
			}
			if len(got) != len(want) || got[apitypes.PrincipalRef{Kind: apitypes.PrincipalKindUser, ID: agentIDString}] != 10 ||
				got[apitypes.PrincipalRef{Kind: apitypes.PrincipalKindAgent, ID: agentIDString}] != 20 {
				t.Errorf("budget usage = %v, want %v", got, want)
			}
			for _, entry := range entries {
				if entry.Principal.Kind == apitypes.PrincipalKindAgent && entry.UserID != agentOwner {
					t.Errorf("agent budget entry user ID = %q, want %q", entry.UserID, agentOwner)
				}
			}
		})

		t.Run("api key options", func(t *testing.T) {
			options, err := c.GetLLMAuditLogAPIKeyFilterOptions(t.Context(), LLMAuditLogOptions{})
			if err != nil {
				t.Fatalf("failed to get API key options: %v", err)
			}
			if len(options) != 1 {
				t.Fatalf("API key options = %+v", options)
			}
			want := apitypes.AuditLogAPIKeyFilterOption{
				Value:            decimal(agentKey.ID),
				Name:             "ci key",
				MaskedKey:        "oak1-" + agentIDString + "-" + decimal(agentKey.ID) + "-*****",
				AgentID:          agentIDString,
				AgentDisplayName: "Nightly builder",
				// Deleting an agent revokes its keys.
				Revoked: deleted,
			}
			if options[0] != want {
				t.Errorf("API key option = %+v, want %+v", options[0], want)
			}
		})
	}

	check(t)

	// Deleting the agent revokes its keys and keeps its row: its history, and its name, stay readable.
	if err := c.DeleteAgent(ctx, agentID); err != nil {
		t.Fatalf("failed to delete the agent: %v", err)
	}
	deleted = true
	t.Run("after the agent is deleted", check)
}
