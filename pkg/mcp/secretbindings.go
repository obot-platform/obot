package mcp

import (
	"context"
	"fmt"
	"maps"
	"sort"

	"github.com/obot-platform/obot/apiclient/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MergeBoundCreds resolves every secretBinding referenced by envs and (for
// remote runtime) remoteConfig.Headers from the obot namespace and returns a
// NEW map containing credEnv merged with the resolved values:
//
//   - env bindings → out[env.Key] = <secret value>
//   - header bindings → out[header.Key] = <secret value>
//
// Pass `manifest.Env, manifest.RemoteConfig` from any of MCPServerManifest,
// SystemMCPServerManifest, or a synthesized shape — they share the same
// MCPEnv / RemoteRuntimeConfig field types.
//
// Bound values overwrite any pre-existing credEnv entry for the same key.
// The validator rejects the bound-and-literal combination at write-time, so
// collisions only happen via a misbehaving caller; we drop the stale credEnv
// value defensively.
//
// IMPORTANT: This function never mutates the caller's credEnv. The caller's
// credEnv reflects only user-supplied credential-store values and is safe to
// return to API reveal endpoints. The returned merged map carries bound
// secret VALUES and MUST NOT be returned to API callers — pass it to
// ServerToServerConfig / ConvertMCPServer only.
//
// If there are no secretBindings, MergeBoundCreds returns credEnv unchanged
// (no allocation). If c is nil (docker backend), bindings cannot be resolved
// and the returned map omits them — the downstream missing-required gate
// then fires for required bindings.
//
// Secrets must have the configured secret-binding allow label. A Secret without
// that label is treated as unavailable, the same as a missing Secret/key.
//
// Lookups are cached per-call by Secret name so a manifest with N bindings
// against the same Secret performs one Get. Reads hit nah's watch cache, so
// calling this from API request paths is cheap.
func MergeBoundCreds(
	ctx context.Context,
	c kclient.Client,
	obotNamespace string,
	envs []types.MCPEnv,
	remoteConfig *types.RemoteRuntimeConfig,
	credEnv map[string]string,
	allowedLabel string,
) (map[string]string, error) {
	// Fast path: no bindings → nothing to merge, return credEnv as-is.
	if !hasAnyBinding(envs, remoteConfig) {
		return credEnv, nil
	}

	// Copy credEnv so we never mutate the caller's map.
	merged := make(map[string]string, len(credEnv)+8)
	maps.Copy(merged, credEnv)

	if c == nil {
		// No kclient (docker backend) → strip any stale credEnv values for
		// bound keys so the downstream missing-required gate fires
		// uniformly. The API validator rejects bindings on the docker
		// backend, but be defensive.
		for _, e := range envs {
			if e.SecretBinding != nil {
				delete(merged, e.Key)
			}
		}
		if remoteConfig != nil {
			for _, h := range remoteConfig.Headers {
				if h.SecretBinding != nil {
					delete(merged, h.Key)
				}
			}
		}
		return merged, nil
	}

	// secretCache[name] is nil when the Secret was confirmed unavailable,
	// non-nil (possibly empty) when it exists and is allowed.
	secretCache := map[string]map[string][]byte{}

	lookup := func(b *types.MCPSecretBinding) (string, bool, error) {
		if b == nil || b.Name == "" || b.Key == "" {
			return "", false, nil
		}
		data, cached := secretCache[b.Name]
		if !cached {
			var s corev1.Secret
			getErr := c.Get(ctx, kclient.ObjectKey{Namespace: obotNamespace, Name: b.Name}, &s)
			switch {
			case apierrors.IsNotFound(getErr):
				secretCache[b.Name] = nil
				return "", false, nil
			case getErr != nil:
				return "", false, fmt.Errorf("get secret %s/%s: %w", obotNamespace, b.Name, getErr)
			}
			if _, ok := s.Labels[allowedLabel]; !ok {
				secretCache[b.Name] = nil
				return "", false, nil
			}
			secretCache[b.Name] = s.Data
			data = s.Data
		}
		if data == nil {
			return "", false, nil
		}
		v, ok := data[b.Key]
		if !ok || len(v) == 0 {
			return "", false, nil
		}
		return string(v), true, nil
	}

	for _, env := range envs {
		if env.SecretBinding == nil {
			continue
		}
		// Strip any stale credEnv value before resolving. Bound source of
		// truth is the Secret; user-supplied values for bound keys are
		// rejected by the validator.
		delete(merged, env.Key)

		val, ok, err := lookup(env.SecretBinding)
		if err != nil {
			return nil, err
		}
		if !ok {
			// Not resolved → leave key absent so the downstream
			// missing-required gate marks it as missing.
			continue
		}
		merged[env.Key] = val
	}

	if remoteConfig != nil {
		for _, h := range remoteConfig.Headers {
			if h.SecretBinding == nil {
				continue
			}
			delete(merged, h.Key)

			val, ok, err := lookup(h.SecretBinding)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			merged[h.Key] = val
		}
	}

	return merged, nil
}

// ValidateSecretBindingsAvailable verifies secret-bound config can be resolved
// before creating/updating/launching a server that users cannot fix.
func ValidateSecretBindingsAvailable(ctx context.Context, c kclient.Client, obotNamespace string, envs []types.MCPEnv, remoteConfig *types.RemoteRuntimeConfig, allowedLabel string) error {
	hasBinding := false
	for _, env := range envs {
		if env.SecretBinding != nil {
			hasBinding = true
			break
		}
	}
	if !hasBinding && remoteConfig != nil {
		for _, header := range remoteConfig.Headers {
			if header.SecretBinding != nil {
				hasBinding = true
				break
			}
		}
	}
	if !hasBinding {
		return nil
	}
	if c == nil {
		return fmt.Errorf("secret bindings require a Kubernetes client")
	}

	resolved, err := MergeBoundCreds(ctx, c, obotNamespace, envs, remoteConfig, nil, allowedLabel)
	if err != nil {
		return err
	}

	check := func(kind, key string, b *types.MCPSecretBinding) error {
		if b == nil {
			return nil
		}
		if _, ok := resolved[key]; !ok {
			return fmt.Errorf("secret binding %s %q references unavailable Kubernetes Secret %s/%s", kind, key, obotNamespace, b.Name)
		}
		return nil
	}

	for _, env := range envs {
		if err := check("env", env.Key, env.SecretBinding); err != nil {
			return err
		}
	}
	if remoteConfig != nil {
		for _, header := range remoteConfig.Headers {
			if err := check("header", header.Key, header.SecretBinding); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasAnyBinding(envs []types.MCPEnv, remoteConfig *types.RemoteRuntimeConfig) bool {
	for _, e := range envs {
		if e.SecretBinding != nil {
			return true
		}
	}
	if remoteConfig != nil {
		for _, h := range remoteConfig.Headers {
			if h.SecretBinding != nil {
				return true
			}
		}
	}
	return false
}

// ListAllowedSecretBindingTargets returns labeled Kubernetes Secrets and data keys that admins may select for MCP secret bindings.
func ListAllowedSecretBindingTargets(ctx context.Context, c kclient.Client, obotNamespace, allowedLabel string) ([]types.MCPAllowedSecretBindingTarget, error) {
	if c == nil {
		return nil, nil
	}

	requirement, err := labels.NewRequirement(allowedLabel, selection.Exists, nil)
	if err != nil || requirement == nil {
		return nil, fmt.Errorf("create allowed secret binding label selector: %w", err)
	}
	selector := labels.NewSelector().Add(*requirement)
	var secrets corev1.SecretList
	if err := c.List(ctx, &secrets, kclient.InNamespace(obotNamespace), kclient.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, fmt.Errorf("list allowed secret bindings: %w", err)
	}

	targets := make([]types.MCPAllowedSecretBindingTarget, 0, len(secrets.Items))
	for _, secret := range secrets.Items {
		keys := make([]string, 0, len(secret.Data))
		for key := range secret.Data {
			keys = append(keys, key)
		}
		if len(keys) == 0 {
			continue
		}
		sort.Strings(keys)
		targets = append(targets, types.MCPAllowedSecretBindingTarget{
			Name: secret.Name,
			Keys: keys,
		})
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})
	return targets, nil
}
