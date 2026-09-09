package systemmcpserver

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
)

func TestIsSystemServerConfiguredWithRequiredStaticConfiguration(t *testing.T) {
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
	require.Empty(t, server.Spec.Manifest.Env[0].Value)
	require.True(t, isSystemServerConfigured(server, staticSecrets))
}
