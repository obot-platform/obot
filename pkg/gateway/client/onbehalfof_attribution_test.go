package client

import (
	"slices"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auditlog"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
)

func TestOnBehalfOfAttributionOnSQLite(t *testing.T) {
	testOnBehalfOfAttribution(t, newLifecycleTestClient(t))
}

func TestOnBehalfOfAttributionOnPostgres(t *testing.T) {
	testOnBehalfOfAttribution(t, newPostgresLifecycleTestClient(t))
}

// testOnBehalfOfAttribution checks the rows an agent writes when it acts for a user: they are the agent's, with the
// user as onBehalfOf and no user ID, so a user's rows, totals and stats leave them out unless a reader asks for them
// with IncludeOnBehalfOf, and the agent's include them.
func testOnBehalfOfAttribution(t *testing.T, c *Client) {
	t.Helper()

	ctx := t.Context()
	agentID := createTestAgent(t, c, "Assistant").ID
	agentOwner := principal.Agent(agentID).String()
	userID := decimal(agentID + 1000)
	now := time.Now().UTC().Truncate(time.Second)

	user := types.ActorForCaller(attributionTestCaller(userID, principal.KindUser))
	agent := types.ActorForCaller(attributionTestCaller(agentOwner, principal.KindAgent))
	forUserCaller := attributionTestCaller(agentOwner, principal.KindAgent)
	principal.RecordOnBehalfOf(forUserCaller.GetExtra(), agentID+1000)
	forUser := types.ActorForCaller(forUserCaller)
	if forUser != (types.Actor{PrincipalKind: "agent", PrincipalID: decimal(agentID), OnBehalfOf: userID}) {
		t.Fatalf("attribution of the agent acting for the user = %+v", forUser)
	}

	for _, log := range []types.LLMAuditLog{
		{ID: "llm-user", CreatedAt: now, UserID: user.UserID, PrincipalKind: user.PrincipalKind, ModelProvider: "openai"},
		{ID: "llm-agent", CreatedAt: now, PrincipalKind: agent.PrincipalKind, PrincipalID: agent.PrincipalID, ModelProvider: "openai"},
		{ID: "llm-for-user", CreatedAt: now, PrincipalKind: forUser.PrincipalKind, PrincipalID: forUser.PrincipalID, OnBehalfOf: forUser.OnBehalfOf, ModelProvider: "openai"},
	} {
		if err := c.InsertLLMAuditLog(ctx, &log); err != nil {
			t.Fatalf("failed to insert LLM audit log %s: %v", log.ID, err)
		}
	}
	for _, row := range []struct {
		tool  string
		actor types.Actor
	}{
		{tool: "user-tool", actor: user},
		{tool: "agent-tool", actor: agent},
		{tool: "for-user-tool", actor: forUser},
	} {
		if err := c.insertMCPAuditLogs(ctx, []types.MCPAuditLog{{
			CreatedAt: now, SourceType: apitypes.AuditLogSourceTypeMCP,
			UserID: row.actor.UserID, PrincipalKind: row.actor.PrincipalKind, PrincipalID: row.actor.PrincipalID, OnBehalfOf: row.actor.OnBehalfOf,
			MCPFields: &types.MCPAuditLogFields{MCPID: "mcp-1", CallType: "tools/call", CallIdentifier: row.tool},
		}}); err != nil {
			t.Fatalf("failed to insert MCP audit log %s: %v", row.tool, err)
		}
	}
	for _, row := range []struct {
		actor  types.Actor
		tokens int
	}{
		{actor: user, tokens: 10},
		{actor: agent, tokens: 20},
		{actor: forUser, tokens: 40},
	} {
		if err := c.InsertTokenUsage(ctx, &types.RunTokenActivity{
			CreatedAt: now, UserID: row.actor.UserID, PrincipalKind: row.actor.PrincipalKind, PrincipalID: row.actor.PrincipalID, OnBehalfOf: row.actor.OnBehalfOf,
			Model: "openai/gpt-5", Usage: types.TokenUsage{InputTokens: row.tokens, TotalTokens: row.tokens},
		}); err != nil {
			t.Fatalf("failed to insert token usage: %v", err)
		}
	}

	wantAgent := apitypes.PrincipalRef{Kind: apitypes.PrincipalKindAgent, ID: decimal(agentID)}
	wantUser := apitypes.PrincipalRef{Kind: apitypes.PrincipalKindUser, ID: userID}

	t.Run("llm audit logs", func(t *testing.T) {
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
		assertLLMLogs(t, LLMAuditLogOptions{UserID: []string{userID}}, "llm-user")
		assertLLMLogs(t, LLMAuditLogOptions{UserID: []string{userID}, IncludeOnBehalfOf: true}, "llm-user", "llm-for-user")
		// The agent's rows are all its own, whether or not it acted for someone.
		assertLLMLogs(t, LLMAuditLogOptions{UserID: []string{agentOwner}}, "llm-agent", "llm-for-user")
		assertLLMLogs(t, LLMAuditLogOptions{UserID: []string{agentOwner}, IncludeOnBehalfOf: true}, "llm-agent", "llm-for-user")
		assertLLMLogs(t, LLMAuditLogOptions{ActorKind: []string{"user"}, IncludeOnBehalfOf: true}, "llm-user")

		log, err := c.GetLLMAuditLog(ctx, "llm-for-user", false)
		if err != nil {
			t.Fatalf("failed to get the LLM audit log: %v", err)
		}
		if got := types.ConvertLLMAuditLog(*log); got.Principal != wantAgent || got.OnBehalfOf != wantUser || got.UserID != "" || got.AgentDisplayName != "Assistant" {
			t.Errorf("LLM audit log of the agent acting for the user = %+v", got)
		}
		log, err = c.GetLLMAuditLog(ctx, "llm-agent", false)
		if err != nil {
			t.Fatalf("failed to get the LLM audit log: %v", err)
		}
		if got := types.ConvertLLMAuditLog(*log); got.OnBehalfOf != (apitypes.PrincipalRef{}) {
			t.Errorf("LLM audit log of the agent acting for itself = %+v", got)
		}
	})

	t.Run("mcp audit logs", func(t *testing.T) {
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
		assertMCPTools(t, MCPAuditLogOptions{UserID: []string{userID}}, "user-tool")
		assertMCPTools(t, MCPAuditLogOptions{UserID: []string{userID}, IncludeOnBehalfOf: true}, "user-tool", "for-user-tool")
		assertMCPTools(t, MCPAuditLogOptions{Actor: []string{userID}}, "user-tool")
		assertMCPTools(t, MCPAuditLogOptions{Actor: []string{userID}, IncludeOnBehalfOf: true}, "user-tool", "for-user-tool")
		assertMCPTools(t, MCPAuditLogOptions{UserID: []string{agentOwner}}, "agent-tool", "for-user-tool")

		logs, _, err := c.GetMCPAuditLogs(ctx, MCPAuditLogOptions{
			SourceTypes: []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP},
			UserID:      []string{userID},
			// Only the agent's row is the user's through the agent.
			ActorKind:         []string{"agent"},
			IncludeOnBehalfOf: true,
		})
		if err != nil || len(logs) != 1 {
			t.Fatalf("MCP audit logs of the agent acting for the user = %d, %v", len(logs), err)
		}
		want := apitypes.AuditLogActor{ActorType: apitypes.AuditLogActorTypeAgent, ID: decimal(agentID), DisplayName: "Assistant", OnBehalfOf: wantUser}
		if actor := auditlog.Present(logs[0], auditlog.PresentOptions{}).Actor; actor != want {
			t.Errorf("actor = %+v, want %+v", actor, want)
		}

		usage := func(include bool) types.MCPUsageStatsList {
			t.Helper()
			stats, err := c.GetMCPUsageStats(ctx, MCPUsageStatsOptions{
				StartTime:         now.Add(-time.Hour),
				EndTime:           now.Add(time.Hour),
				UserIDs:           []string{userID},
				IncludeOnBehalfOf: include,
			})
			if err != nil {
				t.Fatalf("failed to get MCP usage stats: %v", err)
			}
			return stats
		}
		if stats := usage(false); stats.TotalCalls != 1 || stats.UniqueUsers != 1 {
			t.Errorf("MCP usage of the user = %d calls by %d callers, want 1 by 1", stats.TotalCalls, stats.UniqueUsers)
		}
		// The user's calls through the agent are the agent's: two callers.
		stats := usage(true)
		if stats.TotalCalls != 2 || stats.UniqueUsers != 2 {
			t.Errorf("MCP usage of the user through agents = %d calls by %d callers, want 2 by 2", stats.TotalCalls, stats.UniqueUsers)
		}
		var found bool
		for _, item := range stats.Items {
			for _, tool := range types.ConvertMCPUsageStats(item).ToolCalls {
				if tool.ToolName != "for-user-tool" {
					continue
				}
				for _, call := range tool.Items {
					found = true
					if call.Principal != wantAgent || call.OnBehalfOf != wantUser || call.UserID != "" {
						t.Errorf("tool call of the agent acting for the user = %+v", call)
					}
				}
			}
		}
		if !found {
			t.Error("the tool call of the agent acting for the user is missing from the usage stats")
		}
	})

	t.Run("token usage", func(t *testing.T) {
		records, err := c.TokenUsageForUser(ctx, userID, now.Add(-time.Hour), now.Add(time.Hour), false, nil)
		if err != nil || len(records) != 1 || records[0].Usage.TotalTokens != 10 {
			t.Errorf("token usage of the user = %+v, %v", records, err)
		}
		records, err = c.TokenUsageForUser(ctx, userID, now.Add(-time.Hour), now.Add(time.Hour), true, nil)
		if err != nil || len(records) != 2 {
			t.Fatalf("token usage of the user through agents = %+v, %v", records, err)
		}
		for _, record := range records {
			got := types.ConvertTokenActivity(record)
			switch record.Usage.TotalTokens {
			case 10:
				if got.Principal != wantUser || got.OnBehalfOf != (apitypes.PrincipalRef{}) {
					t.Errorf("the user's own token usage = %+v", got)
				}
			case 40:
				if got.Principal != wantAgent || got.OnBehalfOf != wantUser || got.UserID != "" {
					t.Errorf("token usage of the agent acting for the user = %+v", got)
				}
			default:
				t.Errorf("unexpected token usage record %+v", record)
			}
		}

		// Totals are the principal's own: the agent's work for the user counts for the agent, never the user.
		total, err := c.TotalTokenUsageForUser(ctx, userID, now.Add(-time.Hour), now.Add(time.Hour))
		if err != nil || total.Usage.TotalTokens != 10 {
			t.Errorf("total token usage of the user = %+v, %v", total.Usage, err)
		}
		total, err = c.TotalTokenUsageForUser(ctx, agentOwner, now.Add(-time.Hour), now.Add(time.Hour))
		if err != nil || total.Usage.TotalTokens != 60 {
			t.Errorf("total token usage of the agent = %+v, %v", total.Usage, err)
		}
		byPrincipal, err := c.TokenUsageByUser(ctx, now.Add(-time.Hour), now.Add(time.Hour))
		if err != nil || len(byPrincipal) != 2 {
			t.Fatalf("token usage by principal = %+v, %v", byPrincipal, err)
		}
		for _, row := range byPrincipal {
			if row.UserID == userID && row.Usage.TotalTokens != 10 || row.PrincipalKind == "agent" && row.Usage.TotalTokens != 60 {
				t.Errorf("token usage of %s %s%s = %d", row.PrincipalKind, row.UserID, row.PrincipalID, row.Usage.TotalTokens)
			}
		}

		for _, tt := range []struct {
			opts TokenUsageStatsOptions
			want int64
		}{
			{opts: TokenUsageStatsOptions{UserIDs: []string{userID}}, want: 1},
			{opts: TokenUsageStatsOptions{UserIDs: []string{userID}, IncludeOnBehalfOf: true}, want: 2},
			{opts: TokenUsageStatsOptions{UserIDs: []string{agentOwner}}, want: 2},
		} {
			tt.opts.Start, tt.opts.End = now.Add(-time.Hour), now.Add(time.Hour)
			stats, err := c.TokenUsageStats(ctx, tt.opts)
			if err != nil || stats.Totals.Requests != tt.want {
				t.Errorf("token usage stats for %+v = %d requests, %v; want %d", tt.opts, stats.Totals.Requests, err, tt.want)
			}
		}
	})
}

func TestOnBehalfOfFilterOnSQLite(t *testing.T) {
	testOnBehalfOfFilter(t, newLifecycleTestClient(t))
}

func TestOnBehalfOfFilterOnPostgres(t *testing.T) {
	testOnBehalfOfFilter(t, newPostgresLifecycleTestClient(t))
}

// testOnBehalfOfFilter checks the on-behalf-of filter of every activity source: it selects the rows agents made for
// the users it names, any of them, and every other filter narrows it, so that an agent and a user select what that
// agent did for that user, and nothing the agent did for itself or for someone else, what another agent did for the
// user, or what the user did themselves.
func testOnBehalfOfFilter(t *testing.T, c *Client) {
	t.Helper()

	ctx := t.Context()
	seven := createTestAgent(t, c, "Seven").ID
	eight := createTestAgent(t, c, "Eight").ID
	sevenOwner, eightOwner := principal.Agent(seven).String(), principal.Agent(eight).String()
	twelve, thirteen := decimal(seven+1000), decimal(seven+1001)
	now := time.Now().UTC().Truncate(time.Second)

	// Each row is named for who made it and for whom, and has a power of two of tokens, so a total names its rows.
	rows := []struct {
		name   string
		actor  types.Actor
		tokens int
	}{
		{name: "seven-for-twelve", actor: types.Actor{PrincipalKind: "agent", PrincipalID: decimal(seven), OnBehalfOf: twelve}, tokens: 1},
		{name: "eight-for-twelve", actor: types.Actor{PrincipalKind: "agent", PrincipalID: decimal(eight), OnBehalfOf: twelve}, tokens: 2},
		{name: "seven-for-thirteen", actor: types.Actor{PrincipalKind: "agent", PrincipalID: decimal(seven), OnBehalfOf: thirteen}, tokens: 4},
		{name: "twelve", actor: types.Actor{PrincipalKind: "user", UserID: twelve}, tokens: 8},
		{name: "seven", actor: types.Actor{PrincipalKind: "agent", PrincipalID: decimal(seven)}, tokens: 16},
	}
	tokensOf := map[string]int{}
	for _, row := range rows {
		tokensOf[row.name] = row.tokens
		actor := row.actor
		if err := c.InsertLLMAuditLog(ctx, &types.LLMAuditLog{
			ID: row.name, CreatedAt: now, UserID: actor.UserID, PrincipalKind: actor.PrincipalKind, PrincipalID: actor.PrincipalID,
			OnBehalfOf: actor.OnBehalfOf, ModelProvider: "openai",
		}); err != nil {
			t.Fatalf("failed to insert LLM audit log %s: %v", row.name, err)
		}
		if err := c.insertMCPAuditLogs(ctx, []types.MCPAuditLog{{
			CreatedAt: now, SourceType: apitypes.AuditLogSourceTypeMCP,
			UserID: actor.UserID, PrincipalKind: actor.PrincipalKind, PrincipalID: actor.PrincipalID, OnBehalfOf: actor.OnBehalfOf,
			MCPFields: &types.MCPAuditLogFields{MCPID: "mcp-1", CallType: "tools/call", CallIdentifier: row.name},
		}}); err != nil {
			t.Fatalf("failed to insert MCP audit log %s: %v", row.name, err)
		}
		if err := c.InsertTokenUsage(ctx, &types.RunTokenActivity{
			CreatedAt: now, UserID: actor.UserID, PrincipalKind: actor.PrincipalKind, PrincipalID: actor.PrincipalID, OnBehalfOf: actor.OnBehalfOf,
			Model: "openai/gpt-5", Usage: types.TokenUsage{InputTokens: row.tokens, TotalTokens: row.tokens},
		}); err != nil {
			t.Fatalf("failed to insert token usage %s: %v", row.name, err)
		}
	}

	// The cases each source is checked with. owners is the filter on who acted: the user_id of the LLM audit log and
	// of usage, and both the user_id and the actor of the MCP audit log.
	tests := []struct {
		name       string
		owners     []string
		include    bool
		actorKinds []string
		onBehalfOf []string
		want       []string
	}{
		{
			name:       "one agent for one user",
			owners:     []string{sevenOwner},
			onBehalfOf: []string{twelve},
			want:       []string{"seven-for-twelve"},
		},
		{
			name:       "every agent for one user",
			onBehalfOf: []string{twelve},
			want:       []string{"seven-for-twelve", "eight-for-twelve"},
		},
		{
			name:       "every agent for any of two users",
			onBehalfOf: []string{twelve, thirteen},
			want:       []string{"seven-for-twelve", "eight-for-twelve", "seven-for-thirteen"},
		},
		{
			name:       "any of two agents for one user",
			owners:     []string{sevenOwner, eightOwner},
			onBehalfOf: []string{twelve},
			want:       []string{"seven-for-twelve", "eight-for-twelve"},
		},
		{
			name:       "the user's own actions are not on their behalf",
			owners:     []string{twelve},
			onBehalfOf: []string{twelve},
		},
		{
			name:       "including actions on the user's behalf",
			owners:     []string{twelve},
			include:    true,
			onBehalfOf: []string{twelve},
			want:       []string{"seven-for-twelve", "eight-for-twelve"},
		},
		{
			name:       "users act on no one's behalf",
			actorKinds: []string{"user"},
			onBehalfOf: []string{twelve, thirteen},
		},
		{
			name:       "no such user",
			onBehalfOf: []string{"999999"},
		},
	}

	sameNames := func(got, want []string) bool {
		got, want = slices.Clone(got), slices.Clone(want)
		slices.Sort(got)
		slices.Sort(want)
		return slices.Equal(got, want)
	}
	tokens := func(names []string) int64 {
		var total int64
		for _, name := range names {
			total += int64(tokensOf[name])
		}
		return total
	}

	t.Run("llm audit logs", func(t *testing.T) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				logs, total, err := c.GetLLMAuditLogs(ctx, LLMAuditLogOptions{
					UserID:            tt.owners,
					IncludeOnBehalfOf: tt.include,
					ActorKind:         tt.actorKinds,
					OnBehalfOf:        tt.onBehalfOf,
				})
				if err != nil {
					t.Fatalf("failed to get LLM audit logs: %v", err)
				}
				var got []string
				for _, log := range logs {
					got = append(got, log.ID)
				}
				if !sameNames(got, tt.want) || total != int64(len(tt.want)) {
					t.Errorf("LLM audit logs = %v (total %d), want %v", got, total, tt.want)
				}
			})
		}
	})

	t.Run("mcp audit logs", func(t *testing.T) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// The filter on who acted is the user_id filter of exports and the actor filter of the audit log
				// view, over one source or both.
				for _, opts := range []MCPAuditLogOptions{
					{SourceTypes: []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP}, UserID: tt.owners},
					{Actor: tt.owners, SourceTypes: []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP, apitypes.AuditLogSourceTypeLocalAgentToolCall}},
				} {
					opts.IncludeOnBehalfOf, opts.ActorKind, opts.OnBehalfOf = tt.include, tt.actorKinds, tt.onBehalfOf
					logs, total, err := c.GetMCPAuditLogs(ctx, opts)
					if err != nil {
						t.Fatalf("failed to get MCP audit logs: %v", err)
					}
					var got []string
					for _, log := range logs {
						got = append(got, log.MCP().CallIdentifier)
					}
					if !sameNames(got, tt.want) || total != int64(len(tt.want)) {
						t.Errorf("MCP audit logs for user %v, actor %v = %v (total %d), want %v", opts.UserID, opts.Actor, got, total, tt.want)
					}
				}
			})
		}
	})

	t.Run("mcp usage stats", func(t *testing.T) {
		for _, tt := range tests {
			if len(tt.actorKinds) > 0 {
				// Usage stats have no filter on the kind of actor.
				continue
			}
			t.Run(tt.name, func(t *testing.T) {
				stats, err := c.GetMCPUsageStats(ctx, MCPUsageStatsOptions{
					StartTime:         now.Add(-time.Hour),
					EndTime:           now.Add(time.Hour),
					UserIDs:           tt.owners,
					IncludeOnBehalfOf: tt.include,
					OnBehalfOf:        tt.onBehalfOf,
				})
				if err != nil {
					t.Fatalf("failed to get MCP usage stats: %v", err)
				}
				// A server's tool calls are narrowed like its totals.
				var got []string
				for _, item := range stats.Items {
					for _, tool := range item.ToolCalls {
						got = append(got, tool.ToolName)
					}
				}
				if stats.TotalCalls != int64(len(tt.want)) || !sameNames(got, tt.want) {
					t.Errorf("MCP usage stats = %d calls, tools %v; want %v", stats.TotalCalls, got, tt.want)
				}
			})
		}
	})

	t.Run("token usage stats", func(t *testing.T) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stats, err := c.TokenUsageStats(ctx, TokenUsageStatsOptions{
					Start:             now.Add(-time.Hour),
					End:               now.Add(time.Hour),
					UserIDs:           tt.owners,
					IncludeOnBehalfOf: tt.include,
					ActorKinds:        tt.actorKinds,
					OnBehalfOf:        tt.onBehalfOf,
				})
				if err != nil {
					t.Fatalf("failed to get token usage stats: %v", err)
				}
				if stats.Totals.Requests != int64(len(tt.want)) || stats.Totals.TotalTokens != tokens(tt.want) {
					t.Errorf("token usage stats = %d requests, %d tokens; want %v", stats.Totals.Requests, stats.Totals.TotalTokens, tt.want)
				}
			})
		}
	})

	t.Run("token usage of a principal", func(t *testing.T) {
		for _, tt := range []struct {
			owner      string
			include    bool
			onBehalfOf []string
			want       []string
		}{
			{owner: sevenOwner, onBehalfOf: []string{twelve}, want: []string{"seven-for-twelve"}},
			{owner: sevenOwner, onBehalfOf: []string{twelve, thirteen}, want: []string{"seven-for-twelve", "seven-for-thirteen"}},
			{owner: twelve, onBehalfOf: []string{twelve}},
			{owner: twelve, include: true, onBehalfOf: []string{twelve}, want: []string{"seven-for-twelve", "eight-for-twelve"}},
		} {
			records, err := c.TokenUsageForUser(ctx, tt.owner, now.Add(-time.Hour), now.Add(time.Hour), tt.include, tt.onBehalfOf)
			if err != nil {
				t.Fatalf("failed to get token usage: %v", err)
			}
			var got int64
			for _, record := range records {
				got += int64(record.Usage.TotalTokens)
			}
			if len(records) != len(tt.want) || got != tokens(tt.want) {
				t.Errorf("token usage of %s on behalf of %v (include %v) = %d records, %d tokens; want %v", tt.owner, tt.onBehalfOf, tt.include, len(records), got, tt.want)
			}
		}
	})

	t.Run("filter options", func(t *testing.T) {
		mcpOnly := []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP}
		bothSources := []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP, apitypes.AuditLogSourceTypeLocalAgentToolCall}
		for _, tt := range []struct {
			name   string
			option string
			opts   MCPAuditLogOptions
			want   []string
		}{
			{name: "the users agents acted for", option: "on_behalf_of", opts: MCPAuditLogOptions{SourceTypes: bothSources}, want: []string{twelve, thirteen}},
			{name: "the users one agent acted for", option: "on_behalf_of", opts: MCPAuditLogOptions{SourceTypes: mcpOnly, Actor: []string{eightOwner}}, want: []string{twelve}},
			{name: "the agents that acted for one user", option: "actor", opts: MCPAuditLogOptions{SourceTypes: bothSources, OnBehalfOf: []string{twelve}}, want: []string{sevenOwner, eightOwner}},
			{name: "the operations for one user", option: "operation", opts: MCPAuditLogOptions{SourceTypes: mcpOnly, OnBehalfOf: []string{thirteen}}, want: []string{"tools/call"}},
		} {
			t.Run("mcp "+tt.name, func(t *testing.T) {
				got, err := c.GetAuditLogFilterOptions(ctx, tt.option, tt.opts)
				if err != nil {
					t.Fatalf("failed to get MCP filter options: %v", err)
				}
				if !sameNames(got, tt.want) {
					t.Errorf("MCP %s options = %v, want %v", tt.option, got, tt.want)
				}
			})
		}
		// The legacy per-column options of one source are narrowed too.
		got, err := c.GetAuditLogFilterOptions(ctx, "call_identifier", MCPAuditLogOptions{SourceTypes: mcpOnly, OnBehalfOf: []string{thirteen}}, "")
		if err != nil || !slices.Equal(got, []string{"seven-for-thirteen"}) {
			t.Errorf("MCP call_identifier options on behalf of the other user = %v, %v", got, err)
		}

		for _, tt := range []struct {
			name   string
			option string
			opts   LLMAuditLogOptions
			want   []string
		}{
			{name: "the users agents acted for", option: "on_behalf_of", want: []string{twelve, thirteen}},
			{name: "the users one agent acted for", option: "on_behalf_of", opts: LLMAuditLogOptions{UserID: []string{eightOwner}}, want: []string{twelve}},
			{name: "the agents that acted for one user", option: "user_id", opts: LLMAuditLogOptions{OnBehalfOf: []string{twelve}}, want: []string{sevenOwner, eightOwner}},
		} {
			t.Run("llm "+tt.name, func(t *testing.T) {
				got, err := c.GetLLMAuditLogFilterOptions(ctx, tt.option, tt.opts, "")
				if err != nil {
					t.Fatalf("failed to get LLM filter options: %v", err)
				}
				if !sameNames(got, tt.want) {
					t.Errorf("LLM %s options = %v, want %v", tt.option, got, tt.want)
				}
			})
		}
	})
}
