package handlers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"strings"

	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/clauderunner"
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

func TestClaudeTagCredentials(t *testing.T) {
	gateway := newHandlerTestGateway(t)
	ctx := t.Context()
	environment := &gt.ClaudeEnvironment{
		ID:               "environment",
		EnvironmentID:    "ccpool_test",
		OrganizationID:   "org_test",
		ClaudeTagEnabled: true,
	}
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, environment, nil))
	require.NoError(t, gateway.EnsureClaudeTagUser(ctx, environment))
	environment.Profiles = []gt.ClaudeEnvironmentProfile{{
		Name:         "service",
		MCPServerIDs: []string{"vmcp_test"},
		Subjects:     []types.Subject{{Type: types.SubjectTypeUser, ID: environment.ClaudeTagUserID}},
	}}
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, environment, nil))
	instance := &v1.AgentInstance{
		Spec: v1.AgentInstanceSpec{
			UserID:            environment.ClaudeTagUserID,
			ClaudeEnvironment: environment.ID,
			ClaudeAccount:     clauderunner.ClaudeTagAccount,
			ClaudeOrder:       "order",
			ClaudeSession:     "session_test",
		},
	}
	instance.Name = "runner"
	instance.Namespace = system.DefaultNamespace
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(instance).Build()
	require.NoError(t, gateway.SaveClaudeWorkOrder(ctx, gt.Credential{
		Context: "claude-runner",
		Name:    instance.Name,
		Secrets: map[string]string{"token": "runner-secret"},
	}))

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	point, err := key.PublicKey.Bytes()
	require.NoError(t, err)
	jwks, err := json.Marshal(map[string]any{"keys": []map[string]string{{
		"kty": "EC", "crv": "P-256", "kid": "test",
		"x": base64.RawURLEncoding.EncodeToString(point[1:33]),
		"y": base64.RawURLEncoding.EncodeToString(point[33:]),
	}}})
	require.NoError(t, err)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = mcpTesterRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "https://api.anthropic.com/v1/code/.well-known/jwks.json", req.URL.String())
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(jwks)))}, nil
	})
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss":            "ccr",
		"sub":            "ccr:session:cse_test",
		"aud":            []string{environment.EnvironmentID},
		"exp":            time.Now().Add(time.Hour).Unix(),
		"ccr:role":       "session_worker",
		"ccr:pool_id":    environment.EnvironmentID,
		"ccr:org_id":     environment.OrganizationID,
		"ccr:session_id": "cse_test",
		"act":            map[string]string{"sub": "agent:tag-test"},
	})
	token.Header["kid"] = "test"
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/claude-runners/runner/credentials", nil)
	request.SetPathValue("instance", instance.Name)
	request.Header.Set("X-Obot-Runner-Token", "runner-secret")
	request.Header.Set("X-Claude-Session-Token", "sk-ant-cc-"+signed)
	response := httptest.NewRecorder()
	handler := &ClaudeRunnerHandler{Substrate: &substrate.Client{Config: substrate.Config{ObotURL: "http://obot"}}}
	apiRequest := api.Context{Request: request, ResponseWriter: response, Storage: storage, GatewayClient: gateway}
	require.NoError(t, handler.credentials(apiRequest))
	var result struct {
		MCPServers map[string]struct{ Headers map[string]string }
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Equal(t, map[string]string{
		"Authorization":     "Bearer obot-egress-placeholder",
		"X-Obot-Egress-Key": "placeholder",
	}, result.MCPServers["vmcp_test"].Headers)
	_, err = gateway.RevealCredential(ctx, []string{"substrate-agent"}, instance.Name)
	require.Error(t, err, "the runner configuration must not mint or expose an API key")

	// A live session loses access when the administrator disables Claude Tag.
	environment.ClaudeTagEnabled = false
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, environment, nil))
	require.ErrorContains(t, handler.credentials(apiRequest), "access has been removed")
}

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

func TestClaudeRunnerSlackMetadata(t *testing.T) {
	gateway := newHandlerTestGateway(t)
	instance := &v1.AgentInstance{
		Name: "runner", Namespace: system.DefaultNamespace,
		Spec: v1.AgentInstanceSpec{ClaudeOrder: "order", ClaudeAccount: clauderunner.ClaudeTagAccount},
	}
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(instance).Build()
	require.NoError(t, gateway.SaveClaudeWorkOrder(t.Context(), gt.Credential{
		Context: "claude-runner",
		Name:    instance.Name,
		Secrets: map[string]string{"token": "runner-secret"},
	}))
	handler := &ClaudeRunnerHandler{Substrate: &substrate.Client{}}
	for _, body := range []string{
		`{"slackSenderName":"Donnie","slackMentioned":false,"slackThreadTimestamp":"1791829233.654919","slackLink":"https://acorn-io.slack.com/archives/C123/p1791830531603449"}`,
		`{"slackSenderName":"Someone else","slackSenderHandle":"thedadams","slackChannelName":"craig-test","slackMentioned":true,"slackThreadTimestamp":"1.2","slackLink":"https://acorn-io.slack.com/archives/C456/p12"}`,
		`{}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.SetPathValue("instance", instance.Name)
		request.Header.Set("X-Obot-Runner-Token", "runner-secret")
		var input v1.AgentInstanceSpec
		require.NoError(t, json.Unmarshal([]byte(body), &input))
		require.NoError(t, handler.status(api.Context{Request: request, ResponseWriter: httptest.NewRecorder(), Storage: storage, GatewayClient: gateway}, input))
	}
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(instance), instance))
	require.Equal(t, "Donnie", instance.Spec.SlackSenderName)
	require.Equal(t, "thedadams", instance.Spec.SlackSenderHandle)
	require.Equal(t, "craig-test", instance.Spec.SlackChannelName)
	require.NotNil(t, instance.Spec.SlackMentioned)
	require.False(t, *instance.Spec.SlackMentioned)

	require.Equal(t, "1791829233.654919", instance.Spec.SlackThreadTimestamp)
	require.Equal(t, "https://acorn-io.slack.com/archives/C123/p1791830531603449", instance.Spec.SlackLink)

	resumed := v1.AgentInstanceSpec{}
	require.True(t, mergeSlackMetadata(&resumed, instance.Spec))
	require.Equal(t, instance.Spec.SlackSenderName, resumed.SlackSenderName)
	require.False(t, *resumed.SlackMentioned)
	require.Equal(t, instance.Spec.SlackThreadTimestamp, resumed.SlackThreadTimestamp)
	require.Equal(t, instance.Spec.SlackLink, resumed.SlackLink)
	require.NotSame(t, instance.Spec.SlackMentioned, resumed.SlackMentioned)

	for _, body := range []string{
		`{"slackThreadTimestamp":"not-a-timestamp"}`,
		`{"slackLink":"javascript:alert(1)"}`,
		`{"slackLink":"https://slack.com.evil.example/archives/C1/p12"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.SetPathValue("instance", instance.Name)
		request.Header.Set("X-Obot-Runner-Token", "runner-secret")
		require.Error(t, handler.Status(api.Context{Request: request, ResponseWriter: httptest.NewRecorder(), Storage: storage, GatewayClient: gateway}))
	}
}
