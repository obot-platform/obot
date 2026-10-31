package mcpcatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSourceRef(t *testing.T) {
	const currentSourceID = "current-source"

	tests := []struct {
		name           string
		catalogEntryID string
		wantSourceID   string
		wantEntryKey   string
		wantHasSep     bool
		wantValid      bool
	}{
		{
			name:           "same source shorthand",
			catalogEntryID: "gmail",
			wantSourceID:   currentSourceID,
			wantEntryKey:   "gmail",
			wantHasSep:     false,
			wantValid:      true,
		},
		{
			name:           "explicit source",
			catalogEntryID: "other-source::gmail",
			wantSourceID:   "other-source",
			wantEntryKey:   "gmail",
			wantHasSep:     true,
			wantValid:      true,
		},
		{
			name:           "missing source",
			catalogEntryID: "::gmail",
			wantEntryKey:   "gmail",
			wantHasSep:     true,
			wantValid:      false,
		},
		{
			name:           "missing entry key",
			catalogEntryID: "source::",
			wantSourceID:   "source",
			wantHasSep:     true,
			wantValid:      false,
		},
		{
			name:           "too many separators",
			catalogEntryID: "source::key::extra",
			wantSourceID:   "source",
			wantEntryKey:   "key::extra",
			wantHasSep:     true,
			wantValid:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceID, entryKey, hasSep, valid := ParseSourceRef(currentSourceID, tt.catalogEntryID)

			assert.Equal(t, tt.wantSourceID, sourceID)
			assert.Equal(t, tt.wantEntryKey, entryKey)
			assert.Equal(t, tt.wantHasSep, hasSep)
			assert.Equal(t, tt.wantValid, valid)
		})
	}
}
