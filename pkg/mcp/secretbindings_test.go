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

func secret(data map[string]string) *corev1.Secret {
	s := &corev1.Secret{Data: make(map[string][]byte, len(data))}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

func binding(secretName, key string) *types.MCPSecretBinding {
	return &types.MCPSecretBinding{Name: secretName, Key: key}
}

func TestHashReferencedKeys(t *testing.T) {
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeNPX,
		Env: []types.MCPEnv{
			{MCPHeader: types.MCPHeader{Key: "API_KEY", SecretBinding: binding("my-secret", "api_key")}},
			{MCPHeader: types.MCPHeader{Key: "PLAIN"}}, // no binding
		},
	}

	t.Run("nil secret returns empty string", func(t *testing.T) {
		h := HashReferencedKeys(manifest, "my-secret", nil)
		assert.Equal(t, "", h)
	})

	t.Run("same referenced value produces same hash", func(t *testing.T) {
		s := secret(map[string]string{"api_key": "token123", "other_key": "irrelevant"})
		h1 := HashReferencedKeys(manifest, "my-secret", s)
		h2 := HashReferencedKeys(manifest, "my-secret", s)
		require.NotEmpty(t, h1)
		assert.Equal(t, h1, h2)
	})

	t.Run("changing referenced key changes hash", func(t *testing.T) {
		s1 := secret(map[string]string{"api_key": "token-v1"})
		s2 := secret(map[string]string{"api_key": "token-v2"})
		h1 := HashReferencedKeys(manifest, "my-secret", s1)
		h2 := HashReferencedKeys(manifest, "my-secret", s2)
		assert.NotEqual(t, h1, h2)
	})

	t.Run("changing unreferenced key does not change hash", func(t *testing.T) {
		s1 := secret(map[string]string{"api_key": "token123", "other_key": "v1"})
		s2 := secret(map[string]string{"api_key": "token123", "other_key": "v2"})
		h1 := HashReferencedKeys(manifest, "my-secret", s1)
		h2 := HashReferencedKeys(manifest, "my-secret", s2)
		assert.Equal(t, h1, h2)
	})

	t.Run("secret name mismatch returns hash of empty map", func(t *testing.T) {
		s := secret(map[string]string{"api_key": "token123"})
		h := HashReferencedKeys(manifest, "different-secret", s)
		// No bindings reference "different-secret" so vals is empty.
		hEmpty := HashReferencedKeys(types.MCPServerManifest{}, "my-secret", s)
		assert.Equal(t, h, hEmpty)
	})

	t.Run("missing key in secret data treated as empty string", func(t *testing.T) {
		// Secret exists but doesn't have the referenced key yet.
		s1 := secret(map[string]string{})
		s2 := secret(map[string]string{"api_key": ""})
		h1 := HashReferencedKeys(manifest, "my-secret", s1)
		h2 := HashReferencedKeys(manifest, "my-secret", s2)
		// Both produce the same hash — empty byte slice and absent key are equivalent.
		assert.Equal(t, h1, h2)
	})

	t.Run("remote header bindings included", func(t *testing.T) {
		remoteManifest := types.MCPServerManifest{
			Runtime: types.RuntimeRemote,
			RemoteConfig: &types.RemoteRuntimeConfig{
				Headers: []types.MCPHeader{
					{Key: "Authorization", SecretBinding: binding("auth-secret", "token")},
				},
			},
		}
		s1 := secret(map[string]string{"token": "bearer-v1", "noise": "ignored"})
		s2 := secret(map[string]string{"token": "bearer-v2", "noise": "ignored"})
		s3 := secret(map[string]string{"token": "bearer-v1", "noise": "changed"})

		h1 := HashReferencedKeys(remoteManifest, "auth-secret", s1)
		h2 := HashReferencedKeys(remoteManifest, "auth-secret", s2)
		h3 := HashReferencedKeys(remoteManifest, "auth-secret", s3)

		assert.NotEqual(t, h1, h2, "token change should alter hash")
		assert.Equal(t, h1, h3, "noise-only change should not alter hash")
	})

	t.Run("multiple bindings to same secret all contribute", func(t *testing.T) {
		multi := types.MCPServerManifest{
			Runtime: types.RuntimeNPX,
			Env: []types.MCPEnv{
				{MCPHeader: types.MCPHeader{Key: "KEY_A", SecretBinding: binding("my-secret", "key_a")}},
				{MCPHeader: types.MCPHeader{Key: "KEY_B", SecretBinding: binding("my-secret", "key_b")}},
			},
		}
		s1 := secret(map[string]string{"key_a": "v1", "key_b": "v1"})
		s2 := secret(map[string]string{"key_a": "v1", "key_b": "v2"}) // only key_b changes

		h1 := HashReferencedKeys(multi, "my-secret", s1)
		h2 := HashReferencedKeys(multi, "my-secret", s2)
		assert.NotEqual(t, h1, h2)
	})

	t.Run("deletion (nil) always differs from non-empty secret hash", func(t *testing.T) {
		s := secret(map[string]string{"api_key": "token123"})
		hNil := HashReferencedKeys(manifest, "my-secret", nil)
		hPresent := HashReferencedKeys(manifest, "my-secret", s)
		assert.NotEqual(t, hNil, hPresent)
	})
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
