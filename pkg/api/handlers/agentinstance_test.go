package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClaudeSessionsAllParameter(t *testing.T) {
	for _, tc := range []struct {
		name   string
		groups []string
		query  string
		count  int
	}{
		{
			name:   "admin all",
			groups: []string{types.GroupAdmin},
			query:  "?all=true",
			count:  1,
		},
		{
			name:   "admin personal",
			groups: []string{types.GroupAdmin},
			count:  0,
		},
		{
			name:   "user all ignored",
			groups: []string{types.GroupBasic},
			query:  "?all=true",
			count:  0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instance := &v1.AgentInstance{
				Spec: v1.AgentInstanceSpec{
					UserID:        "other",
					ClaudeSession: "session",
					ClaudeOrder:   "order",
				},
				Status: v1.AgentInstanceStatus{State: "SUSPENDED"},
			}
			instance.Name = "other-session"
			instance.Namespace = system.DefaultNamespace

			storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).
				WithObjects(instance).WithIndex(&v1.AgentInstance{}, "spec.userID", func(obj kclient.Object) []string {
				return []string{obj.(*v1.AgentInstance).Spec.UserID}
			}).Build()
			response := httptest.NewRecorder()
			request := api.Context{
				Request:        httptest.NewRequest(http.MethodGet, "/api/claude-sessions"+tc.query, nil),
				ResponseWriter: response,
				Storage:        storage,
				User:           &kuser.DefaultInfo{UID: "viewer", Groups: tc.groups},
			}

			require.NoError(t, (&ClaudeSessionHandler{}).List(request))
			var result v1.AgentInstanceList
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			require.Len(t, result.Items, tc.count)
			if tc.count > 0 {
				require.Equal(t, "SUSPENDED", result.Items[0].Status.State)
			}
		})
	}
}

func TestSuspendClaudeSessionRequiresAdmin(t *testing.T) {
	req := api.Context{User: &kuser.DefaultInfo{UID: "user", Groups: []string{types.GroupBasic}}}
	require.ErrorContains(t, (&ClaudeSessionHandler{}).Suspend(req), "Administrator access required")
}

func TestSuspendLatestClaudeExecution(t *testing.T) {
	old := &v1.AgentInstance{
		Spec: v1.AgentInstanceSpec{
			UserID:        "other",
			ClaudeSession: "session",
			ClaudeOrder:   "old-order",
			ClaudeDone:    true,
		},
		Status: v1.AgentInstanceStatus{State: "SUSPENDED"},
	}
	old.Name = "a-old"
	old.Namespace = system.DefaultNamespace
	latest := old.DeepCopy()
	latest.Name = "b-new"
	latest.Spec.ClaudeOrder = "new-order"
	latest.Spec.ClaudeDone = false
	latest.Status.State = "RUNNING"
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(old, latest).Build()
	request := httptest.NewRequest(http.MethodPost, "/api/claude-sessions/session/suspend", nil)
	request.SetPathValue("session_id", agentSessionID(latest))
	response := httptest.NewRecorder()
	req := api.Context{
		Request:        request,
		ResponseWriter: response,
		Storage:        storage,
		User:           &kuser.DefaultInfo{UID: "admin", Groups: []string{types.GroupAdmin}},
	}
	handler := &ClaudeSessionHandler{}
	require.NoError(t, handler.suspend(req))
	require.Equal(t, http.StatusAccepted, response.Code)
	require.NoError(t, handler.suspend(req))
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(latest), latest))
	require.True(t, latest.Spec.Suspended)
	require.False(t, latest.Spec.ClaudeDone, "the runner must checkpoint before it is marked done")
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(old), old))
	require.False(t, old.Spec.Suspended, "historical attempts must not be changed")
}
