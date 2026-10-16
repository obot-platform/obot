package handlers

import (
	"testing"
)

func TestRejectAmbientCredentialRedirects(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		body         string
		wantErr      bool
	}{
		{
			name:         "Azure key with managed identity and an Azure endpoint",
			providerType: "azure",
			body:         `{"name":"k","value":"","azure_key_config":{"endpoint":"https://res.openai.azure.com"}}`,
		},
		{
			name:         "Azure key with managed identity and another endpoint",
			providerType: "azure",
			body:         `{"name":"k","value":"","azure_key_config":{"endpoint":"https://collector.example.com"}}`,
			wantErr:      true,
		},
		{
			name:         "a host that only ends like Azure's",
			providerType: "azure",
			body:         `{"name":"k","azure_key_config":{"endpoint":"https://evilopenai.azure.com.example.com"}}`,
			wantErr:      true,
		},
		{
			name:         "a model alias pointing the managed identity elsewhere",
			providerType: "azure",
			body:         `{"name":"k","value":"","azure_key_config":{"endpoint":"https://res.openai.azure.com"},"aliases":{"gpt":{"model":"gpt-4o","config":{"azure":{"endpoint":"https://collector.example.com"}}}}}`,
			wantErr:      true,
		},
		{
			name:         "managed identity asking for another scope",
			providerType: "azure",
			body:         `{"name":"k","value":"","azure_key_config":{"endpoint":"https://res.openai.azure.com","scopes":["https://management.azure.com/.default"]}}`,
			wantErr:      true,
		},
		{
			name:         "field names in another case",
			providerType: "azure",
			body:         `{"name":"k","Azure_Key_Config":{"Endpoint":"https://collector.example.com"}}`,
			wantErr:      true,
		},
		{
			name:         "an API key may go to any endpoint, such as an API Management gateway",
			providerType: "azure",
			body:         `{"name":"k","value":"azure-api-key","azure_key_config":{"endpoint":"https://gw.azure-api.net"}}`,
		},
		{
			name:         "a service principal may use other scopes",
			providerType: "azure",
			body:         `{"name":"k","azure_key_config":{"endpoint":"https://gw.azure-api.net","client_id":"c","client_secret":"s","tenant_id":"t","scopes":["api://x/.default"]}}`,
		},
		{
			name:         "a provider's keys in a provider update",
			providerType: "azure",
			body:         `{"keys":[{"name":"k","value":"","azure_key_config":{"endpoint":"https://collector.example.com"}}]}`,
			wantErr:      true,
		},
		{
			name:         "certificate checks off on Vertex",
			providerType: "vertex",
			body:         `{"network_config":{"insecure_skip_verify":true}}`,
			wantErr:      true,
		},
		{
			name:         "another CA on Bedrock",
			providerType: "bedrock",
			body:         `{"network_config":{"ca_cert_pem":"-----BEGIN CERTIFICATE-----"}}`,
			wantErr:      true,
		},
		{
			name:         "certificate checks off on a self-hosted provider",
			providerType: "openai",
			body:         `{"network_config":{"base_url":"https://vllm.internal","insecure_skip_verify":true}}`,
		},
		{
			name:         "ordinary network settings on Azure",
			providerType: "azure",
			body:         `{"network_config":{"default_request_timeout_in_seconds":60,"insecure_skip_verify":false}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectAmbientCredentialRedirects([]byte(tt.body), tt.providerType)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
