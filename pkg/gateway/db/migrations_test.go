package db

import (
	"errors"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	return db
}

func TestMigrateIfEntryNotFoundInMigrationsTable(t *testing.T) {
	tests := []struct {
		name           string
		migrationName  string
		existingEntry  bool
		funcReturnsErr bool
		wantErr        bool
		wantCallCount  int
	}{
		{
			name:          "migration not found - runs function successfully",
			migrationName: "test_migration",
			existingEntry: false,
			wantErr:       false,
			wantCallCount: 1,
		},
		{
			name:          "migration already exists - skips function",
			migrationName: "existing_migration",
			existingEntry: true,
			wantErr:       false,
			wantCallCount: 0,
		},
		{
			name:           "migration not found - function returns error",
			migrationName:  "failing_migration",
			existingEntry:  false,
			funcReturnsErr: true,
			wantErr:        true,
			wantCallCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)

			// Create migrations table
			if err := db.AutoMigrate(&types.Migration{}); err != nil {
				t.Fatalf("failed to create migrations table: %v", err)
			}

			// Insert existing migration if needed
			if tt.existingEntry {
				if err := db.Create(&types.Migration{Name: tt.migrationName}).Error; err != nil {
					t.Fatalf("failed to create existing migration entry: %v", err)
				}
			}

			// Track function calls
			callCount := 0
			testFunc := func(tx *gorm.DB) error {
				callCount++
				if tt.funcReturnsErr {
					return errors.New("migration function error")
				}
				return nil
			}

			// Run the migration
			err := migrateIfEntryNotFoundInMigrationsTable(db, tt.migrationName, testFunc)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("migrateIfEntryNotFoundInMigrationsTable() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check function was called expected number of times
			if callCount != tt.wantCallCount {
				t.Errorf("migration function called %d times, want %d", callCount, tt.wantCallCount)
			}

			// Verify migration entry was created if successful
			if !tt.wantErr && !tt.existingEntry {
				var migration types.Migration
				if err := db.Where("name = ?", tt.migrationName).First(&migration).Error; err != nil {
					t.Errorf("expected migration entry to be created, but got error: %v", err)
				}
			}
		})
	}
}

func TestMigrateUserRoles(t *testing.T) {
	tests := []struct {
		name           string
		users          []types.User
		expectedRoles  map[uint]types2.Role
		expectModified map[uint]bool
	}{
		{
			name: "bootstrap user gets owner role",
			users: []types.User{
				{
					ID:             1,
					HashedUsername: "333c04dd151a2a6831c039cb9a651df29198be8a04e16ce861d4b6a34a11c954", // bootstrap user
					Role:           1, // old admin role
				},
			},
			expectedRoles: map[uint]types2.Role{
				1: types2.RoleOwner,
			},
			expectModified: map[uint]bool{1: true},
		},
		{
			name: "nobody user gets owner and auditor roles",
			users: []types.User{
				{
					ID:             2,
					HashedUsername: "6382b3cc881412b77bfcaeed026001c00d9e3025e66c20f6e7e92f079851462a", // nobody user
					Role:           1,
				},
			},
			expectedRoles: map[uint]types2.Role{
				2: types2.RoleOwner | types2.RoleAuditor,
			},
			expectModified: map[uint]bool{2: true},
		},
		{
			name: "old role 1 becomes admin",
			users: []types.User{
				{ID: 3, HashedUsername: "regularuser1", Role: 1},
			},
			expectedRoles: map[uint]types2.Role{
				3: types2.RoleAdmin,
			},
			expectModified: map[uint]bool{3: true},
		},
		{
			name: "old role 2 becomes power user plus",
			users: []types.User{
				{ID: 4, HashedUsername: "regularuser2", Role: 2},
			},
			expectedRoles: map[uint]types2.Role{
				4: types2.RolePowerUserPlus,
			},
			expectModified: map[uint]bool{4: true},
		},
		{
			name: "old role 3 becomes power user",
			users: []types.User{
				{ID: 5, HashedUsername: "regularuser3", Role: 3},
			},
			expectedRoles: map[uint]types2.Role{
				5: types2.RolePowerUser,
			},
			expectModified: map[uint]bool{5: true},
		},
		{
			name: "old role 10 becomes basic",
			users: []types.User{
				{ID: 6, HashedUsername: "regularuser4", Role: 10},
			},
			expectedRoles: map[uint]types2.Role{
				6: types2.RoleBasic,
			},
			expectModified: map[uint]bool{6: true},
		},
		{
			name: "already migrated roles are not changed",
			users: []types.User{
				{ID: 7, HashedUsername: "alreadymigrated", Role: types2.RoleAdmin},
			},
			expectedRoles: map[uint]types2.Role{
				7: types2.RoleAdmin,
			},
			expectModified: map[uint]bool{7: false},
		},
		{
			name: "mixed users with different migrations",
			users: []types.User{
				{ID: 8, HashedUsername: "333c04dd151a2a6831c039cb9a651df29198be8a04e16ce861d4b6a34a11c954", Role: 1},
				{ID: 9, HashedUsername: "6382b3cc881412b77bfcaeed026001c00d9e3025e66c20f6e7e92f079851462a", Role: 1},
				{ID: 10, HashedUsername: "regular1", Role: 1},
				{ID: 11, HashedUsername: "regular2", Role: 2},
				{ID: 12, HashedUsername: "migrated", Role: types2.RolePowerUser},
			},
			expectedRoles: map[uint]types2.Role{
				8:  types2.RoleOwner,
				9:  types2.RoleOwner | types2.RoleAuditor,
				10: types2.RoleAdmin,
				11: types2.RolePowerUserPlus,
				12: types2.RolePowerUser,
			},
			expectModified: map[uint]bool{8: true, 9: true, 10: true, 11: true, 12: false},
		},
		{
			name:           "empty user table",
			users:          []types.User{},
			expectedRoles:  map[uint]types2.Role{},
			expectModified: map[uint]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)

			// Create users table
			if err := db.AutoMigrate(&types.User{}); err != nil {
				t.Fatalf("failed to create users table: %v", err)
			}

			// Insert test users
			for _, user := range tt.users {
				if err := db.Create(&user).Error; err != nil {
					t.Fatalf("failed to create test user: %v", err)
				}
			}

			// Run the migration
			if err := migrateUserRoles(db); err != nil {
				t.Fatalf("migrateUserRoles() error = %v", err)
			}

			// Verify each user has the expected role
			for userID, expectedRole := range tt.expectedRoles {
				var user types.User
				if err := db.First(&user, userID).Error; err != nil {
					t.Fatalf("failed to fetch user %d: %v", userID, err)
				}

				if user.Role != expectedRole {
					t.Errorf("user %d: got role %v, want %v", userID, user.Role, expectedRole)
				}
			}
		})
	}
}

func TestRemoveGitHubGroups(t *testing.T) {
	tests := []struct {
		name              string
		groups            []types.Group
		memberships       []types.GroupMemberships
		expectedGroups    []string
		expectedMembersip []string
	}{
		{
			name: "removes github groups and memberships",
			groups: []types.Group{
				{ID: "github/org1", Name: "org1"},
				{ID: "github/org2", Name: "org2"},
				{ID: "local/group1", Name: "group1"},
			},
			memberships: []types.GroupMemberships{
				{GroupID: "github/org1", UserID: 1},
				{GroupID: "github/org2", UserID: 2},
				{GroupID: "local/group1", UserID: 3},
			},
			expectedGroups:    []string{"local/group1"},
			expectedMembersip: []string{"local/group1"},
		},
		{
			name: "handles only github groups",
			groups: []types.Group{
				{ID: "github/org1", Name: "org1"},
				{ID: "github/org2", Name: "org2"},
			},
			memberships: []types.GroupMemberships{
				{GroupID: "github/org1", UserID: 1},
			},
			expectedGroups:    []string{},
			expectedMembersip: []string{},
		},
		{
			name: "handles no github groups",
			groups: []types.Group{
				{ID: "local/group1", Name: "group1"},
				{ID: "azure/group2", Name: "group2"},
			},
			memberships: []types.GroupMemberships{
				{GroupID: "local/group1", UserID: 1},
				{GroupID: "azure/group2", UserID: 2},
			},
			expectedGroups:    []string{"local/group1", "azure/group2"},
			expectedMembersip: []string{"local/group1", "azure/group2"},
		},
		{
			name:              "handles empty tables",
			groups:            []types.Group{},
			memberships:       []types.GroupMemberships{},
			expectedGroups:    []string{},
			expectedMembersip: []string{},
		},
		{
			name: "removes only github/ prefix groups",
			groups: []types.Group{
				{ID: "github/org1", Name: "org1"},
				{ID: "githublike/org2", Name: "org2"}, // not matching "github/" prefix
				{ID: "gitlab/org3", Name: "org3"},
			},
			memberships: []types.GroupMemberships{
				{GroupID: "github/org1", UserID: 1},
				{GroupID: "githublike/org2", UserID: 2},
				{GroupID: "gitlab/org3", UserID: 3},
			},
			expectedGroups:    []string{"githublike/org2", "gitlab/org3"},
			expectedMembersip: []string{"githublike/org2", "gitlab/org3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)

			// Create tables
			if err := db.AutoMigrate(&types.Group{}, &types.GroupMemberships{}); err != nil {
				t.Fatalf("failed to create tables: %v", err)
			}

			// Insert test data
			for _, group := range tt.groups {
				if err := db.Create(&group).Error; err != nil {
					t.Fatalf("failed to create test group: %v", err)
				}
			}
			for _, membership := range tt.memberships {
				if err := db.Create(&membership).Error; err != nil {
					t.Fatalf("failed to create test membership: %v", err)
				}
			}

			// Run the migration
			if err := removeGitHubGroups(db); err != nil {
				t.Fatalf("removeGitHubGroups() error = %v", err)
			}

			// Verify remaining groups
			var remainingGroups []types.Group
			if err := db.Find(&remainingGroups).Error; err != nil {
				t.Fatalf("failed to fetch remaining groups: %v", err)
			}

			remainingGroupIDs := make([]string, len(remainingGroups))
			for i, g := range remainingGroups {
				remainingGroupIDs[i] = g.ID
			}

			if len(remainingGroupIDs) != len(tt.expectedGroups) {
				t.Errorf("got %d groups, want %d: %v", len(remainingGroupIDs), len(tt.expectedGroups), remainingGroupIDs)
			} else {
				for _, expectedID := range tt.expectedGroups {
					found := false
					for _, actualID := range remainingGroupIDs {
						if actualID == expectedID {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected group %s not found in remaining groups: %v", expectedID, remainingGroupIDs)
					}
				}
			}

			// Verify remaining memberships
			var remainingMemberships []types.GroupMemberships
			if err := db.Find(&remainingMemberships).Error; err != nil {
				t.Fatalf("failed to fetch remaining memberships: %v", err)
			}

			remainingMembershipGroupIDs := make([]string, len(remainingMemberships))
			for i, m := range remainingMemberships {
				remainingMembershipGroupIDs[i] = m.GroupID
			}

			if len(remainingMembershipGroupIDs) != len(tt.expectedMembersip) {
				t.Errorf("got %d memberships, want %d: %v", len(remainingMembershipGroupIDs), len(tt.expectedMembersip), remainingMembershipGroupIDs)
			}
		})
	}
}

func TestDropSessionCookiesTable(t *testing.T) {
	tests := []struct {
		name        string
		tableExists bool
		wantErr     bool
	}{
		{
			name:        "drops existing session_cookies table",
			tableExists: true,
			wantErr:     false,
		},
		{
			name:        "handles non-existent session_cookies table",
			tableExists: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)

			// Create table if needed
			if tt.tableExists {
				// Create a simple table with the name "session_cookies"
				type SessionCookie struct {
					ID    uint   `gorm:"primaryKey"`
					Token string `gorm:"unique"`
				}
				if err := db.Table("session_cookies").AutoMigrate(&SessionCookie{}); err != nil {
					t.Fatalf("failed to create session_cookies table: %v", err)
				}

				// Verify it exists
				if !db.Migrator().HasTable("session_cookies") {
					t.Fatal("session_cookies table should exist but doesn't")
				}
			}

			// Run the migration
			err := dropSessionCookiesTable(db)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("dropSessionCookiesTable() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify table doesn't exist after migration
			if !tt.wantErr && db.Migrator().HasTable("session_cookies") {
				t.Error("session_cookies table still exists after migration")
			}
		})
	}
}

func TestMigrateUserRolesWithHashedFields(t *testing.T) {
	// Test that the migration correctly handles users with hashed fields
	// This tests the integration with the hash package
	db := setupTestDB(t)

	// Create users table
	if err := db.AutoMigrate(&types.User{}); err != nil {
		t.Fatalf("failed to create users table: %v", err)
	}

	// Create a user with a specific username that we can hash
	testUser := types.User{
		ID:             100,
		Username:       "testuser",
		HashedUsername: hash.String("testuser"),
		Role:           1, // old admin role
	}

	if err := db.Create(&testUser).Error; err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Run the migration
	if err := migrateUserRoles(db); err != nil {
		t.Fatalf("migrateUserRoles() error = %v", err)
	}

	// Verify the user role was migrated
	var user types.User
	if err := db.First(&user, 100).Error; err != nil {
		t.Fatalf("failed to fetch user: %v", err)
	}

	if user.Role != types2.RoleAdmin {
		t.Errorf("expected role %v, got %v", types2.RoleAdmin, user.Role)
	}

	// Verify hashed username is still correct
	if user.HashedUsername != hash.String("testuser") {
		t.Errorf("hashed username was modified unexpectedly")
	}
}
