package db

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// legacyVerifiedEmailUser is the users column of a previous release that the migration drops.
type legacyVerifiedEmailUser struct {
	VerifiedEmail *bool
}

func (legacyVerifiedEmailUser) TableName() string {
	return "users"
}

func TestUserVerifiedEmailMigrationOfAnExistingSQLiteDatabase(t *testing.T) {
	database, gormDB := newSQLiteTestDatabase(t)
	testUserVerifiedEmailMigration(t, database, gormDB)
}

func TestUserVerifiedEmailMigrationOfAnExistingPostgresDatabase(t *testing.T) {
	database, gormDB := newPostgresTestDatabase(t, "obot_verified_email_")
	testUserVerifiedEmailMigration(t, database, gormDB)
}

// testUserVerifiedEmailMigration migrates a database whose users still have the verified_email column, and checks
// that the column is dropped and that the users are kept.
func testUserVerifiedEmailMigration(t *testing.T, database *DB, gormDB *gorm.DB) {
	t.Helper()

	// Build the current schema, then add the column back, so that the database looks like one created by a previous
	// release.
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to create the schema: %v", err)
	}
	if err := gormDB.Migrator().AddColumn(&legacyVerifiedEmailUser{}, "VerifiedEmail"); err != nil {
		t.Fatalf("failed to add verified_email column: %v", err)
	}
	if err := gormDB.Create(&types.User{
		Username:       "user",
		HashedUsername: "hashed-user",
	}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}
	if gormDB.Migrator().HasColumn(&types.User{}, "verified_email") {
		t.Fatal("users.verified_email still exists after migration")
	}
	var users int64
	if err := gormDB.Model(new(types.User)).Count(&users).Error; err != nil {
		t.Fatalf("failed to count users: %v", err)
	}
	if users != 1 {
		t.Fatalf("users after migration = %d, want 1", users)
	}
}
