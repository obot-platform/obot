package api

import (
	"net/url"
	"strconv"
	"strings"
)

const (
	// IncludeOnBehalfOfParam is the query parameter of the list and export endpoints that filter activity by user:
	// when true, a user's activity includes what agents did on the user's behalf. By default it does not.
	IncludeOnBehalfOfParam = "include_on_behalf_of"
	// OnBehalfOfParam is the query parameter of the list, filter option and stats endpoints that filter activity by
	// the users agents acted for: one or more user IDs, repeated or comma-separated. It is a filter of its own, which
	// the others narrow, so that user_id=agent:7&on_behalf_of=12 selects what agent 7 did for user 12.
	OnBehalfOfParam = "on_behalf_of"
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

// IncludeOnBehalfOf reports whether query asks for a user's activity to include what agents did on the user's behalf
// (IncludeOnBehalfOfParam). Anything but a true value is false.
func IncludeOnBehalfOf(query url.Values) bool {
	include, _ := strconv.ParseBool(query.Get(IncludeOnBehalfOfParam))
	return include
}
