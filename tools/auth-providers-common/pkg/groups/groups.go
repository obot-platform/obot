// Package groups provides utility functions for filtering group memberships
// used by authentication providers.
package groups

// Filter returns only groups that are in the allowed list.
// If allowed is empty, returns all groups unchanged.
func Filter(groups []string, allowed []string) []string {
	if len(allowed) == 0 {
		return groups
	}

	allowedSet := make(map[string]bool, len(allowed))
	for _, g := range allowed {
		allowedSet[g] = true
	}

	var filtered []string
	for _, g := range groups {
		if allowedSet[g] {
			filtered = append(filtered, g)
		}
	}
	return filtered
}
