package mcp

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func binding(secretName, key string) *types.MCPSecretBinding {
	return &types.MCPSecretBinding{Name: secretName, Key: key}
}

func TestMergeBoundCreds(t *testing.T) {
	const ns = "obot-ns"

	newClient := func(t *testing.T, objects ...kclient.Object) kclient.Client {
		t.Helper()
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	}

	t.Run("does not mutate input cred map and overrides stale values", func(t *testing.T) {
		manifestEnv := []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "API_KEY", SecretBinding: binding("bound-secret", "api_key")}}}
		input := map[string]string{"API_KEY": "stale", "UNCHANGED": "keep"}
		inputBefore := map[string]string{"API_KEY": "stale", "UNCHANGED": "keep"}

		c := newClient(t, &corev1.Secret{
			Data:       map[string][]byte{"api_key": []byte("fresh")},
			ObjectMeta: metav1.ObjectMeta{Name: "bound-secret", Namespace: ns},
		})

		out, err := MergeBoundCreds(context.Background(), c, ns, manifestEnv, nil, input)
		require.NoError(t, err)
		assert.Equal(t, inputBefore, input)
		assert.Equal(t, "fresh", out["API_KEY"])
		assert.Equal(t, "keep", out["UNCHANGED"])
	})

	t.Run("omits missing secret and missing key", func(t *testing.T) {
		manifestEnv := []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "API_KEY", SecretBinding: binding("missing-secret", "api_key")}}}
		input := map[string]string{"API_KEY": "stale", "OTHER": "ok"}

		out, err := MergeBoundCreds(context.Background(), newClient(t), ns, manifestEnv, nil, input)
		require.NoError(t, err)
		assert.NotContains(t, out, "API_KEY")
		assert.Equal(t, "ok", out["OTHER"])

		manifestEnv[0].SecretBinding = binding("present-secret", "missing-key")
		c := newClient(t, &corev1.Secret{
			Data:       map[string][]byte{"other": []byte("x")},
			ObjectMeta: metav1.ObjectMeta{Name: "present-secret", Namespace: ns},
		})
		out, err = MergeBoundCreds(context.Background(), c, ns, manifestEnv, nil, input)
		require.NoError(t, err)
		assert.NotContains(t, out, "API_KEY")
	})

	t.Run("merges remote header bindings", func(t *testing.T) {
		remote := &types.RemoteRuntimeConfig{Headers: []types.MCPHeader{{Key: "Authorization", SecretBinding: binding("auth-secret", "token")}}}
		c := newClient(t, &corev1.Secret{
			Data:       map[string][]byte{"token": []byte("Bearer abc")},
			ObjectMeta: metav1.ObjectMeta{Name: "auth-secret", Namespace: ns},
		})

		out, err := MergeBoundCreds(context.Background(), c, ns, nil, remote, map[string]string{"Authorization": "stale"})
		require.NoError(t, err)
		assert.Equal(t, "Bearer abc", out["Authorization"])
	})

	t.Run("nil client strips bound keys and keeps others", func(t *testing.T) {
		manifestEnv := []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "API_KEY", SecretBinding: binding("s", "k")}}}
		remote := &types.RemoteRuntimeConfig{Headers: []types.MCPHeader{{Key: "Authorization", SecretBinding: binding("s", "token")}}}
		in := map[string]string{"API_KEY": "x", "Authorization": "y", "OTHER": "ok"}

		out, err := MergeBoundCreds(context.Background(), nil, ns, manifestEnv, remote, in)
		require.NoError(t, err)
		assert.NotContains(t, out, "API_KEY")
		assert.NotContains(t, out, "Authorization")
		assert.Equal(t, "ok", out["OTHER"])
	})

	t.Run("empty secret value is treated as missing", func(t *testing.T) {
		manifestEnv := []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "API_KEY", SecretBinding: binding("bound-secret", "api_key")}}}
		in := map[string]string{"API_KEY": "stale"}
		c := newClient(t, &corev1.Secret{
			Data:       map[string][]byte{"api_key": []byte("")},
			ObjectMeta: metav1.ObjectMeta{Name: "bound-secret", Namespace: ns},
		})

		out, err := MergeBoundCreds(context.Background(), c, ns, manifestEnv, nil, in)
		require.NoError(t, err)
		assert.NotContains(t, out, "API_KEY")
	})
}
