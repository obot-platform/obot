package adminmcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSecretBindingsMatchUIWithoutValues(t *testing.T) {
	const (
		namespace = "obot"
		label     = "obot.example/bindable"
		value     = "secret-value-must-never-appear"
	)
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	secret := func(name, ns string, labels map[string]string, data map[string][]byte) *corev1.Secret {
		return &corev1.Secret{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
			Data:      data,
		}
	}
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		secret("allowed", namespace, map[string]string{label: ""}, map[string][]byte{
			"token": []byte(value),
			"empty": {},
			"nil":   nil,
		}),
		secret("empty", namespace, map[string]string{label: "true"}, map[string][]byte{"empty": {}}),
		secret("unlabeled", namespace, nil, map[string][]byte{"token": []byte(value)}),
		secret("other-namespace", "elsewhere", map[string]string{label: "true"}, map[string][]byte{"token": []byte(value)}),
	).Build()
	h := NewHandler(testBaseURL, mcp.RuntimeBackendKubernetes, label, nil, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Builtin(api.Context{
			Request:        r,
			ResponseWriter: w,
			User:           vmcpUser(types.GroupAdmin),
			LocalK8sClient: k8s,
			ObotNamespace:  namespace,
		}).ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	session := connect(t, srv.URL, nil)
	out, result := callTool[listSecretBindingsOutput](t, session, toolListSecretBindings, map[string]any{})
	require.False(t, result.IsError, resultText(result))
	require.True(t, out.Available)
	require.Equal(t, []types.MCPAllowedSecretBindingTarget{{Name: "allowed", Keys: []string{"token"}}}, out.Secrets)
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(data), value)

	recorder := httptest.NewRecorder()
	ui := handlers.NewMCPSecretBindingHandler(mcp.RuntimeBackendKubernetes, k8s, namespace, label)
	require.NoError(t, ui.ListAllowedSecrets(api.Context{
		Request:        httptest.NewRequest(http.MethodGet, "/api/mcp-server-binding-secrets", nil),
		ResponseWriter: recorder,
	}))
	var uiResult types.MCPAllowedSecretBindingTargetList
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &uiResult))
	require.Equal(t, uiResult.Items, out.Secrets)
	require.NotContains(t, recorder.Body.String(), value)
}

func TestSecretBindingsUnavailableWithoutKubernetes(t *testing.T) {
	for _, backend := range []string{"docker", mcp.RuntimeBackendKubernetes} {
		t.Run(backend, func(t *testing.T) {
			tool := &tools{handler: NewHandler(testBaseURL, backend, "", nil, nil)}
			_, out, err := tool.listSecretBindings(t.Context(), nil, listSecretBindingsInput{})
			require.NoError(t, err)
			require.False(t, out.Available)
			require.Empty(t, out.Secrets)
		})
	}
}
