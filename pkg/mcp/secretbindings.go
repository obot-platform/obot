package mcp

import (
	"context"
	"fmt"
	"maps"

	"github.com/obot-platform/obot/apiclient/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	// secretCache[name] is nil when the Secret was confirmed missing,
	// non-nil (possibly empty) when it exists.
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
