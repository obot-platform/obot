package utils

// SlicesEqualIgnoreOrder returns true if slices a and b contain the same elements
// with the same multiplicities, regardless of their order.
func SlicesEqualIgnoreOrder[T comparable](a, b []T) bool {
	// If lengths differ, they cannot be equal
	if len(a) != len(b) {
		return false
	}

	// Count occurrences of each element in slice a
	counts := make(map[T]int, len(a))
	for _, v := range a {
		counts[v]++
	}

	// Subtract counts based on slice b
	for _, v := range b {
		if c, ok := counts[v]; !ok || c == 0 {
			// Either v was not in a, or b has more occurrences than a
			return false
		}
		counts[v]--
	}

	// Ensure all counts are zero (every a element matched by b)
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}

	return true
}
