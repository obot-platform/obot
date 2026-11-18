package client

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

// auditBodyColumns are the audit row columns that hold bodies.
var (
	auditBodyColumns = []string{
		"request_body", "mutated_request_body", "response_body", "original_response_body",
		"local_agent_request_body", "local_agent_response_body", "local_agent_raw_event",
	}
)

// legacyStoredBytes is the size body took in a body column before the field format: encrypted,
// then base64-encoded.
func legacyStoredBytes(b *testing.B, c *Client, log *types.MCPAuditLog, body json.RawMessage) int {
	b.Helper()
	if len(body) == 0 {
		return 0
	}
	field := body
	if err := encryptRawMessageField(b.Context(), c.mcpAuditLogTransformer(), mcpAuditLogDataCtx(log), &field); err != nil {
		b.Fatal(err)
	}
	return len(field)
}

// storedBodyBytes is the total size of every body column plus the shared bodies table.
func storedBodyBytes(b *testing.B, c *Client) int64 {
	b.Helper()
	var total int64
	for _, column := range auditBodyColumns {
		var size int64
		if err := c.db.WithContext(b.Context()).Raw("SELECT COALESCE(SUM(LENGTH(" + column + ")), 0) FROM mcp_audit_logs").Scan(&size).Error; err != nil {
			b.Fatal(err)
		}
		total += size
	}
	var shared int64
	if err := c.db.WithContext(b.Context()).Raw("SELECT COALESCE(SUM(LENGTH(body)), 0) FROM mcp_audit_log_bodies").Scan(&shared).Error; err != nil {
		b.Fatal(err)
	}
	return total + shared
}

// BenchmarkMCPAuditLogBodyStorage reports, per audit entry, the body bytes stored before the field
// format (legacy_bytes/op) and with it (stored_bytes/op), alongside the time to encode and persist
// the entry. Run it with a fixed count to model N calls, for example -benchtime=200x.
//
// The bodies are generated, so the compression ratios here are not those of production traffic;
// the deduplication saving does not depend on content.
func BenchmarkMCPAuditLogBodyStorage(b *testing.B) {
	b.Run("repeated 500KB tools list", func(b *testing.B) {
		c := newTestClient(b)
		c.encryptionConfig = contextBoundEncryptionConfig()
		result := testListResult("bench", 500<<10)

		var legacy, calls int
		for b.Loop() {
			calls++
			entries := listCallEntries(calls, "tools/list", testListResponse(calls, result))
			for i := range entries {
				entries[i].NormalizeMCPFields()
				b.StopTimer()
				legacy += legacyStoredBytes(b, c, &entries[i], entries[i].MCPFields.RequestBody)
				legacy += legacyStoredBytes(b, c, &entries[i], entries[i].MCPFields.ResponseBody)
				b.StartTimer()
				if err := c.encryptMCPAuditLog(b.Context(), &entries[i]); err != nil {
					b.Fatal(err)
				}
			}
			if err := c.insertMCPAuditLogs(b.Context(), entries); err != nil {
				b.Fatal(err)
			}
		}

		b.ReportMetric(float64(legacy)/float64(calls), "legacy_bytes/op")
		b.ReportMetric(float64(storedBodyBytes(b, c))/float64(calls), "stored_bytes/op")
	})

	b.Run("unique 800KB local agent event", func(b *testing.B) {
		c := newTestClient(b)
		c.encryptionConfig = contextBoundEncryptionConfig()

		var legacy, events int
		for b.Loop() {
			events++
			b.StopTimer()
			output := testListResult(fmt.Sprintf("event%d", events), 400<<10)
			log := validLocalAgentAuditLog(time.Now().UTC(), fmt.Sprintf("bench-%d", events), types2.AuditLogOutcomeStatusSuccess)
			local := log.LocalAgentToolCallFields
			local.RequestBody = json.RawMessage(`{"command":"list tools"}`)
			local.ResponseBody = json.RawMessage(output)
			local.RawEvent = json.RawMessage(`{"tool_response":` + output + `}`)
			for _, body := range []json.RawMessage{local.RequestBody, local.ResponseBody, local.RawEvent} {
				legacy += legacyStoredBytes(b, c, &log, body)
			}
			b.StartTimer()
			if err := c.InsertLocalAgentAuditLogs(b.Context(), []types.MCPAuditLog{log}); err != nil {
				b.Fatal(err)
			}
		}

		b.ReportMetric(float64(legacy)/float64(events), "legacy_bytes/op")
		b.ReportMetric(float64(storedBodyBytes(b, c))/float64(events), "stored_bytes/op")
	})
}
