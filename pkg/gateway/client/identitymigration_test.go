package client

import (
	"crypto/aes"
	"testing"

	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/auth"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
	encryptaes "k8s.io/apiserver/pkg/storage/value/encrypt/aes"
)

// encryptResources makes c encrypt the given resources with a real transformer.
func encryptResources(t *testing.T, c *Client, resources ...schema.GroupResource) {
	t.Helper()

	block, err := aes.NewCipher(make([]byte, 32))
	require.NoError(t, err)
	transformer, err := encryptaes.NewGCMTransformer(block)
	require.NoError(t, err)
	transformers := make(map[schema.GroupResource]value.Transformer, len(resources))
	for _, resource := range resources {
		transformers[resource] = transformer
	}
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: transformers,
	}
}

// insertIdentity stores an identity of the lifecycle test provider as a write before the hashed group lookup ID
// existed did: encrypted when c encrypts identities, and without the hash.
func insertIdentity(t *testing.T, c *Client, providerUserID, lookupID string) {
	t.Helper()

	identity := &gatewaytypes.Identity{
		AuthProviderNamespace: lifecycleTestProvider.Namespace,
		AuthProviderName:      lifecycleTestProvider.Name,
		ProviderUsername:      providerUserID,
		ProviderUserID:        providerUserID,
		HashedProviderUserID:  hash.String(providerUserID),
		ProviderGroupLookupID: lookupID,
	}
	require.NoError(t, c.encryptIdentity(t.Context(), identity))
	identity.HashedProviderGroupLookupID = ""
	require.NoError(t, c.db.WithContext(t.Context()).Create(identity).Error)
}

func TestEncryptIdentitySetsTheHashedGroupLookupID(t *testing.T) {
	tests := []struct {
		name     string
		encrypt  bool
		lookupID string
		want     string
	}{
		{
			name:     "without encryption",
			lookupID: "00u-lookup",
			want:     hash.String("00u-lookup"),
		},
		{
			name:     "with encryption, the hash is of the plaintext",
			encrypt:  true,
			lookupID: "00u-lookup",
			want:     hash.String("00u-lookup"),
		},
		{
			name: "an empty lookup ID has no hash",
		},
		{
			name:    "an empty lookup ID has no hash with encryption",
			encrypt: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			if tt.encrypt {
				encryptResources(t, c, identityGroupResource)
			}
			identity := &gatewaytypes.Identity{
				ProviderGroupLookupID: tt.lookupID,
				// A stale hash is replaced.
				HashedProviderGroupLookupID: "stale",
			}
			require.NoError(t, c.encryptIdentity(t.Context(), identity))
			require.Equal(t, tt.want, identity.HashedProviderGroupLookupID)
		})
	}
}

func TestIdentityWritesStoreTheHashedGroupLookupID(t *testing.T) {
	c := newLifecycleTestClient(t)
	encryptResources(t, c, identityGroupResource)
	_, srv := newAuthProviderStub(t)
	ctx := accesstoken.ContextWithAccessToken(auth.ContextWithProviderGroupIDPrefix(auth.ContextWithProviderURL(t.Context(), srv.URL), "okta/"), "access-token")
	unlimited := UserLimit{
		Unlimited: true,
	}

	// A first sign-in fills in the lookup ID from the auth provider's user info.
	signedIn, err := c.EnsureIdentity(ctx, signInIdentity("00u-signed-in", "signed-in@example.com"), "", unlimited)
	require.NoError(t, err)
	require.Equal(t, hash.String("00u-lookup"), storedIdentity(t, c, lifecycleTestProvider, "00u-signed-in").HashedProviderGroupLookupID)

	// A profile refresh writes the lookup ID that the auth provider reports.
	require.NoError(t, c.db.WithContext(ctx).Model(new(gatewaytypes.Identity)).Where("user_id = ?", signedIn.ID).
		UpdateColumn("hashed_provider_group_lookup_id", "").Error)
	user := storedLifecycleUser(t, c, signedIn.ID)
	require.NoError(t, c.decryptUser(ctx, &user))
	user.DisplayName = ""
	require.NoError(t, c.UpdateProfileIfNeeded(ctx, &user, lifecycleTestProvider.Name, lifecycleTestProvider.Namespace, srv.URL))
	require.Equal(t, hash.String("00u-lookup"), storedIdentity(t, c, lifecycleTestProvider, "00u-signed-in").HashedProviderGroupLookupID)

	// Re-encrypting every identity keeps the hash.
	require.NoError(t, c.EncryptIdentities(ctx, true))
	require.Equal(t, hash.String("00u-lookup"), storedIdentity(t, c, lifecycleTestProvider, "00u-signed-in").HashedProviderGroupLookupID)

	// SCIM creates an identity with the adapter's lookup ID, which for Okta is the native user ID.
	conn, _ := createTestSCIMConnection(t, c, true)
	provisionTestSCIMUser(t, c, conn, "00u-provisioned", "provisioned@example.com")
	require.Equal(t, hash.String("00u-provisioned"), storedIdentity(t, c, lifecycleTestProvider, "00u-provisioned").HashedProviderGroupLookupID)
}

func TestBackfillIdentityGroupLookupIDHashes(t *testing.T) {
	testBackfillIdentityGroupLookupIDHashes(t, newTestClient(t))
}

func TestBackfillIdentityGroupLookupIDHashesPostgres(t *testing.T) {
	testBackfillIdentityGroupLookupIDHashes(t, newPostgresLifecycleTestClient(t))
}

func testBackfillIdentityGroupLookupIDHashes(t *testing.T, c *Client) {
	t.Helper()

	insertIdentity(t, c, "00u-plain", "lookup-plain")
	encryptResources(t, c, identityGroupResource)
	for _, id := range []string{"00u-a", "00u-b", "00u-c", "00u-d", "00u-e"} {
		insertIdentity(t, c, id, "lookup-"+id)
	}
	insertIdentity(t, c, "00u-none", "")
	// An identity whose hash is already stored keeps it.
	insertIdentity(t, c, "00u-hashed", "lookup-hashed")
	require.NoError(t, c.db.WithContext(t.Context()).Model(new(gatewaytypes.Identity)).
		Where("hashed_provider_user_id = ?", hash.String("00u-hashed")).
		UpdateColumn("hashed_provider_group_lookup_id", "kept").Error)

	// A batch smaller than the identities makes the backfill page through them.
	require.NoError(t, c.backfillIdentityGroupLookupIDHashes(t.Context(), 2, nil))

	want := map[string]string{
		"00u-plain":  hash.String("lookup-plain"),
		"00u-a":      hash.String("lookup-00u-a"),
		"00u-b":      hash.String("lookup-00u-b"),
		"00u-c":      hash.String("lookup-00u-c"),
		"00u-d":      hash.String("lookup-00u-d"),
		"00u-e":      hash.String("lookup-00u-e"),
		"00u-none":   "",
		"00u-hashed": "kept",
	}
	check := func() {
		t.Helper()
		for id, wantHash := range want {
			require.Equal(t, wantHash, storedIdentity(t, c, lifecycleTestProvider, id).HashedProviderGroupLookupID, id)
		}
	}
	check()

	// Running it again changes nothing.
	require.NoError(t, c.backfillIdentityGroupLookupIDHashes(t.Context(), 2, nil))
	check()
}

func TestBackfillIdentityGroupLookupIDHashesRunsOnce(t *testing.T) {
	c := newTestClient(t)
	insertIdentity(t, c, "00u-first", "lookup-first")
	require.NoError(t, c.BackfillIdentityGroupLookupIDHashes(t.Context()))
	require.Equal(t, hash.String("lookup-first"), storedIdentity(t, c, lifecycleTestProvider, "00u-first").HashedProviderGroupLookupID)

	// Once the migration is recorded, it does not run again.
	insertIdentity(t, c, "00u-later", "lookup-later")
	require.NoError(t, c.BackfillIdentityGroupLookupIDHashes(t.Context()))
	require.Empty(t, storedIdentity(t, c, lifecycleTestProvider, "00u-later").HashedProviderGroupLookupID)
}

func TestBackfillIdentityGroupLookupIDHashesRefusesEncryptedIdentitiesWithoutEncryption(t *testing.T) {
	c := newTestClient(t)
	encryptResources(t, c, identityGroupResource)
	insertIdentity(t, c, "00u-encrypted", "lookup-encrypted")
	c.encryptionConfig = nil

	require.ErrorContains(t, c.BackfillIdentityGroupLookupIDHashes(t.Context()), "encryption is not configured")
	require.Empty(t, storedIdentity(t, c, lifecycleTestProvider, "00u-encrypted").HashedProviderGroupLookupID)

	// The migration is not recorded, so it runs again once encryption is configured.
	encryptResources(t, c, identityGroupResource)
	require.NoError(t, c.BackfillIdentityGroupLookupIDHashes(t.Context()))
	require.Equal(t, hash.String("lookup-encrypted"), storedIdentity(t, c, lifecycleTestProvider, "00u-encrypted").HashedProviderGroupLookupID)
}
