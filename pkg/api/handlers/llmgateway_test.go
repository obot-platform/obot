package handlers

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestRejectUnsafeConfig(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "plain key",
			body: `{"name":"k","value":"sk-123","models":["*"]}`,
		},
		{
			name:    "env reference",
			body:    `{"name":"k","value":"env.OBOT_SERVER_DSN"}`,
			wantErr: true,
		},
		{
			name:    "nested vault reference",
			body:    `{"azure_key_config":{"endpoint":"vault.secret/path"}}`,
			wantErr: true,
		},
		{
			name:    "object env reference",
			body:    `{"value":{"ref":"HOME","type":"env"}}`,
			wantErr: true,
		},
		{
			name:    "from_env flag",
			body:    `{"value":{"env_var":"HOME","from_env":true}}`,
			wantErr: true,
		},
		{
			name:    "reference in array",
			body:    `{"headers":["env.SECRET"]}`,
			wantErr: true,
		},
		{
			name:    "type in another case",
			body:    `{"name":"k","value":{"value":"","ref":"OBOT_SERVER_DSN","TYPE":"env"}}`,
			wantErr: true,
		},
		{
			name:    "from_env in another case",
			body:    `{"name":"k","value":{"value":"","env_var":"OBOT_SERVER_DSN","FROM_ENV":true}}`,
			wantErr: true,
		},
		{
			name:    "escaped env reference",
			body:    `{"name":"k","value":"\u0065nv.OBOT_SERVER_DSN"}`,
			wantErr: true,
		},
		{
			name:    "vault reference by prefix",
			body:    `{"keys":[{"value":{"value":"","ref":"vault.secret/path"}}]}`,
			wantErr: true,
		},
		{
			name:    "from_env without a variable",
			body:    `{"name":"k","value":{"value":"x","From_Env":true}}`,
			wantErr: true,
		},
		{
			name: "a key as the gateway lists it",
			body: `{"name":"k","value":{"value":"sk-p************************abcd","type":"plain_text"},"models":["*"]}`,
		},
		{
			name: "region",
			body: `{"vertex_key_config":{"project_id":"p","region":"us-central1"}}`,
		},
		{
			name:    "region that changes the host",
			body:    `{"vertex_key_config":{"project_id":"p","region":"attacker.example/x?"}}`,
			wantErr: true,
		},
		{
			name:    "region object that changes the host",
			body:    `{"vertex_key_config":{"project_id":"p","Region":{"value":"attacker.example#"}}}`,
			wantErr: true,
		},
		{
			name: "not json",
			body: `hello`,
		},
		{
			name: "Vertex service account key",
			body: `{"name":"k","vertex_key_config":{"project_id":"p","region":"us-central1","auth_credentials":"{\"type\":\"service_account\",\"client_email\":\"a@p.iam.gserviceaccount.com\"}"}}`,
		},
		{
			name: "Vertex service account key as an object",
			body: `{"name":"k","vertex_key_config":{"auth_credentials":{"value":"{\"type\":\"service_account\"}","type":"plain_text"}}}`,
		},
		{
			name: "Vertex without credentials uses the server's identity",
			body: `{"name":"k","vertex_key_config":{"project_id":"p","region":"us-central1","auth_credentials":""}}`,
		},
		{
			name: "Vertex credentials left null",
			body: `{"name":"k","vertex_key_config":{"project_id":"p","region":"us-central1","auth_credentials":null}}`,
		},
		{
			name: "Vertex credentials as the gateway lists them",
			body: `{"name":"k","vertex_key_config":{"auth_credentials":{"value":"{\"ty************************\"}\n}","type":"plain_text"}}}`,
		},
		{
			name:    "Vertex external account credentials",
			body:    `{"name":"k","vertex_key_config":{"auth_credentials":"{\"type\":\"external_account\",\"credential_source\":{\"file\":\"/etc/passwd\"}}"}}`,
			wantErr: true,
		},
		{
			name:    "Vertex authorized user credentials",
			body:    `{"name":"k","vertex_key_config":{"auth_credentials":"{\"type\":\"authorized_user\"}"}}`,
			wantErr: true,
		},
		{
			name:    "Vertex impersonated credentials in a provider's keys",
			body:    `{"provider":"vertex","keys":[{"name":"k","vertex_key_config":{"auth_credentials":"{\"type\":\"impersonated_service_account\"}"}}]}`,
			wantErr: true,
		},
		{
			name:    "Vertex credentials without a type",
			body:    `{"name":"k","vertex_key_config":{"auth_credentials":"{\"client_email\":\"a@p\"}"}}`,
			wantErr: true,
		},
		{
			name:    "Vertex credentials that aren't JSON",
			body:    `{"name":"k","vertex_key_config":{"auth_credentials":"not json"}}`,
			wantErr: true,
		},
		{
			name:    "Vertex credentials as an object holding another type",
			body:    `{"name":"k","vertex_key_config":{"auth_credentials":{"value":"{\"type\":\"external_account\"}"}}}`,
			wantErr: true,
		},
		{
			name:    "Vertex field names in another case",
			body:    `{"name":"k","Vertex_Key_Config":{"Auth_Credentials":"{\"type\":\"external_account\"}"}}`,
			wantErr: true,
		},
		{
			name:    "Vertex credentials with a second type in another case",
			body:    `{"name":"k","vertex_key_config":{"auth_credentials":"{\"type\":\"service_account\",\"TYPE\":\"external_account\"}"}}`,
			wantErr: true,
		},
		{
			name:    "Vertex credentials with a duplicate type",
			body:    `{"name":"k","vertex_key_config":{"auth_credentials":"{\"type\":\"external_account\",\"type\":\"service_account\"}"}}`,
			wantErr: true,
		},
		{
			name:    "Vertex credentials with an escaped type field",
			body:    `{"name":"k","vertex_key_config":{"auth_credentials":"{\"typ\\u0065\":\"external_account\"}"}}`,
			wantErr: true,
		},
		{
			name:    "Vertex credentials beside empty ones in another case",
			body:    `{"name":"k","vertex_key_config":{"auth_credentials":"","AUTH_CREDENTIALS":"{\"type\":\"external_account\"}"}}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectUnsafeConfig([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRejectEngineAuthConfig(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "auth config on config",
			body:    `{"auth_config":{"is_enabled":true,"admin_username":"a","admin_password":"b"}}`,
			wantErr: true,
		},
		{
			name:    "auth config in another case",
			body:    `{"Auth_Config":{"is_enabled":true}}`,
			wantErr: true,
		},
		{
			name:    "client config only",
			body:    `{"client_config":{"drop_excess_requests":true}}`,
			wantErr: false,
		},
		{
			name:    "null auth config",
			body:    `{"client_config":{},"auth_config":null}`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectEngineAuthConfig([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("rejectEngineAuthConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientConfigOnly(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{
			name: "framework config is dropped and retention raised",
			body: `{"client_config":{"drop_excess_requests":true,"log_retention_days":0},"framework_config":{"pricing_url":"file:///data/pricing.json"}}`,
			want: `{"client_config":{"drop_excess_requests":true,"log_retention_days":1}}`,
		},
		{
			name: "missing retention gets the minimum",
			body: `{"client_config":{"max_request_body_size_mb":20}}`,
			want: `{"client_config":{"log_retention_days":1,"max_request_body_size_mb":20}}`,
		},
		{
			name: "retention of a day or more is kept",
			body: `{"client_config":{"log_retention_days":7}}`,
			want: `{"client_config":{"log_retention_days":7}}`,
		},
		{
			name:    "no client config",
			body:    `{"framework_config":{}}`,
			wantErr: true,
		},
		{
			name:    "not JSON",
			body:    `nope`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := clientConfigOnly([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("clientConfigOnly() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && string(got) != tt.want {
				t.Fatalf("clientConfigOnly() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestWithProviderDefaults(t *testing.T) {
	// A new OpenAI-compatible provider whose base URL ends in /v1 gets it without, since the
	// gateway adds /v1 to every path, and gets the default retries.
	body, err := withProviderDefaults([]byte(`{"provider":"vllm-local","network_config":{"base_url":"http://vllm:8000/v1"},"custom_provider_config":{"base_provider_type":"openai"}}`), true, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if got := gjson.GetBytes(body, "network_config.base_url").String(); got != "http://vllm:8000" {
		t.Fatalf("base_url = %q", got)
	}
	if !gjson.GetBytes(body, "network_config.max_retries").Exists() {
		t.Fatalf("defaults not applied: %s", body)
	}

	// An update gets the same base URL, and no defaults.
	body, err = withProviderDefaults([]byte(`{"network_config":{"base_url":"http://vllm:8000/v1/"}}`), false, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if got := gjson.GetBytes(body, "network_config.base_url").String(); got != "http://vllm:8000" || gjson.GetBytes(body, "network_config.max_retries").Exists() {
		t.Fatalf("update body %s", body)
	}

	// An update with nothing to change is sent as it was.
	update := `{"network_config":{"base_url":"https://api.openai.com"},"keys":[]}`
	if body, err = withProviderDefaults([]byte(update), false, "openai"); err != nil || string(body) != update {
		t.Fatalf("update body %s, %v", body, err)
	}
}
