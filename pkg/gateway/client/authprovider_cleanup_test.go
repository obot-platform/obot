package client

import (
	"reflect"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestGetAuthProviderGroupCleanupUserIDs(t *testing.T) {
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
		{
			UserID:  4,
			GroupID: "entra/uncached",
		},
		{
			UserID:  5,
			GroupID: "entra-other/engineering",
		},
	}
	if err := c.db.WithContext(t.Context()).Create(&groups).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.WithContext(t.Context()).Create(&memberships).Error; err != nil {
		t.Fatal(err)
	}

	userIDs, err := c.GetAuthProviderGroupCleanupUserIDs(t.Context(), "entra/")
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint{1, 2, 4}; !reflect.DeepEqual(userIDs, want) {
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
		{
			ID:                    "entra-other/engineering",
			AuthProviderName:      "entra-other-auth-provider",
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
		{
			UserID:  3,
			GroupID: "entra/uncached",
		},
		{
			UserID:  4,
			GroupID: "entra-other/engineering",
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
		{
			GroupName: "entra/uncached",
			Role:      clienttypes.RolePowerUser,
		},
		{
			GroupName: "entra-other/engineering",
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
		if err := c.DeleteAuthProviderGroupData(t.Context(), "default", "entra-auth-provider", "entra/"); err != nil {
			t.Fatal(err)
		}
	}

	var remainingGroups []types.Group
	if err := c.db.WithContext(t.Context()).Order("id").Find(&remainingGroups).Error; err != nil {
		t.Fatal(err)
	}
	if got, want := groupIDs(remainingGroups), []string{"entra-other/engineering", "okta/engineering"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining group IDs = %#v, want %#v", got, want)
	}

	var remainingMemberships []types.GroupMemberships
	if err := c.db.WithContext(t.Context()).Order("group_id").Find(&remainingMemberships).Error; err != nil {
		t.Fatal(err)
	}
	if got, want := membershipGroupIDs(remainingMemberships), []string{"entra-other/engineering", "okta/engineering"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining membership group IDs = %#v, want %#v", got, want)
	}

	remainingAssignments, err := c.ListGroupRoleAssignments(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := assignmentGroupIDs(remainingAssignments), []string{"entra-other/engineering", "okta/engineering"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining assignment group IDs = %#v, want %#v", got, want)
	}

	var gotIdentity types.Identity
	if err := c.db.WithContext(t.Context()).Where("hashed_provider_user_id = ?", identity.HashedProviderUserID).First(&gotIdentity).Error; err != nil {
		t.Fatal(err)
	}
	if !gotIdentity.AuthProviderGroupsLastChecked.IsZero() {
		t.Fatalf("identity group check timestamp = %v, want zero", gotIdentity.AuthProviderGroupsLastChecked)
	}
}

func groupIDs(groups []types.Group) []string {
	result := make([]string, len(groups))
	for i := range groups {
		result[i] = groups[i].ID
	}
	return result
}

func membershipGroupIDs(memberships []types.GroupMemberships) []string {
	result := make([]string, len(memberships))
	for i := range memberships {
		result[i] = memberships[i].GroupID
	}
	return result
}

func assignmentGroupIDs(assignments []types.GroupRoleAssignment) []string {
	result := make([]string, len(assignments))
	for i := range assignments {
		result[i] = assignments[i].GroupName
	}
	return result
}
