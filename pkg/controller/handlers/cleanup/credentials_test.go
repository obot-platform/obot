package cleanup

import (
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newCredentialsCleanupGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()

	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)

	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())

	client := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close gateway client: %v", err)
		}
	})
	return client
}

func credentialExists(t *testing.T, client *gatewayclient.Client, credentialContext, name string) bool {
	t.Helper()

	creds, err := client.ListCredentials(t.Context(), gatewayclient.ListCredentialsOptions{
		CredentialContexts: []string{credentialContext},
	})
	require.NoError(t, err)
	for _, cred := range creds {
		if cred.Name == name {
			return true
		}
	}
	return false
}

func TestRemoveAuditLogCred(t *testing.T) {
	tests := []struct {
		name           string
		object         func() kclient.Object
		credentialName string
		wantDeleted    bool
	}{
		{
			name: "server that has not been swept loses the credential",
			object: func() kclient.Object {
				return &v1.MCPServer{
					Name:      "ms1abcdef",
					Namespace: "default",
				}
			},
			credentialName: "ms1abcdef",
			wantDeleted:    true,
		},
		{
			name: "system server uses the secret info name",
			object: func() kclient.Object {
				return &v1.SystemMCPServer{
					Name:      "sms1obot-mcp-server",
					Namespace: "default",
				}
			},
			credentialName: "sms1obot-mcp-server-secret-info",
			wantDeleted:    true,
		},
		{
			name: "server already recorded as swept is left alone",
			object: func() kclient.Object {
				return &v1.MCPServer{
					Name:      "ms1abcdef",
					Namespace: "default",
					Annotations: map[string]string{
						v1.AuditLogCredentialRemovedAnnotation: "true",
					},
				}
			},
			credentialName: "ms1abcdef",
			wantDeleted:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			object := tt.object()
			gatewayClient := newCredentialsCleanupGatewayClient(t)
			require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
				Context: object.GetName(),
				Name:    tt.credentialName,
				Secrets: map[string]string{"AUDIT_LOG_TOKEN": "legacy"},
			}))

			client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(object).Build()
			err := NewCredentials(nil, gatewayClient, "").RemoveAuditLogCred(router.Request{
				Client:    client,
				Ctx:       t.Context(),
				Object:    object,
				Namespace: object.GetNamespace(),
				Name:      object.GetName(),
			}, &router.ResponseWrapper{})
			require.NoError(t, err)

			assert.Equal(t, !tt.wantDeleted, credentialExists(t, gatewayClient, object.GetName(), tt.credentialName))
			assert.Contains(t, object.GetAnnotations(), v1.AuditLogCredentialRemovedAnnotation,
				"a swept server must be recorded so it is never queried again")
		})
	}
}

func TestRemoveAuditLogCredKeepsSweepPendingWhenDeleteFails(t *testing.T) {
	server := &v1.MCPServer{
		Name:      "ms1abcdef",
		Namespace: "default",
	}

	gatewayClient := newCredentialsCleanupGatewayClient(t)
	require.NoError(t, gatewayClient.Close())

	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(server).Build()
	err := NewCredentials(nil, gatewayClient, "").RemoveAuditLogCred(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}, &router.ResponseWrapper{})

	require.Error(t, err)
	assert.NotContains(t, server.GetAnnotations(), v1.AuditLogCredentialRemovedAnnotation,
		"a failed delete must leave the sweep pending")
}
