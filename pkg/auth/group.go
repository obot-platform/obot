package auth

import (
	"fmt"
	"strings"
)

const (
	authProviderNameSuffix = "-auth-provider"
)

// GroupIDPrefixForAuthProvider returns the globally unique group ID prefix for an auth provider.
// Auth providers use their resource-name prefix followed by a slash for every group they emit.
func GroupIDPrefixForAuthProvider(authProviderName string) (string, error) {
	prefix, ok := strings.CutSuffix(authProviderName, authProviderNameSuffix)
	if !ok || prefix == "" {
		return "", fmt.Errorf("invalid auth provider name %q: expected <name>%s", authProviderName, authProviderNameSuffix)
	}
	return prefix + "/", nil
}
