package api

import (
	"strconv"
	"strings"
)

// ParseUintList parses repeated and comma-separated query parameter values,
// returning only valid, positive uint values.
func ParseUintList(values []string) []uint {
	var result []uint
	for _, value := range values {
		for part := range strings.SplitSeq(value, ",") {
			id, err := strconv.ParseUint(strings.TrimSpace(part), 10, 0)
			if err == nil && id > 0 {
				result = append(result, uint(id))
			}
		}
	}
	return result
}
