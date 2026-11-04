package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

type failingMCPAuditTransformer struct {
	testTransformer
	successfulFields int
	calls            int
	err              error
}

func (f *failingMCPAuditTransformer) TransformToStorage(ctx context.Context, data []byte, dataCtx value.Context) ([]byte, error) {
	f.calls++
	if f.calls > f.successfulFields {
		return nil, f.err
	}

	return f.testTransformer.TransformToStorage(ctx, data, dataCtx)
}

func TestEncryptMCPAuditLogFailure(t *testing.T) {
	tests := []struct {
		name             string
		successfulFields int
	}{
		{
			name:             "total failure",
			successfulFields: 0,
		},
		{
			name:             "partial failure",
			successfulFields: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformErr := errors.New("injected encryption failure")
			transformer := &failingMCPAuditTransformer{
				successfulFields: tt.successfulFields,
				err:              transformErr,
			}
			c := &Client{encryptionConfig: testEncryptionConfig()}
			c.encryptionConfig.Transformers[mcpAuditLogGroupResource] = transformer
			entry := types.MCPAuditLog{
				SourceType: types2.AuditLogSourceTypeMCP,
				MCPFields: &types.MCPAuditLogFields{
					RequestBody:     json.RawMessage(`{"request":"secret"}`),
					ResponseBody:    json.RawMessage(`{"response":"secret"}`),
					ResponseHeaders: json.RawMessage(`{"header":"secret"}`),
				},
			}

			if err := c.encryptMCPAuditLog(t.Context(), &entry); !errors.Is(err, transformErr) {
				t.Fatalf("expected transformation error, got %v", err)
			}
			if entry.Encrypted {
				t.Error("failed encryption marked the entry encrypted")
			}
			if transformer.calls != tt.successfulFields+1 {
				t.Errorf("expected encryption to stop at first failure, got %d calls", transformer.calls)
			}
			requestChanged := !bytes.Equal(entry.MCP().RequestBody, []byte(`{"request":"secret"}`))
			if requestChanged != (tt.successfulFields > 0) {
				t.Error("transformer did not exercise the expected partial encryption state")
			}
			if string(entry.MCP().ResponseBody) != `{"response":"secret"}` {
				t.Error("expected the failed field to remain plaintext")
			}
		})
	}
}

func TestEncryptLocalAgentAuditLogStopsAtFirstFailure(t *testing.T) {
	tests := []struct {
		name             string
		successfulFields int
	}{
		{
			name:             "string",
			successfulFields: 1,
		},
		{
			name:             "git remotes",
			successfulFields: 8,
		},
		{
			name:             "raw message",
			successfulFields: 9,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformErr := errors.New("injected encryption failure")
			transformer := &failingMCPAuditTransformer{
				successfulFields: tt.successfulFields,
				err:              transformErr,
			}
			c := &Client{encryptionConfig: testEncryptionConfig()}
			c.encryptionConfig.Transformers[mcpAuditLogGroupResource] = transformer
			entry := validLocalAgentAuditLog(time.Now().UTC(), "entry-1", types2.AuditLogOutcomeStatusSuccess)

			if err := c.encryptMCPAuditLog(t.Context(), &entry); !errors.Is(err, transformErr) {
				t.Fatalf("expected transformation error, got %v", err)
			}
			if entry.Encrypted {
				t.Error("failed encryption marked the entry encrypted")
			}
			if transformer.calls != tt.successfulFields+1 {
				t.Errorf("expected encryption to stop at first failure, got %d calls", transformer.calls)
			}
		})
	}
}

func TestLogMCPAuditEntryEncryptionFailure(t *testing.T) {
	tests := []struct {
		name             string
		successfulFields int
		responseOnly     bool
	}{
		{
			name:             "total failure",
			successfulFields: 0,
		},
		{
			name:             "partial failure",
			successfulFields: 1,
		},
		{
			name:             "response total failure",
			successfulFields: 0,
			responseOnly:     true,
		},
		{
			name:             "response partial failure",
			successfulFields: 1,
			responseOnly:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			c.encryptionConfig = testEncryptionConfig()
			c.auditBuffer = make([]types.MCPAuditLog, 0, 2)
			c.auditLogBatchSize = 1
			c.kickAuditPersist = make(chan struct{}, 1)
			entry := types.MCPAuditLog{
				CreatedAt:  time.Now().UTC(),
				SourceType: types2.AuditLogSourceTypeMCP,
				UserID:     "user-1",
				MCPFields: &types.MCPAuditLogFields{
					MCPID:           "mcp-1",
					RequestID:       "request-1",
					ProxyExchangeID: "exchange-1",
					RequestBody:     json.RawMessage(`{"request":"secret"}`),
				},
			}

			var before types.MCPAuditLog
			if tt.responseOnly {
				c.LogMCPAuditEntry(entry)
				if err := c.persistMCPAuditLogs(); err != nil {
					t.Fatalf("persist encrypted request: %v", err)
				}
				if err := c.db.WithContext(t.Context()).First(&before).Error; err != nil {
					t.Fatalf("load encrypted request: %v", err)
				}
				if !before.Encrypted {
					t.Fatal("expected existing request to be encrypted")
				}
				<-c.kickAuditPersist

				// Use fresh fields because logging encrypts the supplied fields in place.
				entry.MCPFields = &types.MCPAuditLogFields{
					MCPID:                "mcp-1",
					RequestID:            "request-1",
					ProxyExchangeID:      "exchange-1",
					ResponseReceived:     true,
					OriginalResponseBody: json.RawMessage(`{"original":"secret"}`),
				}
			}
			entry.MCP().ResponseBody = json.RawMessage(`{"response":"secret"}`)
			c.encryptionConfig.Transformers[mcpAuditLogGroupResource] = &failingMCPAuditTransformer{
				successfulFields: tt.successfulFields,
				err:              errors.New("injected encryption failure"),
			}

			c.LogMCPAuditEntry(entry)

			if got := len(c.auditBuffer); got != 0 {
				t.Errorf("failed entry entered buffer: got %d entries", got)
			}
			if len(c.kickAuditPersist) != 0 {
				t.Error("failed entry notified persistence loop")
			}
			if err := c.persistMCPAuditLogs(); err != nil {
				t.Fatalf("flush audit buffer: %v", err)
			}

			wantCount := int64(0)
			if tt.responseOnly {
				wantCount = 1
				var after types.MCPAuditLog
				if err := c.db.WithContext(t.Context()).First(&after).Error; err != nil {
					t.Fatalf("load request after failed response: %v", err)
				}
				if !reflect.DeepEqual(before, after) {
					t.Error("failed response modified the existing encrypted request")
				}
			}
			if got := countAuditLogs(t, c); got != wantCount {
				t.Errorf("expected %d persisted entries, got %d", wantCount, got)
			}
		})
	}
}

func TestLogMCPAuditEntryEncryptionCompatibility(t *testing.T) {
	tests := []struct {
		name      string
		config    *encryptionconfig.EncryptionConfiguration
		encrypted bool
	}{
		{
			name:      "encrypted",
			config:    testEncryptionConfig(),
			encrypted: true,
		},
		{
			name: "no encryption config",
		},
		{
			name:   "missing transformer",
			config: &encryptionconfig.EncryptionConfiguration{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			c.encryptionConfig = tt.config
			fields := types.MCPAuditLogFields{
				MCPID:                "mcp-1",
				RequestBody:          json.RawMessage(`{"request":"secret"}`),
				MutatedRequestBody:   json.RawMessage(`{"mutated":"secret"}`),
				ResponseBody:         json.RawMessage(`{"response":"secret"}`),
				OriginalResponseBody: json.RawMessage(`{"original":"secret"}`),
				RequestHeaders:       json.RawMessage(`{"request-header":"secret"}`),
				ResponseHeaders:      json.RawMessage(`{"response-header":"secret"}`),
				RequestMutated:       true,
				ResponseMutated:      true,
				ResponseReceived:     true,
			}
			want := fields

			c.LogMCPAuditEntry(types.MCPAuditLog{
				CreatedAt: time.Now().UTC(),
				UserID:    "user-1",
				MCPFields: &fields,
			})

			if len(c.auditBuffer) != 1 || c.auditBuffer[0].Encrypted != tt.encrypted {
				t.Fatal("expected entry to be queued with the configured encryption state")
			}
			if err := c.persistMCPAuditLogs(); err != nil {
				t.Fatalf("persist audit log: %v", err)
			}
			if got := countAuditLogs(t, c); got != 1 {
				t.Fatalf("expected one persisted entry, got %d", got)
			}

			var stored types.MCPAuditLog
			if err := c.db.WithContext(t.Context()).First(&stored).Error; err != nil {
				t.Fatalf("load audit log: %v", err)
			}
			if stored.Encrypted != tt.encrypted {
				t.Fatal("unexpected persisted encryption state")
			}
			if tt.encrypted && bytes.Equal(stored.MCP().RequestBody, want.RequestBody) {
				t.Error("expected ciphertext at rest")
			}
			got, err := c.GetMCPAuditLog(t.Context(), stored.ID, true)
			if err != nil {
				t.Fatalf("read audit log payload: %v", err)
			}
			if !reflect.DeepEqual(got.MCP(), &want) {
				t.Error("persisted payload did not round trip")
			}
		})
	}
}

func TestInsertLocalAgentAuditLogsRejectsEncryptionFailure(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = testEncryptionConfig()
	transformErr := errors.New("injected encryption failure")
	c.encryptionConfig.Transformers[mcpAuditLogGroupResource] = &failingMCPAuditTransformer{
		successfulFields: 1,
		err:              transformErr,
	}
	entry := validLocalAgentAuditLog(time.Now().UTC(), "entry-1", types2.AuditLogOutcomeStatusSuccess)

	if err := c.InsertLocalAgentAuditLogs(t.Context(), []types.MCPAuditLog{entry}); !errors.Is(err, transformErr) {
		t.Fatalf("expected transformation error, got %v", err)
	}
	if got := countAuditLogs(t, c); got != 0 {
		t.Fatalf("expected no persisted local-agent entries, got %d", got)
	}
}
