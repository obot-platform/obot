package client

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestDeleteHostedAgentRecords(t *testing.T) {
	c := newTestClient(t)
	db := c.db.WithContext(t.Context())
	require.NoError(t, db.Exec("CREATE TABLE hostedagentinstance (value TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO hostedagentinstance (value) VALUES ('legacy finalizer')").Error)
	require.NoError(t, db.Exec("CREATE TABLE IF NOT EXISTS compaction (name TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO compaction (name) VALUES ('hostedagentinstance'), ('mcpserver')").Error)
	for _, scope := range []string{"hosted-agent-credential", "claude-runner", "mcp"} {
		require.NoError(t, db.Create(&types.Credential{Context: scope, Name: "test", Secrets: map[string]string{"key": "secret"}}).Error)
	}

	require.NoError(t, c.DeleteHostedAgentRecords(t.Context()))
	require.False(t, db.Migrator().HasTable("hostedagentinstance"))
	var credentials []types.Credential
	require.NoError(t, db.Find(&credentials).Error)
	require.Len(t, credentials, 2)
	for _, credential := range credentials {
		require.NotEqual(t, "hosted-agent-credential", credential.Context)
	}
	var names []string
	require.NoError(t, db.Table("compaction").Pluck("name", &names).Error)
	require.Equal(t, []string{"mcpserver"}, names)
	require.NoError(t, c.DeleteHostedAgentRecords(t.Context()))
}
