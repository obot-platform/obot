package client

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestLimitAuditBody(t *testing.T) {
	body := json.RawMessage(`{"text":"é\"hello"}`)

	for _, tt := range []struct {
		name  string
		limit *int
	}{
		{
			name: "unlimited",
		},
		{
			name:  "omit",
			limit: new(0),
		},
		{
			name:  "exact",
			limit: new(len(body)),
		},
		{
			name:  "larger",
			limit: new(len(body) + 1),
		},
		{
			name:  "one byte",
			limit: new(1),
		},
		{
			name:  "inside unicode",
			limit: new(10),
		},
		{
			name:  "escaping",
			limit: new(14),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := limitAuditBody(body, tt.limit)
			require.Nil(t, limitAuditBody(nil, tt.limit))

			if tt.limit == nil || *tt.limit >= len(body) {
				require.Equal(t, body, got)
			} else if *tt.limit == 0 {
				require.Empty(t, got)
			} else {
				var preview struct {
					Truncated     bool   `json:"_obotAuditTruncated"`
					OriginalBytes int    `json:"originalBytes"`
					Preview       string `json:"preview"`
				}

				require.NoError(t, json.Unmarshal(got, &preview))
				require.True(t, preview.Truncated)
				require.Equal(t, len(body), preview.OriginalBytes)
				require.LessOrEqual(t, len(preview.Preview), *tt.limit)
				require.True(t, utf8.ValidString(preview.Preview))
				require.Equal(t, string(body[:len(preview.Preview)]), preview.Preview)
			}
		})
	}
}

func TestMCPAuditBodyPolicyPersistence(t *testing.T) {
	for _, limit := range []*int{nil, new(0), new(8)} {
		limitName := "unlimited"
		if limit != nil {
			limitName = fmt.Sprint(*limit)
		}

		for _, encrypted := range []bool{false, true} {
			t.Run(fmt.Sprintf("limit=%s/encrypted=%v", limitName, encrypted), func(t *testing.T) {
				c := newTestClient(t)
				c.mcpAuditMaxBodyBytes = limit
				if encrypted {
					c.encryptionConfig = testEncryptionConfig()
				}

				now := time.Now().UTC()
				body := json.RawMessage(`{"text":"original payload"}`)
				request := types.MCPAuditLog{
					SourceType: "mcp",
					CreatedAt:  now,
					UserID:     "user-1",
					MCPFields: &types.MCPAuditLogFields{
						MCPID:           "mcp-1",
						RequestID:       "request-1",
						ProxyExchangeID: "exchange-1",
						CallType:        "tools/call",
						CallIdentifier:  "tool-1",
						RequestBody:     body,
					},
				}

				c.LogMCPAuditEntry(request)
				require.Equal(t, body, request.MCP().RequestBody)
				require.NoError(t, c.persistMCPAuditLogs())

				response := types.MCPAuditLog{
					SourceType: "mcp",
					CreatedAt:  now.Add(time.Second),
					UserID:     "user-1",
					MCPFields: &types.MCPAuditLogFields{
						MCPID:                "mcp-1",
						RequestID:            "request-1",
						ProxyExchangeID:      "exchange-1",
						ResponseReceived:     true,
						ResponseStatus:       200,
						ResponseBody:         body,
						MutatedRequestBody:   body,
						OriginalResponseBody: body,
					},
				}

				c.LogMCPAuditEntry(response)
				require.Equal(t, body, response.MCP().ResponseBody)
				require.False(t, response.MCP().ResponseMutated)
				require.NoError(t, c.persistMCPAuditLogs())

				var rows []types.MCPAuditLog
				require.NoError(t, c.db.WithContext(t.Context()).Find(&rows).Error)
				require.Len(t, rows, 1)

				row, err := c.GetMCPAuditLog(t.Context(), rows[0].ID, true)
				require.NoError(t, err)

				mcp := row.MCP()
				require.True(t, mcp.RequestMutated)
				require.True(t, mcp.ResponseMutated)
				require.True(t, mcp.ResponseReceived)
				require.Equal(t, int64(1000), mcp.ProcessingTimeMs)
				require.Equal(t, "tool-1", mcp.CallIdentifier)

				for _, got := range []json.RawMessage{mcp.RequestBody, mcp.ResponseBody, mcp.MutatedRequestBody, mcp.OriginalResponseBody} {
					require.Equal(t, limitAuditBody(body, limit), got)
				}

				_, err = json.Marshal(row)
				require.NoError(t, err)

				// A complete entry sharing protocol identifiers must not merge with a
				// pending request merely because its request body was omitted.
				c.LogMCPAuditEntry(request)
				complete := request
				fields := *request.MCPFields
				complete.MCPFields = &fields
				complete.MCP().ResponseReceived = true
				complete.MCP().ResponseBody = body

				c.LogMCPAuditEntry(complete)
				require.NoError(t, c.persistMCPAuditLogs())
				require.Equal(t, int64(3), countAuditLogs(t, c))

				response.MCP().ProxyExchangeID = "orphan"
				c.LogMCPAuditEntry(response)
				require.NoError(t, c.persistMCPAuditLogs())
				require.Equal(t, int64(4), countAuditLogs(t, c))
			})
		}
	}
}

func TestDisabledMCPAuditPolicy(t *testing.T) {
	c := newTestClient(t)
	insertAuditLog(t, c, time.Now())
	c.mcpAuditDisabled = true
	c.mcpAuditMaxBodyBytes = new(0)
	require.False(t, c.MCPAuditLogEnabled())

	c.LogMCPAuditEntry(types.MCPAuditLog{})
	require.Empty(t, c.auditBuffer)
	require.NoError(t, c.persistMCPAuditLogs())
	require.Equal(t, int64(1), countAuditLogs(t, c))

	local := validLocalAgentAuditLog(time.Now(), "local-policy-test", "success")
	require.NoError(t, c.InsertLocalAgentAuditLogs(t.Context(), []types.MCPAuditLog{local}))
	require.Equal(t, int64(2), countAuditLogs(t, c))
	require.True(t, c.llmAuditEnabled)
}
