package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Skill embeds both Metadata (field Metadata, tag "metadata") and SkillManifest.
// When SkillManifest.MetadataValues was also tagged "metadata", the two promoted
// fields collided at equal depth and encoding/json dropped both, so a skill's
// metadata never reached clients. This pins the fix.
func TestSkillSerializesMetadataValues(t *testing.T) {
	s := Skill{}
	s.ID = "sk1abc"
	s.SkillManifest.Name = "my-skill"
	s.SkillManifest.MetadataValues = map[string]string{"author": "tester"}

	b, err := json.Marshal(s)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))

	require.Contains(t, got, "metadataValues",
		"skill metadata must serialize; a json tag collision silently dropped it before")
	assert.Equal(t, map[string]any{"author": "tester"}, got["metadataValues"])
	assert.Equal(t, "sk1abc", got["id"])
	assert.Equal(t, "my-skill", got["name"])
}

func TestSkillOmitsEmptyMetadataValues(t *testing.T) {
	b, err := json.Marshal(Skill{})
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))

	assert.NotContains(t, got, "metadataValues", "empty metadata should be omitted, not sent as null")
}
