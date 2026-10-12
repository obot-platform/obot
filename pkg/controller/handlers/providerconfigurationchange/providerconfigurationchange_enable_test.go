package providerconfigurationchange

import (
	"testing"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnableSCIMMovesADirectorySynchronizedProviderToSCIM(t *testing.T) {
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, directorySettings()))
	require.Nil(t, s.connection())
	revision := s.daemonRevision()

	require.Empty(t, s.apply(v1.ProviderDesiredStateMigrated, nil))

	// The connection is created without a token, which the API issues once the change is applied.
	conn := s.connection()
	require.NotNil(t, conn)
	assert.Equal(t, gatewaytypes.SCIMConnectionOriginMigrated, conn.Origin)
	assert.Equal(t, gatewaytypes.SCIMConnectionStateConnected, conn.State)
	assert.Equal(t, "https://example.okta.com", conn.Issuer)
	assert.False(t, conn.HasToken())
	// Nothing the daemon runs with changed, so it is not restarted. The directory parameters stay stored, unused,
	// until the administrator removes them.
	assert.Equal(t, revision, s.daemonRevision())
	assert.Equal(t, "service-client", s.credential(oktaProviderName)[oktaServiceClientParam])
	assert.True(t, s.status().Configured)

	// Sign-ins no longer ask the directory for groups.
	requests, providerURL := directoryStub(t)
	s.signIn(providerURL, "00u-owner")
	assert.Zero(t, requests.Load())

	// A retry of the change finds the connection that it created.
	require.Empty(t, s.apply(v1.ProviderDesiredStateMigrated, nil))
	assert.Equal(t, conn.ID, s.connection().ID)

	// Once SCIM is enabled, the directory parameters may be kept.
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, directorySettings()))
	assert.Equal(t, "service-client", s.credential(oktaProviderName)[oktaServiceClientParam])
	assert.Equal(t, conn.ID, s.connection().ID)

	// The provider can be configured again without them, and its daemon restarts without them.
	revision = s.daemonRevision()
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	assert.Greater(t, s.daemonRevision(), revision)
	stored := s.credential(oktaProviderName)
	assert.Equal(t, "oidc-client", stored[oktaClientIDParam])
	assert.NotContains(t, stored, oktaServiceClientParam)
	assert.NotContains(t, stored, oktaServiceKeyParam)
	assert.True(t, s.status().Configured)
	configured, err := s.handler.dispatcher.GetConfiguredAuthProvider(t.Context())
	require.NoError(t, err)
	assert.Equal(t, oktaProviderName, configured)
	assert.Equal(t, conn.ID, s.connection().ID)
}

func TestEnableSCIMIsRefusedUnlessTheProviderSynchronizesItsDirectory(t *testing.T) {
	tests := []struct {
		name string
		// setup prepares the installation, and configures the Okta provider or not.
		setup     func(s *scimChangeTest)
		wantError string
	}{
		{
			name: "another auth provider is configured",
			setup: func(s *scimChangeTest) {
				s.activateOtherProvider()
			},
			wantError: "only be enabled for the configured auth provider",
		},
		{
			name: "a replacement is staged",
			setup: func(s *scimChangeTest) {
				require.Empty(s.t, s.apply(v1.ProviderDesiredStateConfigured, directorySettings()))
				require.NoError(s.t, s.gateway.UpsertCredential(s.t.Context(), gatewaytypes.Credential{
					Context: system.ReplacementAuthProviderCredentialContext,
					Name:    activeProviderName,
					Secrets: map[string]string{
						activeProviderParameter: "secret",
					},
				}))
			},
			wantError: "is staged",
		},
		{
			name: "a cleanup of the provider's group ID prefix is pending",
			setup: func(s *scimChangeTest) {
				require.Empty(s.t, s.apply(v1.ProviderDesiredStateConfigured, directorySettings()))
				require.NoError(s.t, s.client.Create(s.t.Context(), &v1.AuthProviderCleanup{
					Name:      "cleanup",
					Namespace: system.DefaultNamespace,
					Spec: v1.AuthProviderCleanupSpec{
						AuthProviderName: "custom-auth-provider",
						GroupIDPrefix:    "okta/",
					},
				}))
			},
			wantError: "still being cleaned up",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSCIMChangeTest(t)
			tt.setup(s)

			assert.Contains(t, s.apply(v1.ProviderDesiredStateMigrated, nil), tt.wantError)
			assert.Nil(t, s.connection())
		})
	}
}

func TestEnableSCIMCarriesNoSettings(t *testing.T) {
	s := newSCIMChangeTest(t)
	require.Empty(t, s.apply(v1.ProviderDesiredStateConfigured, directorySettings()))

	assert.Contains(t, s.apply(v1.ProviderDesiredStateMigrated, oidcSettings()), "must not reference a staged credential")
	assert.Nil(t, s.connection())
}
