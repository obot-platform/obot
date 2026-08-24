package auth

import (
	"fmt"
	"strings"
)

// ValidateGroupIDPrefix validates a provider-declared group ID namespace. An empty prefix means
// that the provider does not support groups.
func ValidateGroupIDPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if !strings.HasSuffix(prefix, "/") || prefix == "/" {
		return fmt.Errorf("group ID prefix %q must have a non-empty namespace and end with a slash", prefix)
	}
	if strings.ContainsAny(prefix, "%_\\") {
		return fmt.Errorf("group ID prefix %q must not contain SQL wildcard or escape characters", prefix)
	}
	return nil
}

// ValidateGroupID checks that a group returned by an auth provider belongs to the namespace that
// provider declared in its manifest.
func ValidateGroupID(groupID, prefix string) error {
	if prefix == "" {
		return fmt.Errorf("auth provider returned group ID %q without declaring a group ID prefix", groupID)
	}
	if !strings.HasPrefix(groupID, prefix) || groupID == prefix {
		return fmt.Errorf("auth provider returned group ID %q outside its declared prefix %q", groupID, prefix)
	}
	return nil
}
