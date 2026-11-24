package client

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func adminAuditEvents(t *testing.T, c *Client) []types.AdminAuditEvent {
	t.Helper()
	events, total, err := c.ListAdminAuditEvents(t.Context(), AdminAuditEventOptions{Oldest: true})
	require.NoError(t, err)
	require.Equal(t, int64(len(events)), total)
	return events
}

func adminAuditDetails(t *testing.T, event types.AdminAuditEvent) map[string]any {
	t.Helper()
	var details map[string]any
	require.NoError(t, json.Unmarshal(event.Details, &details))
	return details
}

// TestAdminAuditOfAgentChanges checks that every change an administrator makes to an agent or its keys writes exactly
// one event, with the administrator, in the transaction of the change, and that a change that changes nothing, or that
// no administrator made, writes none.
func TestAdminAuditOfAgentChanges(t *testing.T) {
	for _, database := range grantTestDatabases() {
		t.Run(database.name, func(t *testing.T) {
			c := database.open(t)
			admin := WithAdminActor(t.Context(), "42")
			agentID := func(id uint) string { return strconv.FormatUint(uint64(id), 10) }

			// An agent made by no administrator, such as by a controller or a test, leaves no event.
			quiet := createTestAgent(t, c, "quiet")
			require.Empty(t, adminAuditEvents(t, c))
			_, err := c.UpdateAgent(t.Context(), types.Agent{ID: quiet.ID, DisplayName: "renamed"})
			require.NoError(t, err)
			require.Empty(t, adminAuditEvents(t, c))

			agent, err := c.CreateAgent(admin, types.Agent{DisplayName: "builder", Description: "builds", CreatorUserID: 42})
			require.NoError(t, err)

			_, err = c.UpdateAgent(admin, types.Agent{ID: agent.ID, DisplayName: "builder", Description: "builds more"})
			require.NoError(t, err)
			// Saving what is stored changes nothing.
			_, err = c.UpdateAgent(admin, types.Agent{ID: agent.ID, DisplayName: "builder", Description: "builds more"})
			require.NoError(t, err)

			key, err := c.CreateAgentAPIKey(admin, agent.ID, types.AgentAPIKeyCreateRequest{Name: "ci", CanAccessLLMProxy: true, MCPServerIDs: []string{"*"}})
			require.NoError(t, err)

			_, err = c.DisableAgent(admin, agent.ID, "rotating keys")
			require.NoError(t, err)
			// The same disable again changes nothing; another reason changes only the reason.
			_, err = c.DisableAgent(admin, agent.ID, "rotating keys")
			require.NoError(t, err)
			_, err = c.DisableAgent(admin, agent.ID, "")
			require.NoError(t, err)
			_, err = c.EnableAgent(admin, agent.ID)
			require.NoError(t, err)
			// Enabling an agent that is not disabled changes nothing.
			_, err = c.EnableAgent(admin, agent.ID)
			require.NoError(t, err)

			require.NoError(t, c.RevokeAgentAPIKey(admin, agent.ID, key.ID))
			// Revoking a revoked key, or another agent's key, changes nothing.
			require.NoError(t, c.RevokeAgentAPIKey(admin, agent.ID, key.ID))
			require.NoError(t, c.RevokeAgentAPIKey(admin, agent.ID+100, key.ID))

			second, err := c.CreateAgentAPIKey(admin, agent.ID, types.AgentAPIKeyCreateRequest{Name: "second", CanAccessSkills: true})
			require.NoError(t, err)
			third, err := c.CreateAgentAPIKey(admin, agent.ID, types.AgentAPIKeyCreateRequest{Name: "third", CanAccessSkills: true})
			require.NoError(t, err)
			// The admin API keys route revokes any key, and an agent's is recorded as the agent's.
			require.NoError(t, c.RevokeAPIKeyByID(admin, third.ID))
			// A user's key is not an agent's, and is not recorded.
			userKey, err := c.CreateAPIKey(t.Context(), 42, "mine", "", nil, types.APIKeyScopes{CanAccessAPI: true})
			require.NoError(t, err)
			require.NoError(t, c.RevokeAPIKeyByID(admin, userKey.ID))

			require.NoError(t, c.DeleteAgent(admin, agent.ID))

			type want struct {
				action, kind, id, parent string
			}
			var got []want
			for _, event := range adminAuditEvents(t, c) {
				require.Equal(t, "42", event.ActorUserID)
				require.False(t, event.CreatedAt.IsZero())
				got = append(got, want{event.Action, event.ResourceKind, event.ResourceID, event.ParentID})
			}
			require.Equal(t, []want{
				{"agent.create", "agent", agentID(agent.ID), ""},
				{"agent.update", "agent", agentID(agent.ID), ""},
				{"agent.key.create", "apiKey", strconv.Itoa(int(key.ID)), agentID(agent.ID)},
				{"agent.disable", "agent", agentID(agent.ID), ""},
				{"agent.disable", "agent", agentID(agent.ID), ""},
				{"agent.enable", "agent", agentID(agent.ID), ""},
				{"agent.key.revoke", "apiKey", strconv.Itoa(int(key.ID)), agentID(agent.ID)},
				{"agent.key.create", "apiKey", strconv.Itoa(int(second.ID)), agentID(agent.ID)},
				{"agent.key.create", "apiKey", strconv.Itoa(int(third.ID)), agentID(agent.ID)},
				{"agent.key.revoke", "apiKey", strconv.Itoa(int(third.ID)), agentID(agent.ID)},
				{"agent.delete", "agent", agentID(agent.ID), ""},
			}, got)

			events := adminAuditEvents(t, c)
			require.Equal(t, map[string]any{"changedFields": []any{"description"}}, adminAuditDetails(t, events[1]))
			require.Equal(t, true, adminAuditDetails(t, events[3])["accessChanged"])
			require.Equal(t, true, adminAuditDetails(t, events[3])["hasReason"])
			require.Equal(t, false, adminAuditDetails(t, events[4])["accessChanged"])
			require.Equal(t, false, adminAuditDetails(t, events[4])["hasReason"])
			// Deleting revokes the keys that were left: the second one.
			require.EqualValues(t, 1, adminAuditDetails(t, events[10])["keysRevoked"])

			// The events outlive the agent.
			require.True(t, storedAgent(t, c, agent.ID).DeletedAt.Valid)
			require.Len(t, adminAuditEvents(t, c), 11)
		})
	}
}

// TestAdminAuditNeverRecordsKeySecrets checks that the event of a new API key names the key and its scopes, and holds
// nothing of its secret, which the response alone carries.
func TestAdminAuditNeverRecordsKeySecrets(t *testing.T) {
	c := newTestClient(t)
	admin := WithAdminActor(t.Context(), "42")
	agent := createTestAgent(t, c, "builder")
	key, err := c.CreateAgentAPIKey(admin, agent.ID, types.AgentAPIKeyCreateRequest{Name: "ci", CanAccessSkills: true, MCPServerIDs: []string{"*"}})
	require.NoError(t, err)
	// The key is oak1-<agent>-<id>-<secret>.
	secret := key.Key[strings.LastIndex(key.Key, "-")+1:]
	require.NotEmpty(t, secret)

	var rows []types.AdminAuditEvent
	require.NoError(t, c.db.WithContext(t.Context()).Find(&rows).Error)
	require.Len(t, rows, 1)
	serialized, err := json.Marshal(rows[0])
	require.NoError(t, err)
	require.NotContains(t, string(serialized), secret)
	require.NotContains(t, string(serialized), key.Key)
	require.Equal(t, map[string]any{
		"name":              "ci",
		"canAccessSkills":   true,
		"canAccessLLMProxy": false,
		"mcpServerIDs":      []any{"*"},
	}, adminAuditDetails(t, rows[0]))
}

// TestAdminAuditOfGrantInvalidation checks that a vMCP's preconfigured OAuth grant that is marked invalid is recorded
// as the system's change, against the vMCP, once, and that a grant that is already invalid, replaced or gone is not.
func TestAdminAuditOfGrantInvalidation(t *testing.T) {
	for _, database := range grantTestDatabases() {
		t.Run(database.name, func(t *testing.T) {
			c := database.open(t)
			owner, mcpID := principal.PreconfiguredOAuthOwner("vmcp1test"), "vmcp1test/component"
			conf := &oauth2.Config{ClientID: "client"}
			require.NoError(t, c.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, grantTestURL, "grant-1", "1", conf, &oauth2.Token{AccessToken: "a"}))

			require.True(t, invalidateGrant(t, c, owner, mcpID, "grant-1", "token endpoint answered 400 invalid_grant"))
			require.False(t, invalidateGrant(t, c, owner, mcpID, "grant-1", "again"))
			require.False(t, invalidateGrant(t, c, owner, mcpID, "grant-other", "replaced"))

			events := adminAuditEvents(t, c)
			require.Len(t, events, 1)
			require.Equal(t, types.AdminAuditActorSystem, events[0].ActorUserID)
			require.Equal(t, "vmcp.preconfigured_oauth.invalidate", events[0].Action)
			require.Equal(t, "vmcp", events[0].ResourceKind)
			require.Equal(t, "vmcp1test", events[0].ResourceID)
			require.Empty(t, events[0].ParentID)
			require.Equal(t, map[string]any{
				"componentID": "component",
				"grantID":     "grant-1",
				"reason":      "token endpoint answered 400 invalid_grant",
			}, adminAuditDetails(t, events[0]))
		})
	}
}

// TestRecordAdminAuditEventIsBestEffort checks that a failure to record an event is not returned, for a change that has
// been made, and that the event's details are stored.
func TestRecordAdminAuditEventIsBestEffort(t *testing.T) {
	c := newTestClient(t)

	// A canceled request still has its change recorded.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	c.RecordAdminAuditEvent(ctx, AdminAuditEventInput{Actor: "1", Action: "a", ResourceKind: "agent", ResourceID: "1"})
	require.Len(t, adminAuditEvents(t, c), 1)

	// Details that cannot be stored, and a closed database, fail without a panic or an error to return.
	c.RecordAdminAuditEvent(t.Context(), AdminAuditEventInput{Actor: "1", Action: "b", Details: make(chan int)})
	require.NoError(t, c.Close())
	c.RecordAdminAuditEvent(t.Context(), AdminAuditEventInput{Actor: "1", Action: "c"})
}

// TestListAdminAuditEvents checks the filters, the order and the paging of the events.
func TestListAdminAuditEvents(t *testing.T) {
	c := newTestClient(t)
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	for i, in := range []struct {
		actor, action, kind, id string
	}{
		{"1", "agent.create", "agent", "7"},
		{"2", "agent.key.create", "apiKey", "9"},
		{"system", "vmcp.preconfigured_oauth.invalidate", "vmcp", "vmcp1"},
		{"1", "agent.delete", "agent", "7"},
	} {
		require.NoError(t, c.db.WithContext(t.Context()).Create(&types.AdminAuditEvent{
			CreatedAt:    base.Add(time.Duration(i) * time.Hour),
			ActorUserID:  in.actor,
			Action:       in.action,
			ResourceKind: in.kind,
			ResourceID:   in.id,
		}).Error)
	}
	actions := func(opts AdminAuditEventOptions) ([]string, int64) {
		t.Helper()
		events, total, err := c.ListAdminAuditEvents(t.Context(), opts)
		require.NoError(t, err)
		var result []string
		for _, event := range events {
			result = append(result, event.Action)
		}
		return result, total
	}

	got, total := actions(AdminAuditEventOptions{})
	require.Equal(t, []string{"agent.delete", "vmcp.preconfigured_oauth.invalidate", "agent.key.create", "agent.create"}, got)
	require.EqualValues(t, 4, total)
	got, _ = actions(AdminAuditEventOptions{Oldest: true})
	require.Equal(t, []string{"agent.create", "agent.key.create", "vmcp.preconfigured_oauth.invalidate", "agent.delete"}, got)

	got, total = actions(AdminAuditEventOptions{ResourceKinds: []string{"agent"}, ResourceIDs: []string{"7"}})
	require.Equal(t, []string{"agent.delete", "agent.create"}, got)
	require.EqualValues(t, 2, total)
	got, _ = actions(AdminAuditEventOptions{Actors: []string{"system"}})
	require.Equal(t, []string{"vmcp.preconfigured_oauth.invalidate"}, got)
	got, _ = actions(AdminAuditEventOptions{Actors: []string{"2", "system"}})
	require.Equal(t, []string{"vmcp.preconfigured_oauth.invalidate", "agent.key.create"}, got)
	got, _ = actions(AdminAuditEventOptions{Actions: []string{"agent.delete"}})
	require.Equal(t, []string{"agent.delete"}, got)
	got, _ = actions(AdminAuditEventOptions{Since: base.Add(time.Hour), Until: base.Add(3 * time.Hour)})
	require.Equal(t, []string{"vmcp.preconfigured_oauth.invalidate", "agent.key.create"}, got)
	got, _ = actions(AdminAuditEventOptions{ResourceKinds: []string{"nothing"}})
	require.Empty(t, got)

	// Paging keeps the total of the filter.
	got, total = actions(AdminAuditEventOptions{Limit: 2, Offset: 1})
	require.Equal(t, []string{"vmcp.preconfigured_oauth.invalidate", "agent.key.create"}, got)
	require.EqualValues(t, 4, total)
}

// TestAdminAuditRetention checks that old events are deleted with the retention of API keys, which is the longer of the
// audit logs', and are all kept when either is disabled.
func TestAdminAuditRetention(t *testing.T) {
	for _, tt := range []struct {
		name         string
		mcpDays      int
		llmDays      int
		wantRemained int
	}{
		{
			name:         "longer retention",
			mcpDays:      90,
			llmDays:      30,
			wantRemained: 2,
		},
		{
			name:         "shorter than the events",
			mcpDays:      10,
			llmDays:      5,
			wantRemained: 1,
		},
		{
			name:         "MCP retention disabled",
			mcpDays:      0,
			llmDays:      5,
			wantRemained: 3,
		},
		{
			name:         "LLM retention disabled",
			mcpDays:      5,
			llmDays:      0,
			wantRemained: 3,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			now := time.Now().UTC()
			for _, age := range []int{200, 50, 1} {
				require.NoError(t, c.db.WithContext(t.Context()).Create(&types.AdminAuditEvent{CreatedAt: now.AddDate(0, 0, -age), ActorUserID: "1", Action: "agent.create", ResourceKind: "agent", ResourceID: "1"}).Error)
			}
			require.NoError(t, c.cleanupRetainedData(t.Context(), now, tt.mcpDays, tt.llmDays))
			require.Len(t, adminAuditEvents(t, c), tt.wantRemained)
		})
	}
}

// TestAdminAuditRetentionBatches checks that more old events than a batch are all deleted.
func TestAdminAuditRetentionBatches(t *testing.T) {
	c := newTestClient(t)
	now := time.Now().UTC()
	for range 10 {
		require.NoError(t, c.db.WithContext(t.Context()).Create(&types.AdminAuditEvent{CreatedAt: now.AddDate(0, 0, -100), ActorUserID: "1", Action: "agent.create"}).Error)
	}
	require.NoError(t, c.db.WithContext(t.Context()).Create(&types.AdminAuditEvent{CreatedAt: now, ActorUserID: "1", Action: "agent.create"}).Error)
	require.NoError(t, c.deleteOldAdminAuditEvents(t.Context(), now, 30))
	require.Len(t, adminAuditEvents(t, c), 1)
}
