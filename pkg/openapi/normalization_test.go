package openapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYAMLScalarNormalization(t *testing.T) {
	for _, value := range []string{"n", "y", "yes", "no", "on", "off", "N", "Y", "YES", "NO", "ON", "OFF"} {
		t.Run(value, func(t *testing.T) {
			document, _, err := normalize([]byte("name: " + value + "\n" + value + ": value\n"))
			require.NoError(t, err)
			require.Equal(t, value, document["name"])
			require.Equal(t, "value", document[value])
		})
	}

	_, yamlJSON, err := normalize([]byte(`enabled: true
disabled: false
empty: null
integer: 9007199254740993
fraction: 1.25
date: 2026-10-12
timestamp: 2026-10-12T10:20:30Z
values: [n, true, 42, null]
responses:
  200:
    description: OK
`))
	require.NoError(t, err)
	_, jsonJSON, err := normalize([]byte(`{"enabled":true,"disabled":false,"empty":null,"integer":9007199254740993,"fraction":1.25,"date":"2026-10-12","timestamp":"2026-10-12T10:20:30Z","values":["n",true,42,null],"responses":{"200":{"description":"OK"}}}`))
	require.NoError(t, err)
	require.Equal(t, jsonJSON, yamlJSON)
}

func TestYAMLNormalizedKeyCollisions(t *testing.T) {
	for _, content := range []string{
		"responses:\n  200: first\n  '200': second\n",
		"responses:\n  0xc8: first\n  '200': second\n",
	} {
		_, _, err := normalize([]byte(content))
		require.ErrorContains(t, err, "schema object keys must be unique strings")
	}
}

func TestYAMLNonJSONScalars(t *testing.T) {
	for _, value := range []string{".nan", ".inf", "-.inf"} {
		_, _, err := normalize([]byte("value: " + value))
		require.ErrorContains(t, err, "schema cannot be converted to JSON")
	}
}
