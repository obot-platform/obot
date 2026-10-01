package mcp

import (
	"iter"
)

// collectAll drains a paginated go-sdk list iterator (such as ClientSession.Tools),
// which follows nextCursor until the server stops returning one.
func collectAll[T any](seq iter.Seq2[*T, error]) ([]*T, error) {
	var items []*T
	for item, err := range seq {
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
