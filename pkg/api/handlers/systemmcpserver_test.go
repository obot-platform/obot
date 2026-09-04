package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
)

func TestConvertSystemMCPServerConfigurationStatus(t *testing.T) {
	options := []types.MCPConfigurationOption{{Name: "US", Value: "us"}, {Name: "EU", Value: "eu"}}
	server := v1.SystemMCPServer{
		Spec: v1.SystemMCPServerSpec{
			Manifest: types.SystemMCPServerManifest{
				Env: []types.MCPEnv{{Key: "REGION", Required: true, Options: options}},
				RemoteConfig: &types.RemoteRuntimeConfig{Headers: []types.MCPHeader{
					{Key: "TENANT", Required: true, Options: options},
					{Key: "MODE", Options: options},
				}},
			},
		},
	}

	tests := []struct {
		name           string
		credentials    map[string]string
		configured     bool
		missingEnv     []string
		missingHeaders []string
	}{
		{
			name:        "valid selections",
			credentials: map[string]string{"REGION": "us", "TENANT": "eu"},
			configured:  true,
		},
		{
			name:           "missing required header",
			credentials:    map[string]string{"REGION": "us"},
			missingHeaders: []string{"TENANT"},
		},
		{
			name:           "stale optional header selection",
			credentials:    map[string]string{"REGION": "us", "TENANT": "eu", "MODE": "stale"},
			missingHeaders: []string{"MODE"},
		},
		{
			name:        "stale required environment selection",
			credentials: map[string]string{"REGION": "stale", "TENANT": "eu"},
			missingEnv:  []string{"REGION"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converted := convertSystemMCPServer(server, tt.credentials)
			assert.Equal(t, tt.configured, converted.Configured)
			assert.Equal(t, tt.missingEnv, converted.MissingRequiredEnvVars)
			assert.Equal(t, tt.missingHeaders, converted.MissingRequiredHeaders)
		})
	}
}

func TestConvertSystemMCPServerEncryptedStaticConfiguration(t *testing.T) {
	server := v1.SystemMCPServer{
		Spec: v1.SystemMCPServerSpec{
			Manifest: types.SystemMCPServerManifest{
				Env: []types.MCPEnv{{
					Key:       "TOKEN",
					Value:     "secret-token",
					Required:  true,
					Sensitive: true,
				}},
			},
		},
	}
	staticSecrets := mcp.ExtractStaticSystemServerConfiguration(&server.Spec.Manifest, nil, true)

	converted := convertSystemMCPServer(server, staticSecrets)

	assert.True(t, converted.Configured)
	assert.Empty(t, converted.MissingRequiredEnvVars)
	assert.True(t, converted.SystemMCPServerManifest.Env[0].ValueConfigured)
	assert.Empty(t, converted.SystemMCPServerManifest.Env[0].Value)
}
