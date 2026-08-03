package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Skill embeds Metadata, whose map is tagged "metadata". SkillManifest once
// carried a second field tagged the same; two promoted fields at equal depth
// sharing one name is a conflict, and encoding/json resolves it by dropping
// both -- so a skill serialized neither its frontmatter metadata nor the
// envelope's. The frontmatter's metadata now travels in the envelope map, which
// is the only "metadata" on the type. This pins that it survives the trip.
func TestSkillSerializesMetadata(t *testing.T) {
	s := Skill{}
	s.ID = "sk1abc"
	s.Metadata.Metadata = map[string]string{"author": "tester"}
	s.SkillManifest.Name = "my-skill"

	b, err := json.Marshal(s)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))

	require.Contains(t, got, "metadata", "a json tag collision silently dropped this before")
	assert.Equal(t, map[string]any{"author": "tester"}, got["metadata"])
	assert.Equal(t, "sk1abc", got["id"])
	assert.Equal(t, "my-skill", got["name"])
}

func TestSkillOmitsEmptyMetadata(t *testing.T) {
	b, err := json.Marshal(Skill{})
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))

	assert.NotContains(t, got, "metadata", "empty metadata should be omitted, not sent as null")
}
