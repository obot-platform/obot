package client

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/auditlog"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestLocalAgentAuditBodyLimitPersistence(t *testing.T) {
	tests := []struct {
		name      string
		limit     *int
		encrypted bool
	}{
		{
			name: "unlimited",
		},
		{
			name:      "unlimited encrypted",
			encrypted: true,
		},
		{
			name:  "omitted",
			limit: new(0),
		},
		{
			name:      "omitted encrypted",
			limit:     new(0),
			encrypted: true,
		},
		{
			name:  "truncated",
			limit: new(5),
		},
		{
			name:      "truncated encrypted",
			limit:     new(5),
			encrypted: true,
		},
		{
			name:  "within limit",
			limit: new(100),
		},
		{
			name:      "within limit encrypted",
			limit:     new(100),
			encrypted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			c.localAgentAuditMaxBodyBytes = tt.limit

			// Local-agent submissions are independent of both gateway audit settings.
			c.mcpAuditDisabled = true
			c.mcpAuditMaxBodyBytes = new(0)
			c.llmAuditEnabled = false

			if tt.encrypted {
				c.encryptionConfig = testEncryptionConfig()
			}

			entry := validLocalAgentAuditLog(time.Now().UTC(), "local-limit", "success")
			original, err := json.Marshal(entry)
			require.NoError(t, err)

			require.NoError(t, c.InsertLocalAgentAuditLogs(t.Context(), []types.MCPAuditLog{entry}))

			after, err := json.Marshal(entry)
			require.NoError(t, err)
			require.JSONEq(t, string(original), string(after), "caller data must remain unchanged")

			var row types.MCPAuditLog
			require.NoError(t, c.db.WithContext(t.Context()).First(&row).Error)
			require.Equal(t, tt.encrypted, row.Encrypted)

			stored, err := c.GetMCPAuditLog(t.Context(), row.ID, true)
			require.NoError(t, err)

			local := stored.LocalAgentToolCallFields
			for _, pair := range [][2]json.RawMessage{
				{entry.LocalAgentToolCallFields.RequestBody, local.RequestBody},
				{entry.LocalAgentToolCallFields.ResponseBody, local.ResponseBody},
				{entry.LocalAgentToolCallFields.RawEvent, local.RawEvent},
			} {
				if tt.limit != nil && *tt.limit == 0 {
					require.Empty(t, pair[1])

					continue
				}

				if tt.limit == nil || len(pair[0]) <= *tt.limit {
					require.JSONEq(t, string(pair[0]), string(pair[1]))

					continue
				}

				var preview struct {
					Truncated     bool   `json:"_obotAuditTruncated"`
					OriginalBytes int    `json:"originalBytes"`
					Preview       string `json:"preview"`
				}
				require.NoError(t, json.Unmarshal(pair[1], &preview))
				require.True(t, preview.Truncated)
				require.Equal(t, len(pair[0]), preview.OriginalBytes)
				require.Equal(t, string(pair[0][:*tt.limit]), preview.Preview)
			}

			require.Equal(t, entry.LocalAgentToolCallFields.ActionName, local.ActionName)
			require.Equal(t, entry.LocalAgentToolCallFields.OutcomeStatus, local.OutcomeStatus)
			require.Equal(t, entry.LocalAgentToolCallFields.GitRemotes, local.GitRemotes)

			// Omitted payloads and previews must also survive the API/export conversion.
			_, err = json.Marshal(auditlog.Present(*stored, auditlog.PresentOptions{IncludeDetails: true}))
			require.NoError(t, err)
		})
	}
}

func TestDisabledLocalAgentAuditPreservesHistory(t *testing.T) {
	c := newTestClient(t)
	entry := validLocalAgentAuditLog(time.Now().UTC(), "historical", "success")
	require.NoError(t, c.InsertLocalAgentAuditLogs(t.Context(), []types.MCPAuditLog{entry}))

	c.localAgentAuditDisabled = true
	require.False(t, c.LocalAgentAuditLogEnabled())
	require.True(t, c.MCPAuditLogEnabled())
	require.True(t, c.LLMAuditLogEnabled())

	require.NoError(t, c.InsertLocalAgentAuditLogs(t.Context(), []types.MCPAuditLog{{}}))
	require.Equal(t, int64(1), countAuditLogs(t, c))

	var row types.MCPAuditLog
	require.NoError(t, c.db.WithContext(t.Context()).First(&row).Error)

	_, err := c.GetMCPAuditLog(t.Context(), row.ID, true)
	require.NoError(t, err)
}
