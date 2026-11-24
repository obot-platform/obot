package mcpcatalog

import (
	"strings"
)

const (
	catalogReferenceSeparator = "::"
)

// SourceRef builds an explicit source::entryKey reference.
func SourceRef(sourceID, entryKey string) string {
	return sourceID + catalogReferenceSeparator + entryKey
}

// ParseSourceRef returns the source/key pair for either an explicit
// source::entryKey reference or a same-source shorthand entryKey.
func ParseSourceRef(sourceID, catalogEntryID string) (refSourceID, entryKey string, hasSep, valid bool) {
	refSourceID, entryKey, hasSep = strings.Cut(catalogEntryID, catalogReferenceSeparator)
	if !hasSep {
		return sourceID, catalogEntryID, false, true
	}
	if strings.Contains(entryKey, catalogReferenceSeparator) {
		return refSourceID, entryKey, true, false
	}
	return refSourceID, entryKey, true, refSourceID != "" && entryKey != ""
}
