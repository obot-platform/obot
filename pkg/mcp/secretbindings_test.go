package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func binding(secretName, key string) *types.MCPSecretBinding {
	return &types.MCPSecretBinding{Name: secretName, Key: key}
}

func TestMergeBoundCreds(t *testing.T) {
	const ns = "obot-ns"
	const label = "test-secret-binding-label"

	newClient := func(t *testing.T, objects ...kclient.Object) kclient.Client {
		t.Helper()
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	}

	t.Run("does not mutate input cred map and overrides stale values", func(t *testing.T) {
		manifestEnv := []types.MCPConfig{{Usage: types.Env, Key: "API_KEY", SecretBinding: binding("bound-secret", "api_key")}}
		input := map[string]string{"API_KEY": "stale", "UNCHANGED": "keep"}
		inputBefore := map[string]string{"API_KEY": "stale", "UNCHANGED": "keep"}

		c := newClient(t, &corev1.Secret{
			Data: map[string][]byte{"api_key": []byte("fresh")},
			Name: "bound-secret", Namespace: ns, Labels: map[string]string{label: "true"},
		})

		out, err := MergeBoundCreds(t.Context(), c, ns, manifestEnv, input, label)
		require.NoError(t, err)
		assert.Equal(t, inputBefore, input)
		assert.Equal(t, "fresh", out["API_KEY"])
		assert.Equal(t, "keep", out["UNCHANGED"])
	})

	t.Run("omits missing secret and missing key", func(t *testing.T) {
		manifestEnv := []types.MCPConfig{{Usage: types.Env, Key: "API_KEY", SecretBinding: binding("missing-secret", "api_key")}}
		input := map[string]string{"API_KEY": "stale", "OTHER": "ok"}

		out, err := MergeBoundCreds(t.Context(), newClient(t), ns, manifestEnv, input, label)
		require.NoError(t, err)
		assert.NotContains(t, out, "API_KEY")
		assert.Equal(t, "ok", out["OTHER"])

		manifestEnv[0].SecretBinding = binding("present-secret", "missing-key")
		c := newClient(t, &corev1.Secret{
			Data: map[string][]byte{"other": []byte("x")},
			Name: "present-secret", Namespace: ns, Labels: map[string]string{label: "true"},
		})
		out, err = MergeBoundCreds(t.Context(), c, ns, manifestEnv, input, label)
		require.NoError(t, err)
		assert.NotContains(t, out, "API_KEY")
	})

	t.Run("merges remote header bindings", func(t *testing.T) {
		remote := []types.MCPConfig{{Usage: types.Header, Key: "Authorization", SecretBinding: binding("auth-secret", "token")}}
		c := newClient(t, &corev1.Secret{
			Data: map[string][]byte{"token": []byte("Bearer abc")},
			Name: "auth-secret", Namespace: ns, Labels: map[string]string{label: "true"},
		})

		out, err := MergeBoundCreds(t.Context(), c, ns, remote, map[string]string{"Authorization": "stale"}, label)
		require.NoError(t, err)
		assert.Equal(t, "Bearer abc", out["Authorization"])
	})

	t.Run("nil client strips bound keys and keeps others", func(t *testing.T) {
		manifestEnv := []types.MCPConfig{{Usage: types.Env, Key: "API_KEY", SecretBinding: binding("s", "k")}}
		remote := []types.MCPConfig{{Usage: types.Header, Key: "Authorization", SecretBinding: binding("s", "token")}}
		in := map[string]string{"API_KEY": "x", "Authorization": "y", "OTHER": "ok"}

		out, err := MergeBoundCreds(t.Context(), nil, ns, append(manifestEnv, remote...), in, label)
		require.NoError(t, err)
		assert.NotContains(t, out, "API_KEY")
		assert.NotContains(t, out, "Authorization")
		assert.Equal(t, "ok", out["OTHER"])
	})

	t.Run("empty secret value is treated as missing", func(t *testing.T) {
		manifestEnv := []types.MCPConfig{{Usage: types.Env, Key: "API_KEY", SecretBinding: binding("bound-secret", "api_key")}}
		in := map[string]string{"API_KEY": "stale"}
		c := newClient(t, &corev1.Secret{
			Data: map[string][]byte{"api_key": []byte("")},
			Name: "bound-secret", Namespace: ns, Labels: map[string]string{label: "true"},
		})

		out, err := MergeBoundCreds(t.Context(), c, ns, manifestEnv, in, label)
		require.NoError(t, err)
		assert.NotContains(t, out, "API_KEY")
	})

	t.Run("unlabeled secret is treated as missing", func(t *testing.T) {
		manifestEnv := []types.MCPConfig{{Usage: types.Env, Key: "API_KEY", SecretBinding: binding("bound-secret", "api_key")}}
		in := map[string]string{"API_KEY": "stale", "OTHER": "ok"}
		c := newClient(t, &corev1.Secret{
			Data: map[string][]byte{"api_key": []byte("fresh")},
			Name: "bound-secret", Namespace: ns,
		})

		out, err := MergeBoundCreds(t.Context(), c, ns, manifestEnv, in, label)
		require.NoError(t, err)
		assert.NotContains(t, out, "API_KEY")
		assert.Equal(t, "ok", out["OTHER"])
	})
}

func TestMissingSecretBindings(t *testing.T) {
	const ns = "obot-ns"
	const label = "test-secret-binding-label"

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
		Data: map[string][]byte{"env_key": []byte("fresh")},
		Name: "bound-secret", Namespace: ns, Labels: map[string]string{label: "true"},
	}).Build()

	missing, err := MissingSecretBindings(t.Context(), c, ns,
		[]types.MCPConfig{
			{Usage: types.Env, Key: "ENV_KEY", SecretBinding: binding("bound-secret", "env_key")},
			{Usage: types.Header, Key: "Authorization", SecretBinding: binding("missing-secret", "token")},
		},
		label,
	)
	require.NoError(t, err)
	require.Len(t, missing, 1)
	assert.Equal(t, "header", missing[0].Kind)
	assert.Equal(t, "Authorization", missing[0].Header.Key)
	assert.Equal(t, binding("missing-secret", "token"), missing[0].Binding)
}

func TestListAllowedSecretBindingTargets(t *testing.T) {
	const ns = "obot-ns"
	const label = "test-secret-binding-label"

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Secret{
			Name: "z-secret", Namespace: ns, Labels: map[string]string{label: "false"},
			Data: map[string][]byte{"b": []byte("value-b"), "a": []byte("value-a")},
		},
		&corev1.Secret{
			Name: "a-secret", Namespace: ns, Labels: map[string]string{label: "true"},
			Data: map[string][]byte{"token": []byte("secret-value")},
		},
		&corev1.Secret{
			Name: "empty", Namespace: ns, Labels: map[string]string{label: "true"},
		},
		&corev1.Secret{
			Name: "unlabeled", Namespace: ns,
			Data: map[string][]byte{"token": []byte("hidden")},
		},
	).Build()

	targets, err := ListAllowedSecretBindingTargets(t.Context(), c, ns, label)
	require.NoError(t, err)
	assert.Equal(t, []types.MCPAllowedSecretBindingTarget{
		{Name: "a-secret", Keys: []string{"token"}},
		{Name: "z-secret", Keys: []string{"a", "b"}},
	}, targets)
}

func TestUnresolvedSecretBindingKeys(t *testing.T) {
	const ns = "obot-ns"
	const label = "test-secret-binding-label"

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Secret{
			Data: map[string][]byte{"present": []byte("value"), "empty": nil},
			Name: "bound-secret", Namespace: ns, Labels: map[string]string{label: "true"},
		},
		&corev1.Secret{
			Data: map[string][]byte{"present": []byte("value")},
			Name: "unlabeled-secret", Namespace: ns,
		},
	).Build()

	config := []types.MCPConfig{
		{Usage: types.Env, Key: "RESOLVED", SecretBinding: binding("bound-secret", "present")},
		{Usage: types.Header, Key: "X-Empty", SecretBinding: binding("bound-secret", "empty")},
		{Usage: types.Env, Key: "MISSING_KEY", SecretBinding: binding("bound-secret", "absent")},
		{Usage: types.Env, Key: "MISSING_SECRET", SecretBinding: binding("missing-secret", "present")},
		{Usage: types.Env, Key: "NOT_ALLOWED", SecretBinding: binding("unlabeled-secret", "present")},
		{Usage: types.Env, Key: "USER_VALUE"},
	}

	unresolved, err := UnresolvedSecretBindingKeys(t.Context(), c, ns, config, label)
	require.NoError(t, err)
	assert.Equal(t, []string{"MISSING_KEY", "MISSING_SECRET", "NOT_ALLOWED", "X-Empty"}, unresolved)

	unresolved, err = UnresolvedSecretBindingKeys(t.Context(), nil, ns, config, label)
	require.NoError(t, err)
	assert.Equal(t, []string{"MISSING_KEY", "MISSING_SECRET", "NOT_ALLOWED", "RESOLVED", "X-Empty"}, unresolved)

	unresolved, err = UnresolvedSecretBindingKeys(t.Context(), c, ns, config[5:], label)
	require.NoError(t, err)
	assert.Empty(t, unresolved)
}

func TestSecretBindingsCheckHash(t *testing.T) {
	const label = "test-secret-binding-label"
	config := []types.MCPConfig{
		{Usage: types.Env, Key: "TOKEN", SecretBinding: binding("tokens", "token")},
		{Usage: types.Env, Key: "USER_VALUE"},
	}
	hash := SecretBindingsCheckHash(config, label)
	require.NotEmpty(t, hash)

	assert.Empty(t, SecretBindingsCheckHash(config[1:], label), "config without bindings")
	assert.NotEqual(t, hash, SecretBindingsCheckHash(config, "other-label"), "allow label changed")

	rebound := []types.MCPConfig{{Usage: types.Env, Key: "TOKEN", SecretBinding: binding("tokens", "other")}, config[1]}
	assert.NotEqual(t, hash, SecretBindingsCheckHash(rebound, label), "binding changed")

	userValueChanged := []types.MCPConfig{config[0], {Usage: types.Env, Key: "USER_VALUE", Required: true}}
	assert.Equal(t, hash, SecretBindingsCheckHash(userValueChanged, label), "unbound config changed")
}

func TestRefreshSecretBindingStatus(t *testing.T) {
	const ns = "obot-ns"
	const label = "test-secret-binding-label"

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
		Data: map[string][]byte{"token": []byte("value")},
		Name: "tokens", Namespace: ns, Labels: map[string]string{label: "true"},
	}).Build()

	newServer := func(config ...types.MCPConfig) *v1.MCPServer {
		return &v1.MCPServer{Spec: v1.MCPServerSpec{Manifest: types.MCPServerManifest{Config: config}}}
	}
	resolved := types.MCPConfig{Usage: types.Env, Key: "RESOLVED", SecretBinding: binding("tokens", "token")}
	missing := types.MCPConfig{Usage: types.Env, Key: "MISSING", SecretBinding: binding("missing", "token")}

	t.Run("keeps a status the controller computed for the current bindings", func(t *testing.T) {
		server := newServer(resolved, missing)
		server.Status.SecretBindingsCheckHash = SecretBindingsCheckHash(server.Spec.Manifest.Config, label)
		server.Status.UnresolvedSecretBindings = []string{"RESOLVED"}

		// A nil client would leave every binding unresolved if it were read.
		require.NoError(t, RefreshSecretBindingStatus(t.Context(), nil, ns, server, label))
		assert.Equal(t, []string{"RESOLVED"}, server.Status.UnresolvedSecretBindings)
	})

	t.Run("resolves bindings the controller has not checked", func(t *testing.T) {
		server := newServer(resolved, missing)

		require.NoError(t, RefreshSecretBindingStatus(t.Context(), c, ns, server, label))
		assert.Equal(t, []string{"MISSING"}, server.Status.UnresolvedSecretBindings)
		assert.Equal(t, SecretBindingsCheckHash(server.Spec.Manifest.Config, label), server.Status.SecretBindingsCheckHash)
	})

	t.Run("resolves bindings that changed since the controller checked them", func(t *testing.T) {
		server := newServer(resolved)
		server.Status.SecretBindingsCheckHash = SecretBindingsCheckHash(server.Spec.Manifest.Config, label)
		server.Spec.Manifest.Config = append(server.Spec.Manifest.Config, missing)

		require.NoError(t, RefreshSecretBindingStatus(t.Context(), c, ns, server, label))
		assert.Equal(t, []string{"MISSING"}, server.Status.UnresolvedSecretBindings)
	})

	t.Run("does nothing for servers without bindings", func(t *testing.T) {
		server := newServer(types.MCPConfig{Usage: types.Env, Key: "USER_VALUE", Required: true})

		require.NoError(t, RefreshSecretBindingStatus(t.Context(), nil, ns, server, label))
		assert.Empty(t, server.Status.UnresolvedSecretBindings)
		assert.Empty(t, server.Status.SecretBindingsCheckHash)
	})
}
