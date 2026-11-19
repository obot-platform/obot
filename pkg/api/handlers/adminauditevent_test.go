package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// storedAdminAuditEvents returns every event in the gateway, oldest first.
func storedAdminAuditEvents(t *testing.T, gateway *gclient.Client) []gt.AdminAuditEvent {
	t.Helper()
	events, _, err := gateway.ListAdminAuditEvents(t.Context(), gclient.AdminAuditEventOptions{Oldest: true})
	require.NoError(t, err)
	return events
}

func adminAuditEventDetails(t *testing.T, event gt.AdminAuditEvent) map[string]any {
	t.Helper()
	var details map[string]any
	require.NoError(t, json.Unmarshal(event.Details, &details))
	return details
}

func callAdminAuditEvents(t *testing.T, gateway *gclient.Client, query string) (types.AdminAuditEventList, error) {
	t.Helper()
	recorder := httptest.NewRecorder()
	if err := NewAdminAuditEventHandler().List(api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodGet, "/api/admin-audit-events?"+query, nil),
		GatewayClient:  gateway,
		User:           &user.DefaultInfo{UID: "1", Groups: types.RoleAuditor.Groups()},
	}); err != nil {
		return types.AdminAuditEventList{}, err
	}
	var list types.AdminAuditEventList
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &list), recorder.Body.String())
	return list, nil
}

// TestAgentAdminAPIWritesAdminAuditEvents checks that each route of the agent admin API that changes something writes
// one event with the calling administrator, and that the routes that change nothing write none.
func TestAgentAdminAPIWritesAdminAuditEvents(t *testing.T) {
	f := newAgentIdentityFixture(t)
	admin := strconv.Itoa(agentIdentityTestAdminID)

	agent := f.createAgent(types.AgentIdentityManifest{DisplayName: "builder"})
	f.call(http.StatusOK, http.MethodPut, "/api/agents/"+agent.ID, types.AgentIdentityManifest{DisplayName: "builder", Description: "builds"}, nil)
	key := f.createKey(agent.ID, types.AgentAPIKeyCreateRequest{Name: "ci", CanAccessLLMProxy: true})
	f.call(http.StatusOK, http.MethodPost, "/api/agents/"+agent.ID+"/disable", types.AgentIdentityDisableRequest{Reason: "paused"}, nil)
	f.call(http.StatusOK, http.MethodPost, "/api/agents/"+agent.ID+"/enable", nil, nil)
	f.call(http.StatusNoContent, http.MethodDelete, "/api/agents/"+agent.ID+"/api-keys/"+strconv.FormatUint(uint64(key.ID), 10), nil, nil)
	// Reads, and a repeat of the revocation, write nothing.
	f.call(http.StatusOK, http.MethodGet, "/api/agents", nil, nil)
	f.call(http.StatusOK, http.MethodGet, "/api/agents/"+agent.ID, nil, nil)
	f.call(http.StatusOK, http.MethodGet, "/api/agents/"+agent.ID+"/api-keys", nil, nil)
	f.call(http.StatusNoContent, http.MethodDelete, "/api/agents/"+agent.ID+"/api-keys/"+strconv.FormatUint(uint64(key.ID), 10), nil, nil)
	f.call(http.StatusOK, http.MethodPost, "/api/agents/"+agent.ID+"/enable", nil, nil)
	// A request that fails writes nothing.
	f.call(http.StatusBadRequest, http.MethodPost, "/api/agents", types.AgentIdentityManifest{}, nil)
	f.call(http.StatusNotFound, http.MethodPut, "/api/agents/9999", types.AgentIdentityManifest{DisplayName: "x"}, nil)
	f.call(http.StatusNoContent, http.MethodDelete, "/api/agents/"+agent.ID, nil, nil)
	f.call(http.StatusNotFound, http.MethodDelete, "/api/agents/"+agent.ID, nil, nil)

	var got []string
	for _, event := range storedAdminAuditEvents(t, f.gateway) {
		require.Equal(t, admin, event.ActorUserID)
		got = append(got, event.Action)
	}
	require.Equal(t, []string{
		"agent.create",
		"agent.update",
		"agent.key.create",
		"agent.disable",
		"agent.enable",
		"agent.key.revoke",
		"agent.delete",
	}, got)

	// The events are read through the API, and outlive the agent.
	list, err := callAdminAuditEvents(t, f.gateway, "resourceKind=agent&resourceId="+agent.ID)
	require.NoError(t, err)
	require.EqualValues(t, 5, list.Total)
	require.Equal(t, "agent.delete", list.Items[0].Action)
	require.Equal(t, types.AuditLogActor{ActorType: types.AuditLogActorTypeUser, ID: admin}, list.Items[0].Actor)
}

// TestAdminAuditEventsAPI checks the read API: its shape, filters and paging, and that it refuses a filter it cannot
// apply rather than ignoring it. It is not gated by the agent identities option, which the handler does not know.
func TestAdminAuditEventsAPI(t *testing.T) {
	gateway, db := newHandlerTestGatewayWithDB(t)
	require.NoError(t, db.Create(&gt.User{ID: 3, Username: "ada", HashedUsername: "ada", DisplayName: "Ada"}).Error)
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	for i, event := range []gt.AdminAuditEvent{
		{ActorUserID: "3", Action: "agent.create", ResourceKind: "agent", ResourceID: "7", Details: []byte(`{"a":1}`)},
		{ActorUserID: "3", Action: "agent.key.create", ResourceKind: "apiKey", ResourceID: "9", ParentID: "7"},
		{ActorUserID: gt.AdminAuditActorSystem, Action: "vmcp.profile.oauth.invalidate", ResourceKind: "vmcpProfile", ResourceID: "vp1", ParentID: "vmcp1"},
		{ActorUserID: "bootstrap", Action: "agent.delete", ResourceKind: "agent", ResourceID: "7"},
	} {
		event.CreatedAt = base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, db.Create(&event).Error)
	}

	list, err := callAdminAuditEvents(t, gateway, "")
	require.NoError(t, err)
	require.EqualValues(t, 4, list.Total)
	require.Equal(t, 100, list.Limit)
	require.Equal(t, 0, list.Offset)
	require.Len(t, list.Items, 4)
	// Newest first, with the actor resolved: a user, the system, and a bootstrap administrator with no user row.
	require.Equal(t, []string{"agent.delete", "vmcp.profile.oauth.invalidate", "agent.key.create", "agent.create"}, []string{list.Items[0].Action, list.Items[1].Action, list.Items[2].Action, list.Items[3].Action})
	require.Equal(t, types.AuditLogActor{ActorType: types.AuditLogActorTypeUser, ID: "bootstrap"}, list.Items[0].Actor)
	require.Equal(t, types.AuditLogActor{ActorType: types.AuditLogActorTypeSystem}, list.Items[1].Actor)
	require.Equal(t, types.AuditLogActor{ActorType: types.AuditLogActorTypeUser, ID: "3", DisplayName: "Ada"}, list.Items[2].Actor)
	require.Equal(t, "apiKey", list.Items[2].ResourceKind)
	require.Equal(t, "7", list.Items[2].ParentID)
	require.JSONEq(t, `{"a":1}`, string(list.Items[3].Details))

	for _, tt := range []struct {
		name  string
		query string
		want  []string
		total int64
	}{
		{
			name:  "kind and ID",
			query: "resourceKind=agent&resourceId=7",
			want:  []string{"agent.delete", "agent.create"},
			total: 2,
		},
		{
			name:  "actor",
			query: "actor=3",
			want:  []string{"agent.key.create", "agent.create"},
			total: 2,
		},
		{
			name:  "system actor",
			query: "actor=system",
			want:  []string{"vmcp.profile.oauth.invalidate"},
			total: 1,
		},
		{
			name:  "several kinds",
			query: "resourceKind=apiKey,vmcpProfile",
			want:  []string{"vmcp.profile.oauth.invalidate", "agent.key.create"},
			total: 2,
		},
		{
			name:  "since",
			query: "since=" + base.Add(2*time.Minute).Format(time.RFC3339),
			want:  []string{"agent.delete", "vmcp.profile.oauth.invalidate"},
			total: 2,
		},
		{
			name:  "until",
			query: "until=" + base.Add(2*time.Minute).Format(time.RFC3339),
			want:  []string{"agent.key.create", "agent.create"},
			total: 2,
		},
		{
			name:  "action",
			query: "action=agent.delete",
			want:  []string{"agent.delete"},
			total: 1,
		},
		{
			name:  "a page",
			query: "limit=1&offset=1",
			want:  []string{"vmcp.profile.oauth.invalidate"},
			total: 4,
		},
		{
			name:  "no match",
			query: "resourceKind=modelAccessPolicy",
			total: 0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			list, err := callAdminAuditEvents(t, gateway, tt.query)
			require.NoError(t, err)
			require.Equal(t, tt.total, list.Total)
			var got []string
			for _, item := range list.Items {
				got = append(got, item.Action)
			}
			require.Equal(t, tt.want, got)
		})
	}

	for _, query := range []string{"since=yesterday", "until=1", "limit=0", "limit=1001", "limit=x", "offset=-1", "offset=x"} {
		t.Run("rejects "+query, func(t *testing.T) {
			_, err := callAdminAuditEvents(t, gateway, query)
			requireHTTPStatus(t, err, http.StatusBadRequest)
		})
	}
}

// TestSubjectChangesAreAudited checks that policies write an event only for the changes that add or remove agent
// subjects, with the diff, on create, update and delete. The selector wildcard names no agent, so changing it records
// nothing.
func TestSubjectChangesAreAudited(t *testing.T) {
	gateway := newHandlerTestGateway(t)
	agent, err := gateway.CreateAgent(t.Context(), gt.Agent{DisplayName: "builder"})
	require.NoError(t, err)
	agentID := strconv.FormatUint(uint64(agent.ID), 10)

	storageClient := storage.Client(fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build())
	call := func(call func(api.Context) error, method, path, pathName, pathValue string, body any) {
		t.Helper()
		var data []byte
		if body != nil {
			var err error
			data, err = json.Marshal(body)
			require.NoError(t, err)
		}
		request := httptest.NewRequest(method, path, bytes.NewReader(data))
		if pathName != "" {
			request.SetPathValue(pathName, pathValue)
		}
		require.NoError(t, call(api.Context{
			ResponseWriter: httptest.NewRecorder(),
			Request:        request,
			Storage:        storageClient,
			GatewayClient:  gateway,
			User:           &user.DefaultInfo{UID: "5", Groups: types.RoleAdmin.Groups()},
		}))
	}
	subject := func(kind types.SubjectType, id string) types.Subject { return types.Subject{Type: kind, ID: id} }
	users := subject(types.SubjectTypeUser, "2")
	group := subject(types.SubjectTypeGroup, "eng")
	explicit := subject(types.SubjectTypeAgent, agentID)
	everyone := subject(types.SubjectTypeSelector, "*")

	// A model access policy.
	policy := func(subjects ...types.Subject) types.ModelAccessPolicyManifest {
		return types.ModelAccessPolicyManifest{DisplayName: "Models", Models: []types.ModelResource{{ID: "*"}}, Subjects: subjects}
	}
	handler := &ModelAccessPolicyHandler{}
	call(handler.Create, http.MethodPost, "/api/model-access-policies", "", "", policy(users, group))
	require.Empty(t, storedAdminAuditEvents(t, gateway), "a policy of users and groups is not agent-relevant")
	call(handler.Create, http.MethodPost, "/api/model-access-policies", "", "", policy(users, explicit))
	var policies v1.ModelAccessPolicyList
	require.NoError(t, storageClient.List(t.Context(), &policies))
	require.Len(t, policies.Items, 2)
	var created v1.ModelAccessPolicy
	for _, item := range policies.Items {
		if len(item.Spec.Manifest.Subjects) == 2 && item.Spec.Manifest.Subjects[1] == explicit {
			created = item
		}
	}
	require.NotEmpty(t, created.Name)
	events := storedAdminAuditEvents(t, gateway)
	require.Len(t, events, 1)
	require.Equal(t, "model_access_policy.subjects.update", events[0].Action)
	require.Equal(t, "modelAccessPolicy", events[0].ResourceKind)
	require.Equal(t, created.Name, events[0].ResourceID)
	require.Equal(t, "5", events[0].ActorUserID)
	require.Equal(t, map[string]any{
		"cause": "create",
		"added": []any{map[string]any{"type": "agent", "id": agentID}},
	}, adminAuditEventDetails(t, events[0]))

	// Editing user and group subjects only records nothing; swapping the agent for every user records only that the
	// agent was removed.
	call(handler.Update, http.MethodPut, "/api/model-access-policies/"+created.Name, "id", created.Name, policy(explicit, group))
	require.Len(t, storedAdminAuditEvents(t, gateway), 1)
	call(handler.Update, http.MethodPut, "/api/model-access-policies/"+created.Name, "id", created.Name, policy(everyone))
	events = storedAdminAuditEvents(t, gateway)
	require.Len(t, events, 2)
	require.Equal(t, map[string]any{
		"cause":   "update",
		"removed": []any{map[string]any{"type": "agent", "id": agentID}},
	}, adminAuditEventDetails(t, events[1]))
	// Deleting a policy for every user records nothing.
	call(handler.Delete, http.MethodDelete, "/api/model-access-policies/"+created.Name, "id", created.Name, nil)
	require.Len(t, storedAdminAuditEvents(t, gateway), 2)

	// A skill access rule for every user records nothing until it names the agent.
	rule := func(subjects ...types.Subject) types.SkillAccessRuleManifest {
		return types.SkillAccessRuleManifest{DisplayName: "Rule", Resources: []types.SkillResource{{Type: types.SkillResourceTypeSelector, ID: "*"}}, Subjects: subjects}
	}
	rules := &SkillAccessRuleHandler{}
	call(rules.Create, http.MethodPost, "/api/skill-access-rules", "", "", rule(everyone))
	var ruleList v1.SkillAccessRuleList
	require.NoError(t, storageClient.List(t.Context(), &ruleList))
	require.Len(t, ruleList.Items, 1)
	ruleID := ruleList.Items[0].Name
	require.Len(t, storedAdminAuditEvents(t, gateway), 2)
	call(rules.Update, http.MethodPut, "/api/skill-access-rules/"+ruleID, "skill_access_rule_id", ruleID, rule(explicit))
	events = storedAdminAuditEvents(t, gateway)
	require.Len(t, events, 3)
	require.Equal(t, "skill_access_rule.subjects.update", events[2].Action)
	require.Equal(t, "skillAccessRule", events[2].ResourceKind)
	require.Equal(t, ruleID, events[2].ResourceID)
	require.Equal(t, map[string]any{
		"cause": "update",
		"added": []any{map[string]any{"type": "agent", "id": agentID}},
	}, adminAuditEventDetails(t, events[2]))
	call(rules.Delete, http.MethodDelete, "/api/skill-access-rules/"+ruleID, "skill_access_rule_id", ruleID, nil)
	events = storedAdminAuditEvents(t, gateway)
	require.Len(t, events, 4)
	require.Equal(t, "delete", adminAuditEventDetails(t, events[3])["cause"])

	// A refused change records nothing.
	err = handler.Create(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/api/model-access-policies", bytes.NewReader([]byte(`{"displayName":"x","models":[{"id":"*"}],"subjects":[{"type":"agent","id":"9999"}]}`))),
		Storage:        storageClient,
		GatewayClient:  gateway,
		User:           &user.DefaultInfo{UID: "5", Groups: types.RoleAdmin.Groups()},
	})
	requireHTTPStatus(t, err, http.StatusBadRequest)
	require.Len(t, storedAdminAuditEvents(t, gateway), 4)
}

// TestProfileSubjectChangesAreAudited checks the events of the profiles of a vMCP, by profile ID: a created vMCP, a
// profile that gains and loses agent subjects, one that is added and one that is removed.
func TestProfileSubjectChangesAreAudited(t *testing.T) {
	storageClient := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	gateway := newHandlerTestGateway(t)
	handler := NewVMCPHandler(nil, "", "")
	named, err := gateway.CreateAgent(t.Context(), gt.Agent{DisplayName: "builder"})
	require.NoError(t, err)
	namedID := strconv.FormatUint(uint64(named.ID), 10)
	bot, err := gateway.CreateAgent(t.Context(), gt.Agent{DisplayName: "bot"})
	require.NoError(t, err)
	botID := strconv.FormatUint(uint64(bot.ID), 10)

	agent := types.Subject{Type: types.SubjectTypeAgent, ID: botID}
	manifest := testVMCPManifest()
	manifest.Profiles = []types.VMCPProfile{
		vmcpProfileForTest("people", vmcpEngineering),
		vmcpProfileForTest("bots", agent),
	}
	created := callVMCPCreate(t, storageClient, gateway, handler, manifest, vmcpAdminForTest())
	people, bots := created.Profiles[0].ID, created.Profiles[1].ID

	events := storedAdminAuditEvents(t, gateway)
	require.Len(t, events, 1, "only the profile that names agents is audited")
	require.Equal(t, "vmcp.profile.subjects.update", events[0].Action)
	require.Equal(t, "vmcpProfile", events[0].ResourceKind)
	require.Equal(t, bots, events[0].ResourceID)
	require.Equal(t, created.ID, events[0].ParentID)
	require.Equal(t, "admin", events[0].ActorUserID)
	require.Equal(t, map[string]any{"cause": "create", "added": []any{map[string]any{"type": "agent", "id": botID}}}, adminAuditEventDetails(t, events[0]))

	// The people profile gains every user, which names no agent and records nothing; the bots profile is removed; and a
	// new profile is added.
	edited := created.VMCPManifest
	edited.Profiles = []types.VMCPProfile{created.Profiles[0], vmcpProfileForTest("new", types.Subject{Type: types.SubjectTypeAgent, ID: namedID})}
	edited.Profiles[0].Subjects = []types.Subject{vmcpEngineering, vmcpEveryone}
	updated, err := callVMCPUpdate(t, storageClient, gateway, handler, created.ID, edited)
	require.NoError(t, err)
	newID := updated.Profiles[1].ID

	events = storedAdminAuditEvents(t, gateway)
	require.Len(t, events, 3)
	byProfile := map[string]gt.AdminAuditEvent{}
	for _, event := range events[1:] {
		require.Equal(t, "update", adminAuditEventDetails(t, event)["cause"])
		byProfile[event.ResourceID] = event
	}
	require.NotContains(t, byProfile, people)
	require.Equal(t, []any{map[string]any{"type": "agent", "id": namedID}}, adminAuditEventDetails(t, byProfile[newID])["added"])
	require.Equal(t, []any{map[string]any{"type": "agent", "id": botID}}, adminAuditEventDetails(t, byProfile[bots])["removed"])
	require.NotContains(t, adminAuditEventDetails(t, byProfile[bots]), "added")

	// An edit that changes no agent-relevant subject records nothing.
	edited = updated.VMCPManifest
	edited.Profiles[0].Name = "renamed"
	_, err = callVMCPUpdate(t, storageClient, gateway, handler, created.ID, edited)
	require.NoError(t, err)
	require.Len(t, storedAdminAuditEvents(t, gateway), 3)
}
