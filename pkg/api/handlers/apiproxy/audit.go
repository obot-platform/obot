package apiproxy

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp/auditlogs"
)

// AuditCollector mirrors auditlogs.Collector without importing the mcpgateway
// package. It is satisfied by mcpgateway.AuditLogHandler.
type AuditCollector = auditlogs.Collector

// auditBodyLimit bounds audit body excerpts for HTTP proxy calls, matching the
// shared contract cap (32 KiB) used by the MCP bridge.
const auditBodyLimit = 32 * 1024

// AuditWriter emits one MCP audit entry per proxied HTTP call. The proxy path
// previously wrote no audit records at all; this restores parity with the MCP
// tools/call path while keeping bodies bounded and identities platform-derived.
type AuditWriter struct {
	Collector AuditCollector
}

// Record emits one audit entry for one proxied request. result fields are only
// read when err is nil. The entry never includes credentials: input headers
// were sanitized before execution and only their names/sizes are recorded.
func (a *AuditWriter) Record(req api.Context, route Route, operation Operation, result *ExecutionResult, err error, started time.Time) {
	if a == nil || a.Collector == nil {
		return
	}
	entry := auditlogs.MCPAuditLog{
		CreatedAt: started,
		Subject:   req.User.GetUID(),
		ClientIP:  proxyClientIP(req),
		CallType:  "http/proxy",
		UserAgent: req.Request.UserAgent(),
		Metadata: map[string]string{
			"vmcpID":      route.VMCPID,
			"componentID": route.ComponentID,
			"method":      req.Method,
		},
		RequestID: req.Request.Header.Get("X-Obot-Request-ID"),
	}
	if operation.OperationKey != "" {
		entry.CallIdentifier = operation.OperationKey
	}
	if auth := strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Bearer "); auth != "" && strings.Count(auth, ".") != 2 {
		entry.APIKey = auditlogs.RedactAPIKey(auth)
	}
	if req.Request.ContentLength > 0 {
		entry.RequestBody = json.RawMessage(`{"originalBytes":` + jsonInt(req.Request.ContentLength) + `}`)
	}
	if err != nil {
		entry.Error = boundedString(err.Error(), 256)
	} else if result != nil {
		entry.ResponseStatus = result.Status
		if body := result.Body; len(body) > 0 {
			excerpt := body
			if len(excerpt) > auditBodyLimit {
				marker, _ := json.Marshal(map[string]any{"truncated": true, "originalBytes": len(body)})
				entry.ResponseBody = marker
			} else if json.Valid(excerpt) {
				entry.ResponseBody = excerpt
			}
		}
	}
	entry.SetProcessingTime()
	a.Collector.CollectMCPAuditEntry(entry)
}

func proxyClientIP(req api.Context) string {
	if forwarded := req.Request.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host := req.Request.RemoteAddr
	if index := strings.LastIndex(host, ":"); index > 0 {
		host = host[:index]
	}
	return strings.TrimSpace(host)
}

func boundedString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…(truncated)"
}

func jsonInt(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
