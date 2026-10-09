package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/api"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/substrate"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClaudeRunnerFinishedRecordsRetentionStart(t *testing.T) {
	gateway := newHandlerTestGateway(t)
	instance := &v1.AgentInstance{
		Spec: v1.AgentInstanceSpec{
			ClaudeOrder: "order",
		},
	}
	instance.Name = "runner"
	instance.Namespace = system.DefaultNamespace
	instance.CreationTimestamp = metav1.NewTime(time.Now().Add(-31 * 24 * time.Hour))
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(instance).Build()
	require.NoError(t, gateway.SaveClaudeWorkOrder(t.Context(), gt.Credential{
		Context: "claude-runner",
		Name:    instance.Name,
		Secrets: map[string]string{"token": "runner-secret"},
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/claude-runners/runner/finished", strings.NewReader(`{"checkpoint":true,"reason":"completed"}`))
	request.SetPathValue("instance", instance.Name)
	request.Header.Set("X-Obot-Runner-Token", "runner-secret")
	response := httptest.NewRecorder()
	handler := &ClaudeRunnerHandler{Substrate: &substrate.Client{}}
	apiRequest := api.Context{Request: request, ResponseWriter: response, Storage: storage, GatewayClient: gateway}
	before := time.Now().Truncate(time.Second)

	require.NoError(t, handler.finished(apiRequest, true))
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(instance), instance))
	require.True(t, instance.Spec.ClaudeDone)
	require.False(t, instance.Spec.ClaudeFinishedAt.Time.Before(before))
	require.WithinDuration(t, time.Now(), instance.Spec.ClaudeFinishedAt.Time, 5*time.Second)
	require.Equal(t, http.StatusNoContent, response.Code)

	// Rejected duplicate callbacks must not keep extending retention.
	finishedAt := instance.Spec.ClaudeFinishedAt
	require.ErrorContains(t, handler.finished(apiRequest, true), "no longer active")
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(instance), instance))
	require.Equal(t, finishedAt, instance.Spec.ClaudeFinishedAt)
}

func TestSanitizeClaudeRepositories(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
		want string
	}{
		{
			name: "strip credentials and query",
			url:  "https://user:secret@github.com/obot-platform/obot.git?token=secret#fragment",
			want: "https://github.com/obot-platform/obot.git",
		},
		{
			name: "SSH remote",
			url:  "git@github.com:obot-platform/obot.git",
			want: "ssh://github.com/obot-platform/obot.git",
		},
		{
			name: "reject non-repository scheme",
			url:  "javascript:alert(1)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repositories := []v1.ClaudeSessionRepository{{URL: tc.url, Revision: "main"}}
			err := sanitizeClaudeRepositories(repositories)
			if tc.want == "" {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, repositories[0].URL)
			require.Equal(t, "main", repositories[0].Revision)
		})
	}
}
