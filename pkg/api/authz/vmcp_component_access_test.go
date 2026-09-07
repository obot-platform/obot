package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestCheckVMCPComponentAccessUsesOwner(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		Name:      "entry",
		Namespace: system.DefaultNamespace,
		Spec:      v1.MCPServerCatalogEntrySpec{MCPCatalogName: system.DefaultCatalog},
	}
	for _, tc := range []struct {
		name       string
		ownerID    string
		allowedID  string
		wantError  string
		wantLookup bool
	}{
		{
			name:       "admin access does not grant owner access",
			ownerID:    "2",
			allowedID:  "1",
			wantError:  "access denied",
			wantLookup: true,
		},
		{
			name:       "owner access permits admin edit without admin catalog access",
			ownerID:    "2",
			allowedID:  "2",
			wantLookup: true,
		},
		{
			name:      "owner editing own resource needs no lookup",
			ownerID:   "1",
			allowedID: "1",
		},
		{
			name: "shared resource needs no owner catalog access",
		},
		{
			name:      "invalid owner fails closed",
			ownerID:   "invalid",
			wantError: "invalid VMCP owner ID",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authorizer := newCatalogEntryTestAuthorizer(t, nil, &v1.AccessControlRule{
				Name:      "access",
				Namespace: system.DefaultNamespace,
				Spec: v1.AccessControlRuleSpec{
					MCPCatalogID: system.DefaultCatalog,
					Manifest: types.AccessControlRuleManifest{
						Subjects:  []types.Subject{{Type: types.SubjectTypeUser, ID: tc.allowedID}},
						Resources: []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name}},
					},
				},
			})
			lookedUp := false
			err := CheckVMCPComponentAccess(t.Context(), &user.DefaultInfo{UID: "1", Groups: []string{types.GroupAdmin}}, tc.ownerID, entry, authorizer.acrHelper,
				func(_ context.Context, id uint) (user.Info, error) {
					lookedUp = true
					require.Equal(t, uint(2), id)
					return &user.DefaultInfo{UID: "2"}, nil
				})
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantLookup, lookedUp)
		})
	}

	t.Run("owner lookup failure fails closed", func(t *testing.T) {
		lookupErr := errors.New("user not found")
		err := CheckVMCPComponentAccess(t.Context(), &user.DefaultInfo{UID: "1"}, "2", entry, nil,
			func(context.Context, uint) (user.Info, error) { return nil, lookupErr })
		require.ErrorIs(t, err, lookupErr)
	})
}
