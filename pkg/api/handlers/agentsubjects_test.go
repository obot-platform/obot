package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// TestAgentIdentityAccess checks the access summary of an agent: the shared vMCP profiles, model access policies and
// skill access rules that match it, with how they match, and none that only name users, groups, another agent, or
// the user that has the agent's numeric ID.
func TestAgentIdentityAccess(t *testing.T) {
	f := newAgentIdentityFixture(t, true, true)
	agent := f.createAgent(types.AgentIdentityManifest{DisplayName: "builder"})
	other := f.createAgent(types.AgentIdentityManifest{DisplayName: "other"})

	everyone := types.Subject{Type: types.SubjectTypeSelector, ID: "*"}
	agents := types.Subject{Type: types.SubjectTypeAgent, ID: "*"}
	users := types.Subject{Type: types.SubjectTypeUser, ID: "*"}
	explicit := types.Subject{Type: types.SubjectTypeAgent, ID: agent.ID}
	otherAgent := types.Subject{Type: types.SubjectTypeAgent, ID: other.ID}
	// The user whose ID is the agent's numeric ID.
	sameIDUser := types.Subject{Type: types.SubjectTypeUser, ID: agent.ID}
	group := types.Subject{Type: types.SubjectTypeGroup, ID: "eng"}
	admins := types.Subject{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin}

	for _, obj := range []kclient.Object{
		&v1.VMCP{
			Name:      "vmcp1a",
			Namespace: system.DefaultNamespace,
			Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
				DisplayName: "A",
				Profiles: []types.VMCPProfile{
					{Name: "everyone", Subjects: []types.Subject{everyone}},
					{Name: "users", Subjects: []types.Subject{users, group, admins, sameIDUser}},
					{Name: "agents", Subjects: []types.Subject{agents}},
					{Name: "mine", Subjects: []types.Subject{everyone, explicit}},
					{Name: "other", Subjects: []types.Subject{otherAgent}},
				},
			}},
		},
		&v1.VMCP{
			Name:      "vmcp1personal",
			Namespace: system.DefaultNamespace,
			Spec: v1.VMCPSpec{
				UserID: "1",
				Manifest: types.VMCPManifest{
					DisplayName: "Personal",
					Profiles:    []types.VMCPProfile{{Name: "everyone", Subjects: []types.Subject{everyone}}},
				},
			},
		},
		&v1.ModelAccessPolicy{
			Name:      "map1everyone",
			Namespace: system.DefaultNamespace,
			Spec:      v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{DisplayName: "Everyone", Subjects: []types.Subject{everyone}}},
		},
		&v1.ModelAccessPolicy{
			Name:      "map1mine",
			Namespace: system.DefaultNamespace,
			Spec:      v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{DisplayName: "Mine", Subjects: []types.Subject{explicit}}},
		},
		&v1.ModelAccessPolicy{
			Name:      "map1users",
			Namespace: system.DefaultNamespace,
			Spec:      v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{DisplayName: "Users", Subjects: []types.Subject{sameIDUser, group}}},
		},
		&v1.SkillAccessRule{
			Name:      "sar1agents",
			Namespace: system.DefaultNamespace,
			Spec:      v1.SkillAccessRuleSpec{Manifest: types.SkillAccessRuleManifest{DisplayName: "Agents", Subjects: []types.Subject{agents}}},
		},
		&v1.SkillAccessRule{
			Name:      "sar1other",
			Namespace: system.DefaultNamespace,
			Spec:      v1.SkillAccessRuleSpec{Manifest: types.SkillAccessRuleManifest{DisplayName: "Other", Subjects: []types.Subject{otherAgent}}},
		},
	} {
		require.NoError(t, f.storage.Create(t.Context(), obj))
	}

	var access types.AgentIdentityAccess
	f.call(http.StatusOK, http.MethodGet, "/api/agents/"+agent.ID+"/access", nil, &access)
	require.Equal(t, types.AgentIdentityAccess{
		AgentID: agent.ID,
		VMCPProfiles: []types.AgentIdentityVMCPProfileAccess{
			{VMCPID: "vmcp1a", VMCPDisplayName: "A", ProfileName: "everyone", Match: types.SubjectMatchWildcard},
			{VMCPID: "vmcp1a", VMCPDisplayName: "A", ProfileName: "agents", Match: types.SubjectMatchKindWildcard},
			{VMCPID: "vmcp1a", VMCPDisplayName: "A", ProfileName: "mine", Match: types.SubjectMatchExplicit},
		},
		ModelAccessPolicies: []types.AgentIdentityPolicyAccess{
			{ID: "map1everyone", DisplayName: "Everyone", Match: types.SubjectMatchWildcard},
			{ID: "map1mine", DisplayName: "Mine", Match: types.SubjectMatchExplicit},
		},
		SkillAccessRules: []types.AgentIdentityPolicyAccess{
			{ID: "sar1agents", DisplayName: "Agents", Match: types.SubjectMatchKindWildcard},
		},
		Problems: []types.AgentIdentityAccessProblem{},
	}, access)

	// A deleted agent has no access summary.
	f.call(http.StatusNoContent, http.MethodDelete, "/api/agents/"+other.ID, nil, nil)
	f.call(http.StatusNotFound, http.MethodGet, "/api/agents/"+other.ID+"/access", nil, nil)
	f.call(http.StatusNotFound, http.MethodGet, "/api/agents/999/access", nil, nil)
}

// TestCheckNewAgentSubjects checks that a policy write may add an explicit agent subject only for an agent that
// exists and is not deleted, while subjects it already had, and the agent wildcard, are not looked up.
func TestCheckNewAgentSubjects(t *testing.T) {
	f := newAgentIdentityFixture(t, true, true)
	active := f.createAgent(types.AgentIdentityManifest{DisplayName: "active"})
	disabled := f.createAgent(types.AgentIdentityManifest{DisplayName: "disabled"})
	deleted := f.createAgent(types.AgentIdentityManifest{DisplayName: "deleted"})
	f.call(http.StatusOK, http.MethodPost, "/api/agents/"+disabled.ID+"/disable", nil, nil)
	f.call(http.StatusNoContent, http.MethodDelete, "/api/agents/"+deleted.ID, nil, nil)

	agent := func(id string) types.Subject { return types.Subject{Type: types.SubjectTypeAgent, ID: id} }
	req := api.Context{Request: (&http.Request{}).WithContext(t.Context()), GatewayClient: f.gateway}

	for _, tt := range []struct {
		name      string
		subjects  []types.Subject
		previous  []types.Subject
		wantError string
	}{
		{
			name:     "active agent",
			subjects: []types.Subject{agent(active.ID)},
		},
		{
			name:     "disabled agent",
			subjects: []types.Subject{agent(disabled.ID)},
		},
		{
			name:      "deleted agent",
			subjects:  []types.Subject{agent(active.ID), agent(deleted.ID)},
			wantError: "agent \"" + deleted.ID + "\" not found",
		},
		{
			name:      "unknown agent",
			subjects:  []types.Subject{agent("999")},
			wantError: "agent \"999\" not found",
		},
		{
			name:     "deleted agent the policy already named",
			subjects: []types.Subject{agent(deleted.ID)},
			previous: []types.Subject{agent(deleted.ID)},
		},
		{
			name:     "agent wildcard",
			subjects: []types.Subject{agent("*")},
		},
		{
			// A user with the ID of a deleted agent is not an agent subject.
			name:     "user subject",
			subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: deleted.ID}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkNewAgentSubjects(req, tt.subjects, tt.previous)
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantError)
			var httpErr *types.ErrHTTP
			require.ErrorAs(t, err, &httpErr)
			require.Equal(t, http.StatusBadRequest, httpErr.Code)
		})
	}
}

// TestGuardrailFilterAgentSubjectsMustExist checks that creating or updating a guardrail filter refuses a newly
// named agent that does not exist or is deleted, and that an agent the filter already named is not looked up again.
func TestGuardrailFilterAgentSubjectsMustExist(t *testing.T) {
	f := newAgentIdentityFixture(t, true, true)
	active := f.createAgent(types.AgentIdentityManifest{DisplayName: "active"})
	deleted := f.createAgent(types.AgentIdentityManifest{DisplayName: "deleted"})
	f.call(http.StatusNoContent, http.MethodDelete, "/api/agents/"+deleted.ID, nil, nil)
	guardrail := &gatewaytypes.Guardrail{Name: "secrets", Type: "secrets"}
	require.NoError(t, f.gateway.CreateGuardrail(t.Context(), guardrail, nil))

	handler := NewMCPWebhookValidationHandler(&mcp.SessionManager{})
	call := func(method string, handle api.HandlerFunc, id string, subjects ...types.MCPGuardrailFilterSubject) error {
		data, err := json.Marshal(types.MCPWebhookValidationManifest{
			Name: "filter",
			Guardrail: &types.MCPGuardrailFilterConfig{
				GuardrailIDs: []string{guardrail.ID},
				Action:       types.MCPGuardrailActionBlock,
				Subjects:     subjects,
			},
		})
		require.NoError(t, err)
		req := httptest.NewRequest(method, "/api/mcp-webhook-validations/"+id, bytes.NewReader(data))
		req.SetPathValue("mcp_webhook_validation_id", id)
		return handle(api.Context{
			ResponseWriter: httptest.NewRecorder(),
			Request:        req,
			GatewayClient:  f.gateway,
			Storage:        f.storage,
		})
	}
	agent := func(id string) types.MCPGuardrailFilterSubject {
		return types.MCPGuardrailFilterSubject{Type: types.MCPGuardrailSubjectAgent, ID: id}
	}
	requireBadRequest := func(err error, want string) {
		t.Helper()
		require.ErrorContains(t, err, want)
		var httpErr *types.ErrHTTP
		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, http.StatusBadRequest, httpErr.Code)
	}

	requireBadRequest(call(http.MethodPost, handler.Create, "", agent(deleted.ID)), "agent \""+deleted.ID+"\" not found")
	requireBadRequest(call(http.MethodPost, handler.Create, "", agent("999")), "agent \"999\" not found")
	var filters v1.MCPWebhookValidationList
	require.NoError(t, f.storage.List(t.Context(), &filters))
	require.Empty(t, filters.Items, "a refused filter must not be created")

	require.NoError(t, call(http.MethodPost, handler.Create, "", agent(active.ID), agent("*")))
	require.NoError(t, f.storage.List(t.Context(), &filters))
	require.Len(t, filters.Items, 1)
	id := filters.Items[0].Name

	requireBadRequest(call(http.MethodPut, handler.Update, id, agent(active.ID), agent(deleted.ID)), "agent \""+deleted.ID+"\" not found")

	// An agent the filter already names is kept on an unrelated edit, even once it is deleted.
	f.call(http.StatusNoContent, http.MethodDelete, "/api/agents/"+active.ID, nil, nil)
	require.NoError(t, call(http.MethodPut, handler.Update, id, agent(active.ID)))
}
