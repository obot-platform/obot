package db

import (
	"errors"
	"fmt"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
)

func addAuthProviderNameAndNamespace(tx *gorm.DB) error {
	// Check if the identities table exists
	migrator := tx.Migrator()
	// If the identities table exists and the hashed_provider_user_id column doesn't, then we need
	// to migrate the old identities.
	if migrator.HasTable(&types.Identity{}) && !migrator.HasColumn(&types.Identity{}, "hashed_provider_user_id") {
		// The identities table needs to have auth_provider_namespace,auth_provider_name,provider_user_id as a primary key.
		// It used to have auth_provider_namespace,auth_provider_name,provider_username as a primary key.

		// Check if the migration is needed.
		var needsIdentityMigration bool
		if err := tx.Raw(`
				SELECT COUNT(*) = 0 as needs_migration
				FROM information_schema.key_column_usage
				WHERE table_name = 'identities'
				AND constraint_name = 'identities_pkey'
				AND column_name = 'provider_user_id'
			`).Scan(&needsIdentityMigration).Error; err != nil {
			return err
		}

		if needsIdentityMigration {
			// Add provider_user_id to identities table and update primary key.
			if err := tx.Exec(`
				-- Drop existing primary key
				ALTER TABLE identities DROP CONSTRAINT identities_pkey;

				-- Add provider_user_id column
				ALTER TABLE identities ADD COLUMN provider_user_id text NOT NULL DEFAULT '';

				-- Set placeholder values for existing records
				UPDATE identities SET provider_user_id = 'OBOT_PLACEHOLDER_' || provider_username WHERE provider_user_id = '';

				-- Add new primary key
					ALTER TABLE identities ADD PRIMARY KEY (auth_provider_name, auth_provider_namespace, provider_user_id);
				`).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func addIdentityAndUserHashedFields(tx *gorm.DB) error {
	migrator := tx.Migrator()
	u := new(types.User)
	if migrator.HasTable(u) {
		var usersNeedHashedFields bool
		if !migrator.HasColumn(u, "hashed_username") {
			if err := migrator.AddColumn(u, "hashed_username"); err != nil {
				return err
			}

			usersNeedHashedFields = true
		}

		if !migrator.HasColumn(u, "hashed_email") {
			if err := migrator.AddColumn(u, "hashed_email"); err != nil {
				return err
			}

			usersNeedHashedFields = true
		}

		if usersNeedHashedFields {
			var users []types.User
			if err := tx.Find(&users).Error; err != nil {
				return err
			}

			for _, user := range users {
				if user.Username != "" {
					user.HashedUsername = hash.String(user.Username)
				}
				if user.Email != "" {
					user.HashedEmail = hash.String(user.Email)
				}

				if err := tx.Model(&user).Updates(user).Error; err != nil {
					return fmt.Errorf("failed to migrate user ID %d: %w", user.ID, err)
				}
			}
		}
	}

	id := new(types.Identity)
	if migrator.HasTable(id) && !migrator.HasColumn(id, "hashed_provider_user_id") {
		if err := migrator.AddColumn(id, "hashed_provider_user_id"); err != nil {
			return err
		}

		if err := migrator.AddColumn(id, "hashed_email"); err != nil {
			return err
		}

		if migrator.HasConstraint(id, "identities_pkey") {
			if err := migrator.DropConstraint(id, "identities_pkey"); err != nil {
				return err
			}
		}

		var identities []types.Identity
		if err := tx.Find(&identities).Error; err != nil {
			return err
		}

		for _, i := range identities {
			if i.ProviderUserID != "" {
				if err := tx.Model(&i).Where("provider_user_id = ? AND auth_provider_name = ? AND auth_provider_namespace = ?", i.ProviderUserID, i.AuthProviderName, i.AuthProviderNamespace).Update("hashed_provider_user_id", hash.String(i.ProviderUserID)).Error; err != nil {
					return fmt.Errorf("failed to migrate identity for user ID %d: %w", i.UserID, err)
				}
			}
			if i.Email != "" {
				i.HashedEmail = hash.String(i.Email)
				if err := tx.Model(&i).Where("provider_user_id = ? AND auth_provider_name = ? AND auth_provider_namespace = ?", i.ProviderUserID, i.AuthProviderName, i.AuthProviderNamespace).Update("hashed_email", hash.String(i.Email)).Error; err != nil {
					return fmt.Errorf("failed to migrate identity for user ID %d: %w", i.UserID, err)
				}
			}
		}
	}

	return nil
}

func dropMCPOAuthTokensTableForUserIDPrimaryKey(tx *gorm.DB) error {
	migrator := tx.Migrator()
	if migrator.HasTable(&types.MCPOAuthToken{}) && !migrator.HasColumn(&types.MCPOAuthToken{}, "user_id") {
		if err := migrator.DropTable(&types.MCPOAuthToken{}); err != nil {
			return err
		}
	}

	return nil
}

func migrateMCPAuditLogClientInfo(tx *gorm.DB) error {
	migrator := tx.Migrator()
	if migrator.HasTable(&types.MCPAuditLog{}) && !migrator.HasColumn(&types.MCPAuditLog{}, "client_name") {
		if err := migrator.RenameColumn(&types.MCPAuditLog{}, "name", "client_name"); err != nil {
			return err
		}
		if err := migrator.RenameColumn(&types.MCPAuditLog{}, "version", "client_version"); err != nil {
			return err
		}
	}

	return nil
}

func migrateUserRoles(tx *gorm.DB) error {
	migrator := tx.Migrator()
	if migrator.HasTable(&types.User{}) && migrator.HasColumn(&types.User{}, "role") {
		var users []types.User
		if err := tx.Find(&users).Error; err != nil {
			return err
		}
		for _, user := range users {
			if user.HashedUsername == "333c04dd151a2a6831c039cb9a651df29198be8a04e16ce861d4b6a34a11c954" ||
				user.HashedUsername == "6382b3cc881412b77bfcaeed026001c00d9e3025e66c20f6e7e92f079851462a" {
				// These represent the bootstrap and nobody (authentication is turned off) users, respectively.
				// Their roles will be updated the next time they log in.
				continue
			}

			switch user.Role {
			case 1:
				user.Role = types2.RoleAdmin
			case 2:
				user.Role = types2.RolePowerUser
			case 3:
				user.Role = types2.RolePowerUserPlus
			case 10:
				user.Role = types2.RoleBasic
			default:
				// The role was already migrated, so know need to save it.
				continue
			}
			if err := tx.Save(&user).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func migrateIfEntryNotFoundInMigrationsTable(tx *gorm.DB, name string, f func(*gorm.DB) error) error {
	var migration types.Migration
	if err := tx.Where("name = ?", name).First(&migration).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// Entry not found, so migrate the data.

	if err := f(tx); err != nil {
		return err
	}

	// Update the migration table to mark the migration as complete.
	return tx.Model(&types.Migration{}).Create(&types.Migration{Name: name}).Error
}
