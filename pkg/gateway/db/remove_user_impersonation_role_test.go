package db

import (
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
)

func TestRemoveUserImpersonationRole(t *testing.T) {
	services, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { _ = services.DB.SQLDB.Close() })

	database, err := New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway database: %v", err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate gateway database: %v", err)
	}

	tx := services.DB.DB
	impersonatingAdmin := apitypes.RoleAdmin | apitypes.RoleAuditor | userImpersonationRole
	for _, record := range []any{
		&types.User{HashedUsername: "impersonator", Role: impersonatingAdmin},
		&types.User{HashedUsername: "owner", Role: apitypes.RoleOwner},
		&types.GroupRoleAssignment{GroupName: "impersonators", Role: apitypes.RoleOwner | userImpersonationRole},
		&types.GroupRoleAssignment{GroupName: "admins", Role: apitypes.RoleAdmin},
	} {
		if err := tx.Create(record).Error; err != nil {
			t.Fatalf("failed to create %T: %v", record, err)
		}
	}

	if err := removeUserImpersonationRole(tx); err != nil {
		t.Fatalf("removeUserImpersonationRole() error = %v", err)
	}

	for username, want := range map[string]apitypes.Role{
		"impersonator": apitypes.RoleAdmin | apitypes.RoleAuditor,
		"owner":        apitypes.RoleOwner,
	} {
		var user types.User
		if err := tx.Where("hashed_username = ?", username).First(&user).Error; err != nil {
			t.Fatalf("failed to get user %s: %v", username, err)
		}
		if user.Role != want {
			t.Errorf("user %s role = %d, want %d", username, user.Role, want)
		}
	}
	for group, want := range map[string]apitypes.Role{
		"impersonators": apitypes.RoleOwner,
		"admins":        apitypes.RoleAdmin,
	} {
		var assignment types.GroupRoleAssignment
		if err := tx.Where("group_name = ?", group).First(&assignment).Error; err != nil {
			t.Fatalf("failed to get group role assignment %s: %v", group, err)
		}
		if assignment.Role != want {
			t.Errorf("group %s role = %d, want %d", group, assignment.Role, want)
		}
	}
}
