package apiproxy

import (
	"errors"
	"net/http"
	"testing"

	"github.com/obot-platform/obot/pkg/mcp/auditlogs"
	"github.com/stretchr/testify/require"
)

type recordingAuditCollector struct {
	entries []auditlogs.MCPAuditLog
}

func (c *recordingAuditCollector) CollectMCPAuditEntry(entry auditlogs.MCPAuditLog) {
	c.entries = append(c.entries, entry)
}

func (c *recordingAuditCollector) Close() {}

func TestProxyAuditWriterRecordsBusinessCall(t *testing.T) {
	executor := &recordingExecutor{
		description: Description{
			Operations: []OperationDescription{{
				Operation: Operation{OperationKey: "GET /items/{id}", LegacyToolName: "getItem", Method: "GET", PathTemplate: "/items/{id}"},
			}},
		},
		result: ExecutionResult{Kind: "business", Status: 200, Body: []byte(`{"item":"ok"}`)},
	}
	collector := &recordingAuditCollector{}
	handler := &Handler{Executor: executor, Audit: &AuditWriter{Collector: collector}}
	ctx := testAPIContext(t, http.MethodGet, "/api-proxy/vmcp1abc/component/items/42", nil)
	ctx.Request.Header.Set("Authorization", "Bearer sk-test-abcdef1234567890")

	require.NoError(t, handler.Proxy(ctx))
	require.Len(t, collector.entries, 1)

	entry := collector.entries[0]
	require.Equal(t, "http/proxy", entry.CallType)
	require.Equal(t, "GET /items/{id}", entry.CallIdentifier)
	require.Equal(t, "7", entry.Subject)
	require.Equal(t, 200, entry.ResponseStatus)
	require.Equal(t, "vmcp1abc", entry.Metadata["vmcpID"])
	require.Equal(t, "GET", entry.Metadata["method"])
	require.NotEmpty(t, entry.ProcessingTimeMs)
	// The platform API key is redacted, never stored raw.
	require.NotContains(t, entry.APIKey, "sk-test-abcdef1234567890")
	require.Contains(t, entry.APIKey, "sk-test")
	// Business response body excerpt is the JSON body itself.
	require.JSONEq(t, `{"item":"ok"}`, string(entry.ResponseBody))
}

func TestProxyAuditWriterRecordsExecutionFailure(t *testing.T) {
	executor := &recordingExecutor{
		description: Description{
			Operations: []OperationDescription{{
				Operation: Operation{OperationKey: "POST /items", Method: "POST", PathTemplate: "/items"},
			}},
		},
		executeErr: errors.New("worker_unavailable: worker is unreachable"),
	}
	collector := &recordingAuditCollector{}
	handler := &Handler{Executor: executor, Audit: &AuditWriter{Collector: collector}}
	ctx := testAPIContext(t, http.MethodPost, "/api-proxy/vmcp1abc/component/items", []byte(`{"a":1}`))

	require.NoError(t, handler.Proxy(ctx))
	require.Len(t, collector.entries, 1)
	entry := collector.entries[0]
	require.Equal(t, "http/proxy", entry.CallType)
	require.Equal(t, "POST /items", entry.CallIdentifier)
	require.Zero(t, entry.ResponseStatus)
	require.Contains(t, entry.Error, "worker_unavailable")
	// Request bodies are recorded as bounded metadata, never raw content.
	require.JSONEq(t, `{"originalBytes":7}`, string(entry.RequestBody))
}

func TestProxyAuditWriterNilCollectorIsSafe(t *testing.T) {
	executor := &recordingExecutor{
		description: Description{
			Operations: []OperationDescription{{
				Operation: Operation{OperationKey: "GET /items/{id}", Method: "GET", PathTemplate: "/items/{id}"},
			}},
		},
		result: ExecutionResult{Kind: "business", Status: 200, Body: []byte("ok")},
	}
	handler := &Handler{Executor: executor}
	ctx := testAPIContext(t, http.MethodGet, "/api-proxy/vmcp1abc/component/items/42", nil)
	require.NoError(t, handler.Proxy(ctx))
}

func TestProxyAuditWriterTruncatesLargeBody(t *testing.T) {
	large := make([]byte, auditBodyLimit+1024)
	for i := range large {
		large[i] = 'a'
	}
	executor := &recordingExecutor{
		description: Description{
			Operations: []OperationDescription{{
				Operation: Operation{OperationKey: "GET /items/{id}", Method: "GET", PathTemplate: "/items/{id}"},
			}},
		},
		result: ExecutionResult{Kind: "business", Status: 200, Body: large},
	}
	collector := &recordingAuditCollector{}
	handler := &Handler{Executor: executor, Audit: &AuditWriter{Collector: collector}}
	ctx := testAPIContext(t, http.MethodGet, "/api-proxy/vmcp1abc/component/items/42", nil)

	require.NoError(t, handler.Proxy(ctx))
	entry := collector.entries[0]
	require.Contains(t, string(entry.ResponseBody), `"truncated":true`)
	require.Contains(t, string(entry.ResponseBody), `"originalBytes"`)
}
