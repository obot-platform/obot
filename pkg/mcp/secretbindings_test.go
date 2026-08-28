package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
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

func TestValidateSecretBindingsAvailable(t *testing.T) {
	const ns = "obot-ns"
	const label = "test-secret-binding-label"

	newClient := func(t *testing.T, objects ...kclient.Object) kclient.Client {
		t.Helper()
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	}

	requiredEnv := []types.MCPConfig{{Usage: types.Env, Key: "API_KEY", Required: true, SecretBinding: binding("bound-secret", "api_key")}}

	t.Run("valid secret", func(t *testing.T) {
		c := newClient(t, &corev1.Secret{
			Data: map[string][]byte{"api_key": []byte("fresh")},
			Name: "bound-secret", Namespace: ns, Labels: map[string]string{label: "true"},
		})

		require.NoError(t, ValidateSecretBindingsAvailable(t.Context(), c, ns, requiredEnv, label))
	})

	t.Run("missing secret", func(t *testing.T) {
		err := ValidateSecretBindingsAvailable(t.Context(), newClient(t), ns, requiredEnv, label)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unavailable Kubernetes Secret")
	})

	t.Run("missing key", func(t *testing.T) {
		c := newClient(t, &corev1.Secret{
			Data: map[string][]byte{"other": []byte("fresh")},
			Name: "bound-secret", Namespace: ns, Labels: map[string]string{label: "true"},
		})

		err := ValidateSecretBindingsAvailable(t.Context(), c, ns, requiredEnv, label)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unavailable Kubernetes Secret")
	})

	t.Run("empty value is treated as unavailable", func(t *testing.T) {
		c := newClient(t, &corev1.Secret{
			Data: map[string][]byte{"api_key": []byte("")},
			Name: "bound-secret", Namespace: ns, Labels: map[string]string{label: "true"},
		})

		err := ValidateSecretBindingsAvailable(t.Context(), c, ns, requiredEnv, label)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unavailable Kubernetes Secret")
	})

	t.Run("unlabeled secret", func(t *testing.T) {
		c := newClient(t, &corev1.Secret{
			Data: map[string][]byte{"api_key": []byte("fresh")},
			Name: "bound-secret", Namespace: ns,
		})

		err := ValidateSecretBindingsAvailable(t.Context(), c, ns, requiredEnv, label)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unavailable Kubernetes Secret")
	})

	t.Run("binding is checked even when optional", func(t *testing.T) {
		env := []types.MCPConfig{{Usage: types.Env, Key: "API_KEY", SecretBinding: binding("missing", "api_key")}}

		err := ValidateSecretBindingsAvailable(t.Context(), newClient(t), ns, env, label)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unavailable Kubernetes Secret")
	})

	t.Run("remote header", func(t *testing.T) {
		remote := []types.MCPConfig{{Usage: types.Header, Key: "Authorization", Required: true, SecretBinding: binding("auth-secret", "token")}}
		c := newClient(t, &corev1.Secret{
			Data: map[string][]byte{"token": []byte("Bearer abc")},
			Name: "auth-secret", Namespace: ns, Labels: map[string]string{label: "true"},
		})

		require.NoError(t, ValidateSecretBindingsAvailable(t.Context(), c, ns, remote, label))
	})

	t.Run("reports all missing bindings", func(t *testing.T) {
		remote := []types.MCPConfig{{Usage: types.Header, Key: "Authorization", Required: true, SecretBinding: binding("auth-secret", "token")}}

		err := ValidateSecretBindingsAvailable(t.Context(), newClient(t), ns, append(requiredEnv, remote...), label)
		require.Error(t, err)
		assert.Equal(t, err.Error(), `secret bindings reference unavailable Kubernetes Secrets: env "API_KEY" references obot-ns/bound-secret, header "Authorization" references obot-ns/auth-secret`)
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
