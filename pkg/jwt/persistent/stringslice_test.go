package persistent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringSliceJSON(t *testing.T) {
	t.Run("marshals as comma separated string", func(t *testing.T) {
		data, err := json.Marshal(StringSlice{"admin", "authenticated"})
		require.NoError(t, err)
		assert.JSONEq(t, `"admin,authenticated"`, string(data))
	})

	t.Run("empty string unmarshals to nil", func(t *testing.T) {
		groups := StringSlice{"stale"}
		require.NoError(t, json.Unmarshal([]byte(`""`), &groups))
		assert.Nil(t, groups)
	})

	t.Run("splits non-empty string", func(t *testing.T) {
		var groups StringSlice
		require.NoError(t, json.Unmarshal([]byte(`"admin,authenticated"`), &groups))
		assert.Equal(t, StringSlice{"admin", "authenticated"}, groups)
	})

	t.Run("rejects non-string JSON", func(t *testing.T) {
		var groups StringSlice
		assert.Error(t, json.Unmarshal([]byte(`[]`), &groups))
	})
}
