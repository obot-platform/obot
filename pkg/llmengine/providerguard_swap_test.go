package llmengine

import (
	"encoding/json"
	"testing"
)

func TestGuardAuthenticationSwap(t *testing.T) {
	e, ctx := startTestEngine(t)
	if _, err := e.CreateProvider(ctx, mustJSON(t, map[string]any{"provider": "azure"})); err != nil {
		t.Fatal(err)
	}
	const masked = "abcd************************wxyz"
	const api = `{"value":"` + masked + `","azure_key_config":{"endpoint":"https://8.8.8.8"}}`
	const entra = `{"value":"","azure_key_config":{"endpoint":"https://8.8.8.8","client_id":"` + masked + `","client_secret":"` + masked + `","tenant_id":"` + masked + `"}}`
	tests := []struct {
		name    string
		stored  string
		update  string
		wantErr bool
	}{
		{
			name:   "API key to Entra and new endpoint",
			stored: api,
			update: `{"value":"","azure_key_config":{"endpoint":"https://8.8.4.4","client_id":"client","client_secret":"secret","tenant_id":"tenant"}}`,
		},
		{
			name:   "Entra to API key and new endpoint",
			stored: entra,
			update: `{"value":"new-key","azure_key_config":{"endpoint":"https://8.8.4.4","client_id":"","client_secret":"","tenant_id":""}}`,
		},
		{
			name:   "Entra to server identity and new endpoint",
			stored: entra,
			update: `{"value":"","azure_key_config":{"endpoint":"https://8.8.4.4","client_id":"","client_secret":"","tenant_id":""}}`,
		},
		{
			name:   "API key to server identity and new endpoint",
			stored: api,
			update: `{"value":"","azure_key_config":{"endpoint":"https://8.8.4.4","client_id":"","client_secret":"","tenant_id":""}}`,
		},
		{
			name:    "omitted Entra credentials are not cleared",
			stored:  entra,
			update:  `{"value":"new-key","azure_key_config":{"endpoint":"https://8.8.4.4"}}`,
			wantErr: true,
		},
		{
			name:    "retained masked API key is still rejected",
			stored:  api,
			update:  `{"value":"` + masked + `","azure_key_config":{"endpoint":"https://8.8.4.4"}}`,
			wantErr: true,
		},
		{
			name:    "empty reference is not an explicit removal",
			stored:  api,
			update:  `{"value":{"value":"","ref":"vault.hidden","type":"vault"},"azure_key_config":{"endpoint":"https://8.8.4.4"}}`,
			wantErr: true,
		},
		{
			name:    "alternate-case duplicate cannot disguise a retained credential",
			stored:  api,
			update:  `{"value":"","Value":"` + masked + `","azure_key_config":{"endpoint":"https://8.8.4.4"}}`,
			wantErr: true,
		},
		{
			name:    "alternate-case nested credential cannot disguise retention",
			stored:  entra,
			update:  `{"value":"new-key","azure_key_config":{"endpoint":"https://8.8.4.4","client_id":"","client_secret":"","Client_Secret":"` + masked + `","tenant_id":""}}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tt.update), &fields); err != nil {
				t.Fatal(err)
			}
			err := e.guardKey(ctx, "azure", fields, []byte(tt.stored))
			if (err != nil) != tt.wantErr {
				t.Fatalf("guardKey error = %v, want error = %v", err, tt.wantErr)
			}
		})
	}
}
