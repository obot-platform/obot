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

func TestListAllowedSecretBindingTargets(t *testing.T) {
	const ns = "obot-ns"
	const label = DefaultSecretBindingAllowedLabel

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "z-secret", Namespace: ns, Labels: map[string]string{label: "false"}},
			Data:       map[string][]byte{"b": []byte("value-b"), "a": []byte("value-a")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "a-secret", Namespace: ns, Labels: map[string]string{label: "true"}},
			Data:       map[string][]byte{"token": []byte("secret-value")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: ns, Labels: map[string]string{label: "true"}},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "unlabeled", Namespace: ns},
			Data:       map[string][]byte{"token": []byte("hidden")},
		},
	).Build()

	targets, err := ListAllowedSecretBindingTargets(context.Background(), c, ns, label)
	require.NoError(t, err)
	assert.Equal(t, []types.MCPAllowedSecretBindingTarget{
		{Name: "a-secret", Keys: []string{"token"}},
		{Name: "z-secret", Keys: []string{"a", "b"}},
	}, targets)
}

func TestValidateAllowedSecretBindings(t *testing.T) {
	const ns = "obot-ns"
	const label = DefaultSecretBindingAllowedLabel

	newClient := func(t *testing.T, objects ...kclient.Object) kclient.Client {
		t.Helper()
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	}

	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeContainerized,
		Env: []types.MCPEnv{{
			MCPHeader: types.MCPHeader{Key: "API_KEY", SecretBinding: binding("allowed", "token")},
		}},
	}

	t.Run("validates allowed secret and key", func(t *testing.T) {
		c := newClient(t, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "allowed", Namespace: ns, Labels: map[string]string{label: ""}},
			Data:       map[string][]byte{"token": []byte("value")},
		})

		require.NoError(t, ValidateAllowedSecretBindings(context.Background(), c, ns, manifest, label))
	})

	t.Run("requires label", func(t *testing.T) {
		c := newClient(t, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "allowed", Namespace: ns},
			Data:       map[string][]byte{"token": []byte("value")},
		})

		err := ValidateAllowedSecretBindings(context.Background(), c, ns, manifest, label)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not allowed for binding")
	})

	t.Run("requires key", func(t *testing.T) {
		c := newClient(t, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "allowed", Namespace: ns, Labels: map[string]string{label: "true"}},
			Data:       map[string][]byte{"other": []byte("value")},
		})

		err := ValidateAllowedSecretBindings(context.Background(), c, ns, manifest, label)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key \"token\" was not found")
	})

	t.Run("requires secret", func(t *testing.T) {
		err := ValidateAllowedSecretBindings(context.Background(), newClient(t), ns, manifest, label)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "was not found")
	})
}
