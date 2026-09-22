package mcpcatalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateConfigurationFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		data  string
		field string
	}{
		{
			name:  "env",
			data:  "env: []",
			field: "env",
		},
		{
			name:  "headers",
			data:  `{"remoteConfig":{"headers":[{"name":"Authorization"}]}}`,
			field: "remoteConfig.headers",
		},
		{
			name:  "null user headers",
			data:  "multiUserConfig:\n  userDefinedHeaders: null",
			field: "multiUserConfig.userDefinedHeaders",
		},
		{
			name:  "new and old fields together",
			data:  "config: []\nenv: []",
			field: "env",
		},
		{
			name: "unknown fields and new configuration",
			data: "config:\n- key: TOKEN\n  usage: env\nunknown: true\nnpxConfig:\n  package: test\n  unknown: true",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateConfigurationFields([]byte(tc.data))
			if tc.field == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tc.field)
			require.ErrorContains(t, err, "top-level config")
		})
	}
}
