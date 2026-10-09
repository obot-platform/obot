package db

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestAutoMigrateRetiresHostedAgentKeys(t *testing.T) {
	db, sql := newCatalogConfigMigrationDB(t)
	require.NoError(t, sql.AutoMigrate(&types.APIKey{}))
	require.NoError(t, sql.Exec(`ALTER TABLE api_keys ADD COLUMN "hosted_agent_instance_id" TEXT`).Error)
	require.NoError(t, sql.Exec("CREATE INDEX idx_api_keys_hosted_agent_instance_id ON api_keys(hosted_agent_instance_id)").Error)

	revokedAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	keys := []types.APIKey{
		{
			ID:     1,
			UserID: 7,
		},
		{
			ID:     2,
			UserID: 7,
		},
		{
			ID:        3,
			UserID:    7,
			RevokedAt: &revokedAt,
		},
		{
			ID:     4,
			UserID: 7,
		},
	}
	require.NoError(t, sql.Create(&keys).Error)
	require.NoError(t, sql.Exec("UPDATE api_keys SET hosted_agent_instance_id = 'instance' WHERE id IN (2, 3)").Error)
	require.NoError(t, sql.Exec("UPDATE api_keys SET hosted_agent_instance_id = '' WHERE id = 4").Error)

	require.NoError(t, db.AutoMigrate())
	require.False(t, sql.Migrator().HasColumn(&types.APIKey{}, "hosted_agent_instance_id"))
	require.NoError(t, sql.Order("id").Find(&keys).Error)
	require.Nil(t, keys[0].RevokedAt)
	require.NotNil(t, keys[1].RevokedAt)
	require.True(t, keys[2].RevokedAt.Equal(revokedAt))
	require.Nil(t, keys[3].RevokedAt)
	require.NoError(t, db.AutoMigrate())
}
