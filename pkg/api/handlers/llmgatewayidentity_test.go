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
		{
			name:         "a proxy CA on Vertex",
			providerType: "vertex",
			body:         `{"proxy_config":{"type":"http","url":"http://proxy.corp:3128","ca_cert_pem":"-----BEGIN CERTIFICATE-----"}}`,
			wantErr:      true,
		},
		{
			name:         "a proxy CA as a secret object on Bedrock",
			providerType: "bedrock",
			body:         `{"proxy_config":{"type":"http","url":"http://proxy.corp:3128","ca_cert_pem":{"value":"-----BEGIN CERTIFICATE-----","type":"plain_text"}}}`,
			wantErr:      true,
		},
		{
			name:         "a proxy CA in another case on Azure",
			providerType: "azure",
			body:         `{"Proxy_Config":{"type":"http","url":"http://proxy.corp:3128","CA_CERT_PEM":"-----BEGIN CERTIFICATE-----"}}`,
			wantErr:      true,
		},
		{
			name:         "a proxy CA beside an empty one in another case",
			providerType: "vertex",
			body:         `{"proxy_config":{"ca_cert_pem":"","Ca_Cert_Pem":"-----BEGIN CERTIFICATE-----"}}`,
			wantErr:      true,
		},
		{
			name:         "proxy certificate checks off on Vertex",
			providerType: "vertex",
			body:         `{"proxy_config":{"type":"http","url":"https://proxy.corp:3128","skip_tls_verify":true}}`,
			wantErr:      true,
		},
		{
			name:         "network settings spelled twice",
			providerType: "bedrock",
			body:         `{"network_config":{},"NETWORK_CONFIG":{"insecure_skip_verify":true}}`,
			wantErr:      true,
		},
		{
			name:         "a proxy without a CA on Vertex, as the egress proxy is set",
			providerType: "vertex",
			body:         `{"proxy_config":{"type":"http","url":{"value":"http://proxy.corp:3128","type":"plain_text"}}}`,
		},
		{
			name:         "a proxy turned off on Azure",
			providerType: "azure",
			body:         `{"proxy_config":{"type":"none","ca_cert_pem":""}}`,
		},
		{
			name:         "proxy settings on Vertex as the gateway lists them",
			providerType: "vertex",
			body:         `{"network_config":{"ca_cert_pem":null},"proxy_config":{"type":"none","url":null,"username":null,"password":null,"ca_cert_pem":null}}`,
		},
		{
			name:         "a proxy CA on a provider with its own keys",
			providerType: "openai",
			body:         `{"proxy_config":{"type":"http","url":"http://proxy.corp:3128","ca_cert_pem":"-----BEGIN CERTIFICATE-----"}}`,
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

func TestRejectSubscriptionRedirects(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "a proxy CA",
			body:    `{"proxy_config":{"type":"http","url":"http://proxy.corp:3128","ca_cert_pem":"-----BEGIN CERTIFICATE-----"}}`,
			wantErr: true,
		},
		{
			name:    "a proxy CA as a secret object in another case",
			body:    `{"PROXY_CONFIG":{"type":"http","url":"http://proxy.corp:3128","Ca_Cert_Pem":{"value":"-----BEGIN CERTIFICATE-----"}}}`,
			wantErr: true,
		},
		{
			name:    "a provider created with a proxy CA",
			body:    `{"provider":"anthropic","proxy_config":{"type":"http","url":"http://proxy.corp:3128","ca_cert_pem":"-----BEGIN CERTIFICATE-----"}}`,
			wantErr: true,
		},
		{
			name:    "proxy certificate checks off",
			body:    `{"proxy_config":{"type":"http","url":"https://proxy.corp:3128","skip_tls_verify":true}}`,
			wantErr: true,
		},
		{
			name: "a proxy without a CA, as the egress proxy is set",
			body: `{"proxy_config":{"type":"http","url":{"value":"http://proxy.corp:3128","type":"plain_text"}}}`,
		},
		{
			name: "a proxy turned off",
			body: `{"proxy_config":{"type":"none"}}`,
		},
		{
			name: "ordinary network settings",
			body: `{"network_config":{"default_request_timeout_in_seconds":60}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectSubscriptionRedirects([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRejectGlobalProxySkipVerify(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		body    string
		wantErr bool
	}{
		{
			name:    "certificate checks off",
			path:    "proxy-config",
			body:    `{"enabled":true,"type":"http","url":"https://proxy.corp:3128","skip_tls_verify":true}`,
			wantErr: true,
		},
		{
			name:    "certificate checks off in another case",
			path:    "proxy-config",
			body:    `{"enabled":true,"type":"http","url":"https://proxy.corp:3128","Skip_TLS_Verify":true}`,
			wantErr: true,
		},
		{
			name:    "certificate checks off with the proxy disabled",
			path:    "proxy-config",
			body:    `{"enabled":false,"skip_tls_verify":true}`,
			wantErr: true,
		},
		{
			name:    "certificate checks off beside a false spelling",
			path:    "proxy-config",
			body:    `{"skip_tls_verify":false,"SKIP_TLS_VERIFY":true}`,
			wantErr: true,
		},
		{
			name: "a proxy with certificate checks on",
			path: "proxy-config",
			body: `{"enabled":true,"type":"http","url":"http://proxy.corp:3128","skip_tls_verify":false,"enable_for_inference":true}`,
		},
		{
			name: "a proxy without the setting",
			path: "proxy-config",
			body: `{"enabled":true,"type":"http","url":"http://proxy.corp:3128"}`,
		},
		{
			name: "another API",
			path: "providers/openai",
			body: `{"skip_tls_verify":true}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectGlobalProxySkipVerify(tt.path, []byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsAnthropicProvider(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		want bool
	}{
		{
			name: "updating the provider",
			path: "providers/anthropic",
			body: `{"proxy_config":{"type":"none"}}`,
			want: true,
		},
		{
			name: "creating the provider",
			path: "providers",
			body: `{"provider":"anthropic"}`,
			want: true,
		},
		{
			name: "creating the provider with its name spelled twice",
			path: "providers",
			body: `{"provider":"e2e","PROVIDER":"anthropic"}`,
			want: true,
		},
		{
			name: "another provider",
			path: "providers/openai",
			body: `{"provider":"anthropic"}`,
		},
		{
			name: "a custom provider",
			path: "providers",
			body: `{"provider":"claude-proxy","custom_provider_config":{"base_provider_type":"anthropic"}}`,
		},
		{
			name: "another API",
			path: "proxy-config",
			body: `{"provider":"anthropic"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAnthropicProvider(tt.path, []byte(tt.body)); got != tt.want {
				t.Fatalf("isAnthropicProvider = %v, want %v", got, tt.want)
			}
		})
	}
}
