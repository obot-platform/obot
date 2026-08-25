package auth

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	groupIDPrefixPattern = regexp.MustCompile(`^[[:alnum:]]+/$`)
)

// ValidateGroupIDPrefix validates a provider-declared group ID namespace. An empty prefix means
// that the provider does not support groups.
func ValidateGroupIDPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if !groupIDPrefixPattern.MatchString(prefix) {
		return fmt.Errorf("group ID prefix %q must contain only an alphanumeric namespace followed by a slash", prefix)
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
