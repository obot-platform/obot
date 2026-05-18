package imagepullsecrets

import (
	"slices"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func EffectiveSecretNames(staticPullSecrets []string, managedSecrets []v1.ImagePullSecret) []string {
	static := CleanSecretNames(staticPullSecrets)
	if len(static) > 0 {
		return static
	}

	names := make([]string, 0, len(managedSecrets))
	for _, secret := range managedSecrets {
		name, ok := activeSecretName(secret)
		if !ok {
			continue
		}
		names = append(names, name)
	}

	return CleanSecretNames(names)
}

func Hash(secretNames []string) string {
	return hash.Digest(CleanSecretNames(secretNames))
}

func CleanSecretNames(names []string) []string {
	result := make([]string, 0, len(names))
	seen := map[string]struct{}{}

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}

	slices.Sort(result)
	return result
}

func activeSecretName(secret v1.ImagePullSecret) (string, bool) {
	if secret.DeletionTimestamp != nil {
		return "", false
	}
	if !secret.Spec.Enabled {
		return "", false
	}
	name := strings.TrimSpace(secret.Name)
	if name == "" {
		return "", false
	}
	return name, true
}
