package client

import (
	"reflect"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestAuthProviderGroupCleanupData(t *testing.T) {
	c := newTestClient(t)
	groups := []types.Group{
		{
			ID:                    "entra/engineering",
			AuthProviderName:      "entra-auth-provider",
			AuthProviderNamespace: "default",
			Name:                  "Engineering",
		},
		{
			ID:                    "entra/security",
			AuthProviderName:      "entra-auth-provider",
			AuthProviderNamespace: "default",
			Name:                  "Security",
		},
		{
			ID:                    "okta/engineering",
			AuthProviderName:      "okta-auth-provider",
			AuthProviderNamespace: "default",
			Name:                  "Engineering",
		},
	}
	memberships := []types.GroupMemberships{
		{
			UserID:  1,
			GroupID: "entra/engineering",
		},
		{
			UserID:  1,
			GroupID: "entra/security",
		},
		{
			UserID:  2,
			GroupID: "entra/security",
		},
		{
			UserID:  3,
			GroupID: "okta/engineering",
		},
	}
	if err := c.db.WithContext(t.Context()).Create(&groups).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.WithContext(t.Context()).Create(&memberships).Error; err != nil {
		t.Fatal(err)
	}

	groupIDs, userIDs, err := c.GetAuthProviderGroupCleanupData(t.Context(), "default", "entra-auth-provider")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"entra/engineering", "entra/security"}; !reflect.DeepEqual(groupIDs, want) {
		t.Fatalf("group IDs = %#v, want %#v", groupIDs, want)
	}
	if want := []uint{1, 2}; !reflect.DeepEqual(userIDs, want) {
		t.Fatalf("user IDs = %#v, want %#v", userIDs, want)
	}
}

func TestDeleteAuthProviderGroupData(t *testing.T) {
	c := newTestClient(t)
	groups := []types.Group{
		{
			ID:                    "entra/engineering",
			AuthProviderName:      "entra-auth-provider",
			AuthProviderNamespace: "default",
			Name:                  "Engineering",
		},
		{
			ID:                    "okta/engineering",
			AuthProviderName:      "okta-auth-provider",
			AuthProviderNamespace: "default",
			Name:                  "Engineering",
		},
	}
	memberships := []types.GroupMemberships{
		{
			UserID:  1,
			GroupID: "entra/engineering",
		},
		{
			UserID:  2,
			GroupID: "okta/engineering",
		},
	}
	assignments := []types.GroupRoleAssignment{
		{
			GroupName: "entra/engineering",
			Role:      clienttypes.RoleAdmin,
		},
		{
			GroupName: "okta/engineering",
			Role:      clienttypes.RolePowerUser,
		},
	}
	identity := &types.Identity{
		AuthProviderName:              "entra-auth-provider",
		AuthProviderNamespace:         "default",
		HashedProviderUserID:          "user-1",
		AuthProviderGroupsLastChecked: time.Now(),
	}
	if err := c.db.WithContext(t.Context()).Create(&groups).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.WithContext(t.Context()).Create(&memberships).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.WithContext(t.Context()).Create(&assignments).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.WithContext(t.Context()).Create(identity).Error; err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := c.DeleteAuthProviderGroupData(t.Context(), "default", "entra-auth-provider", []string{"entra/engineering"}); err != nil {
			t.Fatal(err)
		}
	}

	var remainingGroups []types.Group
	if err := c.db.WithContext(t.Context()).Order("id").Find(&remainingGroups).Error; err != nil {
		t.Fatal(err)
	}
	if len(remainingGroups) != 1 || remainingGroups[0].ID != "okta/engineering" {
		t.Fatalf("remaining groups = %#v, want only okta/engineering", remainingGroups)
	}

	var remainingMemberships []types.GroupMemberships
	if err := c.db.WithContext(t.Context()).Find(&remainingMemberships).Error; err != nil {
		t.Fatal(err)
	}
	if len(remainingMemberships) != 1 || remainingMemberships[0].GroupID != "okta/engineering" {
		t.Fatalf("remaining memberships = %#v, want only okta/engineering", remainingMemberships)
	}

	remainingAssignments, err := c.ListGroupRoleAssignments(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(remainingAssignments) != 1 || remainingAssignments[0].GroupName != "okta/engineering" {
		t.Fatalf("remaining assignments = %#v, want only okta/engineering", remainingAssignments)
	}

	var gotIdentity types.Identity
	if err := c.db.WithContext(t.Context()).Where("hashed_provider_user_id = ?", identity.HashedProviderUserID).First(&gotIdentity).Error; err != nil {
		t.Fatal(err)
	}
	if !gotIdentity.AuthProviderGroupsLastChecked.IsZero() {
		t.Fatalf("identity group check timestamp = %v, want zero", gotIdentity.AuthProviderGroupsLastChecked)
	}
}
