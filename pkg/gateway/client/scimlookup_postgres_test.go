package client

import (
	"fmt"
	"sync"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	"github.com/obot-platform/obot/pkg/scim/adapter/adaptertest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestSignInAndSCIMCreateOfOnePersonMakeOneUserOnPostgres starts a first sign-in and a SCIM create of the same person
// at once, for an adapter that matches users by group lookup ID. Neither can find the other's identity by the key it
// knows, so only the lock they share keeps them from creating two users.
func TestSignInAndSCIMCreateOfOnePersonMakeOneUserOnPostgres(t *testing.T) {
	adapter.RegisterForTest(t, lookupTestProviderName, adaptertest.Adapter{
		Users:  adapter.UserKeyGroupLookupID,
		Groups: adapter.GroupKeyExternalID,
	})
	c := newPostgresLifecycleTestClient(t)
	c.storageClient = fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&v1.UserDefaultRoleSetting{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultRoleSettingName,
			Spec: v1.UserDefaultRoleSettingSpec{
				Role: apitypes.RoleBasic,
			},
		}).
		Build()
	require.NoError(t, c.BackfillIdentityGroupLookupIDHashes(t.Context()))
	stub := newLookupIDStub(t)
	conn := createLookupTestConnection(t, c)

	for i := range 20 {
		objectID := fmt.Sprintf("oid-race-%d", i)
		sub := fmt.Sprintf("sub-race-%d", i)

		var (
			wg                 sync.WaitGroup
			start              = make(chan struct{})
			signedIn           *types.User
			provisioned        *SCIMUser
			signInErr, scimErr error
		)
		wg.Go(func() {
			<-start
			signedIn, signInErr = lookupSignIn(t, c, stub, sub, objectID)
		})
		wg.Go(func() {
			<-start
			provisioned, scimErr = provisionLookupTestUser(t, c, conn, objectID)
		})
		close(start)
		wg.Wait()

		require.NoError(t, signInErr, objectID)
		require.NoError(t, scimErr, objectID)
		require.Equal(t, signedIn.ID, provisioned.UserID, objectID)

		// One identity remains: the one sign-in created, or the one SCIM created, claimed.
		require.EqualValues(t, 1, countRows(t, c, new(types.Identity), "hashed_provider_group_lookup_id = ?", hash.String(objectID)), objectID)
		require.EqualValues(t, 1, countRows(t, c, new(types.Identity), "hashed_provider_user_id = ?", hash.String(sub)), objectID)
	}
}
