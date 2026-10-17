package handlers

import (
	"net/url"
	"strings"
)

// parseMultiValue returns the values of a query parameter that may repeat or hold
// comma-separated values, without empty entries.
func parseMultiValue(queryValues url.Values, key string) []string {
	values := queryValues[key]
	if len(values) == 0 {
		return nil
	}

	var result []string
	for _, value := range values {
		if value == "" {
			continue
		}
		for part := range strings.SplitSeq(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}
