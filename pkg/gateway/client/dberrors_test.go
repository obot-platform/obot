package client

import (
	"errors"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func TestIsUniqueViolation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		client func(t *testing.T) *Client
	}{
		{
			name: "SQLite",
			client: func(t *testing.T) *Client {
				t.Helper()
				return newLifecycleTestClient(t)
			},
		},
		{
			name:   "PostgreSQL",
			client: newPostgresLifecycleTestClient,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.client(t)
			db := c.db.WithContext(t.Context())

			// A second row with the same primary key.
			group := types.Group{
				ID:                    "okta/00g-team",
				AuthProviderName:      lifecycleTestProvider.Name,
				AuthProviderNamespace: lifecycleTestProvider.Namespace,
				Name:                  "Team",
			}
			if err := db.Create(&group).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&group).Error; !IsUniqueViolation(err) {
				t.Errorf("a duplicate primary key gave %v, which is not a unique violation", err)
			}

			// A second row with the same values in a unique index: another binding of the same user.
			conn, _ := createTestSCIMConnection(t, c, false)
			user := provisionTestSCIMUser(t, c, conn, "00u-alice", "alice@example.com")
			var binding types.SCIMUserBinding
			if err := db.Where("id = ?", user.ID).Take(&binding).Error; err != nil {
				t.Fatal(err)
			}
			binding.ID = "another-binding"
			binding.HashedNativeUserID = "another-native-user"
			if err := db.Create(&binding).Error; !IsUniqueViolation(err) {
				t.Errorf("a duplicate unique index value gave %v, which is not a unique violation", err)
			}

			// Other failures are not.
			if err := db.Where("id = ?", "missing").Take(new(types.Group)).Error; !errors.Is(err, gorm.ErrRecordNotFound) || IsUniqueViolation(err) {
				t.Errorf("a missing row gave %v, want a not-found error that is not a unique violation", err)
			}
			if err := db.Exec("SELECT * FROM no_such_table").Error; err == nil || IsUniqueViolation(err) {
				t.Errorf("a query of a missing table gave %v, want an error that is not a unique violation", err)
			}
		})
	}
}
