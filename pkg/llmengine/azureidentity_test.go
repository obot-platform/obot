package llmengine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAzureCredentialClearValidatesRetainedConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		config  string
		patch   string
		wantErr bool
	}{
		{
			name:    "API key with custom endpoint",
			value:   "owned-api-key",
			config:  `{"endpoint":"https://8.8.8.8"}`,
			patch:   `{"value":""}`,
			wantErr: true,
		},
		{
			name:    "custom endpoint",
			config:  `{"endpoint":"https://8.8.8.8","client_id":"client","client_secret":"secret","tenant_id":"tenant"}`,
			patch:   `{"azure_key_config":{"client_secret":""}}`,
			wantErr: true,
		},
		{
			name:    "custom scope",
			config:  `{"endpoint":"https://res.openai.azure.com","client_id":"client","client_secret":"secret","tenant_id":"tenant","scopes":["https://management.azure.com/.default"]}`,
			patch:   `{"azure_key_config":{"client_id":""}}`,
			wantErr: true,
		},
		{
			name:    "custom alias endpoint",
			config:  `{"endpoint":"https://res.openai.azure.com","client_id":"client","client_secret":"secret","tenant_id":"tenant"}`,
			patch:   `{"azure_key_config":{"tenant_id":""}}`,
			wantErr: true,
		},
		{
			name:   "Azure AI endpoint allows server identity",
			config: `{"endpoint":"https://res.openai.azure.com","client_id":"client","client_secret":"secret","tenant_id":"tenant"}`,
			patch:  `{"azure_key_config":{"client_secret":""}}`,
		},
		{
			name:   "owned credentials remain on rename",
			config: `{"endpoint":"https://8.8.8.8","client_id":"client","client_secret":"secret","tenant_id":"tenant"}`,
			patch:  `{"name":"renamed"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			_, err := e.CreateProvider(ctx, []byte(`{"provider":"azure"}`))
			require.NoError(t, err)
			body := `{"name":"owned","value":"` + tc.value + `","azure_key_config":` + tc.config
			if tc.name == "custom alias endpoint" {
				body += `,"aliases":{"friendly":{"model_id":"current","endpoint":"https://8.8.8.8"}}`
			}
			key, err := e.CreateKey(ctx, "azure", []byte(body+`}`))
			require.NoError(t, err)
			before, err := e.store.GetProvider(ctx, "azure")
			require.NoError(t, err)
			_, err = e.UpdateKey(ctx, "azure", key.ID, []byte(tc.patch))
			if tc.wantErr {
				require.Error(t, err)
				after, err := e.store.GetProvider(ctx, "azure")
				require.NoError(t, err)
				require.Equal(t, before, after)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
