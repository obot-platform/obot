package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

func TestInsertLLMAuditLogEncryptsSensitiveFields(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			llmAuditLogGroupResource: testTransformer{},
		},
	}

	entry := types.LLMAuditLog{
		ID:                        uuid.NewString(),
		CreatedAt:                 time.Now().UTC(),
		UserID:                    "user-1",
		RequestHeaders:            json.RawMessage(`{"Authorization":["[REDACTED]"]}`),
		RequestBody:               json.RawMessage(`{"prompt":"secret"}`),
		PolicyModifiedRequestBody: json.RawMessage(`{"prompt":"policy modified"}`),
		MessagePolicyTriggered:    true,
		ResponseHeaders:           json.RawMessage(`{"Content-Type":["application/json"]}`),
		ResponseBody:              json.RawMessage(`{"id":"resp-1"}`),
		ClientSessionID:           "session-1",
	}
	want := entry

	if err := c.InsertLLMAuditLog(t.Context(), &entry); err != nil {
		t.Fatalf("failed to insert LLM audit log: %v", err)
	}

	var stored types.LLMAuditLog
	if err := c.db.WithContext(t.Context()).First(&stored, "id = ?", entry.ID).Error; err != nil {
		t.Fatalf("failed to get LLM audit log: %v", err)
	}

	if !stored.Encrypted {
		t.Fatal("expected encrypted audit log")
	}
	if stored.UserID != "user-1" || stored.ClientSessionID != "session-1" {
		t.Fatalf("expected query fields to remain plaintext, got user=%q session=%q", stored.UserID, stored.ClientSessionID)
	}
	for name, value := range map[string]json.RawMessage{
		"request headers":              stored.RequestHeaders,
		"request body":                 stored.RequestBody,
		"policy-modified request body": stored.PolicyModifiedRequestBody,
		"response headers":             stored.ResponseHeaders,
		"response body":                stored.ResponseBody,
	} {
		decoded, err := base64.StdEncoding.DecodeString(string(value))
		if err != nil {
			t.Fatalf("expected %s to be base64 ciphertext: %v", name, err)
		}
		if !bytes.HasPrefix(decoded, []byte("encrypted:")) {
			t.Fatalf("expected %s to be encrypted, got %q", name, decoded)
		}
	}

	if err := c.decryptLLMAuditLog(t.Context(), &stored); err != nil {
		t.Fatalf("failed to decrypt LLM audit log: %v", err)
	}
	if !bytes.Equal(stored.RequestHeaders, want.RequestHeaders) {
		t.Fatalf("expected decrypted request headers %q, got %q", want.RequestHeaders, stored.RequestHeaders)
	}
	if !bytes.Equal(stored.RequestBody, want.RequestBody) {
		t.Fatalf("expected decrypted request body %q, got %q", want.RequestBody, stored.RequestBody)
	}
	if !bytes.Equal(stored.PolicyModifiedRequestBody, want.PolicyModifiedRequestBody) {
		t.Fatalf("expected decrypted policy-modified request body %q, got %q", want.PolicyModifiedRequestBody, stored.PolicyModifiedRequestBody)
	}
	if !bytes.Equal(stored.ResponseHeaders, want.ResponseHeaders) {
		t.Fatalf("expected decrypted response headers %q, got %q", want.ResponseHeaders, stored.ResponseHeaders)
	}
	if !bytes.Equal(stored.ResponseBody, want.ResponseBody) {
		t.Fatalf("expected decrypted response body %q, got %q", want.ResponseBody, stored.ResponseBody)
	}
}

func TestInsertLLMAuditLogWithoutEncryptionStoresPlaintext(t *testing.T) {
	c := newTestClient(t)
	entry := types.LLMAuditLog{
		ID:             uuid.NewString(),
		CreatedAt:      time.Now().UTC(),
		RequestHeaders: json.RawMessage(`{"Authorization":["[REDACTED]"]}`),
		RequestBody:    json.RawMessage(`{"prompt":"secret"}`),
	}

	if err := c.InsertLLMAuditLog(t.Context(), &entry); err != nil {
		t.Fatalf("failed to insert LLM audit log: %v", err)
	}

	var stored types.LLMAuditLog
	if err := c.db.WithContext(t.Context()).First(&stored, "id = ?", entry.ID).Error; err != nil {
		t.Fatalf("failed to get LLM audit log: %v", err)
	}
	if stored.Encrypted {
		t.Fatal("expected plaintext audit log")
	}
	if !bytes.Equal(stored.RequestHeaders, entry.RequestHeaders) || !bytes.Equal(stored.RequestBody, entry.RequestBody) {
		t.Fatalf("expected plaintext fields, got headers=%q body=%q", stored.RequestHeaders, stored.RequestBody)
	}
	if err := c.decryptLLMAuditLog(t.Context(), &stored); err != nil {
		t.Fatalf("failed to decrypt plaintext LLM audit log: %v", err)
	}
	if !bytes.Equal(stored.RequestHeaders, entry.RequestHeaders) || !bytes.Equal(stored.RequestBody, entry.RequestBody) {
		t.Fatalf("expected decrypt no-op, got headers=%q body=%q", stored.RequestHeaders, stored.RequestBody)
	}
}

func TestGetLLMAuditLogStripsSensitiveFields(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			llmAuditLogGroupResource: testTransformer{},
		},
	}
	entry := types.LLMAuditLog{
		ID:                        uuid.NewString(),
		CreatedAt:                 time.Now().UTC(),
		UserID:                    "user-1",
		RequestHeaders:            json.RawMessage(`{"Authorization":["[REDACTED]"]}`),
		RequestBody:               json.RawMessage(`{"prompt":"secret"}`),
		PolicyModifiedRequestBody: json.RawMessage(`{"prompt":"policy modified"}`),
		MessagePolicyTriggered:    true,
		ResponseHeaders:           json.RawMessage(`{"Content-Type":["application/json"]}`),
		ResponseBody:              json.RawMessage(`{"id":"resp-1"}`),
		ClientSessionID:           "session-1",
	}
	if err := c.InsertLLMAuditLog(t.Context(), &entry); err != nil {
		t.Fatalf("failed to insert LLM audit log: %v", err)
	}

	got, err := c.GetLLMAuditLog(t.Context(), entry.ID, false)
	if err != nil {
		t.Fatalf("failed to get LLM audit log: %v", err)
	}
	if got.UserID != entry.UserID || got.ClientSessionID != entry.ClientSessionID {
		t.Fatalf("expected metadata to remain, got user=%q session=%q", got.UserID, got.ClientSessionID)
	}
	if !got.MessagePolicyTriggered {
		t.Fatal("expected input policy trigger metadata to remain")
	}
	if len(got.RequestHeaders) != 0 || len(got.RequestBody) != 0 || len(got.PolicyModifiedRequestBody) != 0 || len(got.ResponseHeaders) != 0 || len(got.ResponseBody) != 0 {
		t.Fatalf("expected sensitive fields to be stripped, got %#v", got)
	}
}

func TestGetLLMAuditLogDecryptsSensitiveFields(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			llmAuditLogGroupResource: testTransformer{},
		},
	}
	entry := types.LLMAuditLog{
		ID:                        uuid.NewString(),
		CreatedAt:                 time.Now().UTC(),
		RequestHeaders:            json.RawMessage(`{"Authorization":["[REDACTED]"]}`),
		RequestBody:               json.RawMessage(`{"prompt":"secret"}`),
		PolicyModifiedRequestBody: json.RawMessage(`{"prompt":"policy modified"}`),
		MessagePolicyTriggered:    true,
		ResponseHeaders:           json.RawMessage(`{"Content-Type":["application/json"]}`),
		ResponseBody:              json.RawMessage(`{"id":"resp-1"}`),
	}
	if err := c.InsertLLMAuditLog(t.Context(), &entry); err != nil {
		t.Fatalf("failed to insert LLM audit log: %v", err)
	}

	got, err := c.GetLLMAuditLog(t.Context(), entry.ID, true)
	if err != nil {
		t.Fatalf("failed to get LLM audit log: %v", err)
	}
	if !bytes.Equal(got.RequestHeaders, entry.RequestHeaders) || !bytes.Equal(got.RequestBody, entry.RequestBody) || !bytes.Equal(got.PolicyModifiedRequestBody, entry.PolicyModifiedRequestBody) || !bytes.Equal(got.ResponseHeaders, entry.ResponseHeaders) || !bytes.Equal(got.ResponseBody, entry.ResponseBody) {
		t.Fatalf("expected decrypted sensitive fields, got %#v", got)
	}
	if !got.MessagePolicyTriggered {
		t.Fatal("expected input policy trigger metadata")
	}
}

func TestGetLLMAuditLogsFiltersAndStripsSensitiveFields(t *testing.T) {
	c := newTestClient(t)
	for _, entry := range []types.LLMAuditLog{
		{ID: uuid.NewString(), CreatedAt: time.Now().UTC(), UserID: "user-1", ModelProvider: system.OpenAIModelProvider, RequestBody: json.RawMessage(`{"prompt":"secret"}`), PolicyModifiedRequestBody: json.RawMessage(`{"prompt":"blocked"}`), MessagePolicyTriggered: true},
		{ID: uuid.NewString(), CreatedAt: time.Now().UTC(), UserID: "user-2", ModelProvider: system.AnthropicModelProvider, RequestBody: json.RawMessage(`{"prompt":"secret"}`)},
	} {
		if err := c.InsertLLMAuditLog(t.Context(), &entry); err != nil {
			t.Fatalf("failed to insert LLM audit log: %v", err)
		}
	}

	logs, total, err := c.GetLLMAuditLogs(t.Context(), LLMAuditLogOptions{ModelProvider: []string{system.OpenAIModelProvider}})
	if err != nil {
		t.Fatalf("failed to list LLM audit logs: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected one LLM audit log, got total=%d len=%d", total, len(logs))
	}
	if logs[0].ModelProvider != system.OpenAIModelProvider || len(logs[0].RequestBody) != 0 {
		t.Fatalf("expected filtered metadata without sensitive fields, got %#v", logs[0])
	}
	if !logs[0].MessagePolicyTriggered || len(logs[0].PolicyModifiedRequestBody) != 0 {
		t.Fatalf("expected policy trigger metadata without policy-modified body, got %#v", logs[0])
	}
}

func TestGetLLMAuditLogsExcludesModelsRequestsByDefault(t *testing.T) {
	c := newTestClient(t)
	now := time.Now().UTC()
	for _, path := range []string{
		"/api/llm-proxy/openai/v1/responses",
		"/api/llm-proxy/openai/models",
		"/api/llm-proxy/openai/v1/models",
		"/api/llm-proxy/anthropic/models/",
		"/api/llm-proxy/openai/models/model-1",
	} {
		entry := types.LLMAuditLog{ID: uuid.NewString(), CreatedAt: now, RequestPath: path}
		if err := c.InsertLLMAuditLog(t.Context(), &entry); err != nil {
			t.Fatalf("failed to insert LLM audit log for %q: %v", path, err)
		}
	}

	logs, total, err := c.GetLLMAuditLogs(t.Context(), LLMAuditLogOptions{})
	if err != nil {
		t.Fatalf("failed to list LLM audit logs: %v", err)
	}
	if total != 2 || len(logs) != 2 {
		t.Fatalf("expected two non-models requests, got total=%d len=%d", total, len(logs))
	}
	paths := []string{logs[0].RequestPath, logs[1].RequestPath}
	slices.Sort(paths)
	if !slices.Equal(paths, []string{
		"/api/llm-proxy/openai/models/model-1",
		"/api/llm-proxy/openai/v1/responses",
	}) {
		t.Fatalf("expected non-models request paths, got %v", paths)
	}

	logs, total, err = c.GetLLMAuditLogs(t.Context(), LLMAuditLogOptions{IncludeModelsRequests: true})
	if err != nil {
		t.Fatalf("failed to list all LLM audit logs: %v", err)
	}
	if total != 5 || len(logs) != 5 {
		t.Fatalf("expected all requests, got total=%d len=%d", total, len(logs))
	}

	options, err := c.GetLLMAuditLogFilterOptions(t.Context(), "request_path", LLMAuditLogOptions{}, "")
	if err != nil {
		t.Fatalf("failed to list request path options: %v", err)
	}
	slices.Sort(options)
	if !slices.Equal(options, paths) {
		t.Fatalf("expected filtered request path options %v, got %v", paths, options)
	}
}

func TestGetLLMAuditLogFilterOptions(t *testing.T) {
	c := newTestClient(t)
	if !c.db.WithContext(t.Context()).Migrator().HasIndex(&types.LLMAuditLog{}, "idx_llm_audit_message_policy_triggered_created") {
		t.Fatal("expected input policy trigger index")
	}
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	for _, entry := range []types.LLMAuditLog{
		{ID: uuid.NewString(), CreatedAt: now, ModelProvider: system.OpenAIModelProvider, TargetModel: "model-a", ResponseStatus: 200, UserAgent: "open-webui/1.0", PolicyModifiedRequestBody: json.RawMessage(`{"prompt":"blocked"}`), MessagePolicyTriggered: true},
		{ID: uuid.NewString(), CreatedAt: now.Add(time.Minute), ModelProvider: system.OpenAIModelProvider, TargetModel: "model-c", ResponseStatus: 500, UserAgent: "obot/1.0"},
		{ID: uuid.NewString(), CreatedAt: now, ModelProvider: system.OpenAIModelProvider, TargetModel: "", ResponseStatus: 0, UserAgent: ""},
		{ID: uuid.NewString(), CreatedAt: now, ModelProvider: system.AnthropicModelProvider, TargetModel: "model-b", ResponseStatus: 200, UserAgent: "claude/1.0"},
	} {
		if err := c.InsertLLMAuditLog(t.Context(), &entry); err != nil {
			t.Fatalf("failed to insert LLM audit log: %v", err)
		}
	}

	options, err := c.GetLLMAuditLogFilterOptions(t.Context(), "target_model", LLMAuditLogOptions{ModelProvider: []string{system.OpenAIModelProvider}}, "")
	if err != nil {
		t.Fatalf("failed to get target model filter options: %v", err)
	}
	slices.Sort(options)
	if !slices.Equal(options, []string{"model-a", "model-c"}) {
		t.Fatalf("expected target model options, got %v", options)
	}

	limited, err := c.GetLLMAuditLogFilterOptions(t.Context(), "user_agent", LLMAuditLogOptions{ModelProvider: []string{system.OpenAIModelProvider}, Limit: 1}, "")
	if err != nil {
		t.Fatalf("failed to get limited user agent options: %v", err)
	}
	if !slices.Equal(limited, []string{"obot/1.0"}) {
		t.Fatalf("expected deterministic limited options, got %v", limited)
	}

	filtered, total, err := c.GetLLMAuditLogs(t.Context(), LLMAuditLogOptions{UserAgent: []string{"obot/1.0"}})
	if err != nil {
		t.Fatalf("failed to filter by user agent: %v", err)
	}
	if total != 1 || len(filtered) != 1 || filtered[0].UserAgent != "obot/1.0" {
		t.Fatalf("expected one user agent match, got total=%d logs=%#v", total, filtered)
	}

	statuses, err := c.GetLLMAuditLogFilterOptions(t.Context(), "response_status", LLMAuditLogOptions{ModelProvider: []string{system.OpenAIModelProvider}}, 0)
	if err != nil {
		t.Fatalf("failed to get response status options: %v", err)
	}
	slices.Sort(statuses)
	if !slices.Equal(statuses, []string{"200", "500"}) {
		t.Fatalf("expected response status options, got %v", statuses)
	}

	policyTriggered, err := c.GetLLMAuditLogFilterOptions(t.Context(), "message_policy_triggered", LLMAuditLogOptions{ModelProvider: []string{system.OpenAIModelProvider}})
	if err != nil {
		t.Fatalf("failed to get message policy triggered filter options: %v", err)
	}
	slices.Sort(policyTriggered)
	if !slices.Equal(policyTriggered, []string{"false", "true"}) {
		t.Fatalf("expected message policy triggered options, got %v", policyTriggered)
	}
	policyTriggered, err = c.GetLLMAuditLogFilterOptions(t.Context(), "message_policy_triggered", LLMAuditLogOptions{ModelProvider: []string{system.AnthropicModelProvider}})
	if err != nil {
		t.Fatalf("failed to get filtered message policy triggered options: %v", err)
	}
	if !slices.Equal(policyTriggered, []string{"false"}) {
		t.Fatalf("expected filtered message policy triggered options, got %v", policyTriggered)
	}

	triggeredLogs, total, err := c.GetLLMAuditLogs(t.Context(), LLMAuditLogOptions{MessagePolicyTriggered: []bool{true}})
	if err != nil {
		t.Fatalf("failed to filter input policy triggered logs: %v", err)
	}
	if total != 1 || len(triggeredLogs) != 1 || !triggeredLogs[0].MessagePolicyTriggered {
		t.Fatalf("expected one input policy triggered log, got total=%d logs=%#v", total, triggeredLogs)
	}

	notTriggeredLogs, total, err := c.GetLLMAuditLogs(t.Context(), LLMAuditLogOptions{MessagePolicyTriggered: []bool{false}})
	if err != nil {
		t.Fatalf("failed to filter non-triggered input policy logs: %v", err)
	}
	if total != 3 || len(notTriggeredLogs) != 3 {
		t.Fatalf("expected three non-triggered input policy logs, got total=%d logs=%#v", total, notTriggeredLogs)
	}
}

func TestLogLLMAuditEntryQueuesPlaintextWithoutBlocking(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			llmAuditLogGroupResource: testTransformer{},
		},
	}
	entry := types.LLMAuditLog{
		ID:            uuid.NewString(),
		CreatedAt:     time.Now().UTC(),
		ModelProvider: system.OpenAIModelProvider,
		RequestBody:   json.RawMessage(`{"prompt":"secret"}`),
	}

	c.LogLLMAuditEntry(entry, []byte(`data: {"type":"response.created","response":{"id":"resp_1","output":[]}}`+"\n"))

	queued := <-c.llmAuditEntries
	if queued.log.Encrypted {
		t.Fatal("expected request-path enqueue to skip encryption")
	}
	if !bytes.Equal(queued.log.RequestBody, entry.RequestBody) {
		t.Fatalf("expected plaintext queued body %q, got %q", entry.RequestBody, queued.log.RequestBody)
	}
	if len(queued.responseStream) == 0 {
		t.Fatal("expected raw response stream to be queued")
	}
}

func TestLogLLMAuditEntryDropsWhenBufferFull(t *testing.T) {
	c := newTestClient(t)
	c.llmAuditEntries = make(chan llmAuditEntry, 1)

	c.LogLLMAuditEntry(types.LLMAuditLog{ID: uuid.NewString(), CreatedAt: time.Now().UTC()}, nil)
	c.LogLLMAuditEntry(types.LLMAuditLog{ID: uuid.NewString(), CreatedAt: time.Now().UTC()}, nil)

	if got := len(c.llmAuditEntries); got != 1 {
		t.Fatalf("expected one queued entry, got %d", got)
	}
}

func TestLogLLMAuditEntryNoopsWhenDisabled(t *testing.T) {
	c := newTestClient(t)
	c.llmAuditEnabled = false

	c.LogLLMAuditEntry(types.LLMAuditLog{ID: uuid.NewString(), CreatedAt: time.Now().UTC()}, nil)

	if got := len(c.llmAuditEntries); got != 0 {
		t.Fatalf("expected no queued entries, got %d", got)
	}
}

func TestRunLLMAuditPersistenceLoopFlushesQueuedEntries(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			llmAuditLogGroupResource: testTransformer{},
		},
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.runLLMAuditPersistenceLoop(ctx, 3, time.Hour)
	}()

	for i := range 3 {
		c.LogLLMAuditEntry(types.LLMAuditLog{
			ID:            uuid.NewString(),
			CreatedAt:     time.Now().UTC(),
			ModelProvider: system.OpenAIModelProvider,
			RequestBody:   json.RawMessage(fmt.Sprintf(`{"prompt":"secret-%d"}`, i)),
		}, []byte(`data: {"type":"response.created","response":{"id":"resp_async","output":[]}}`+"\n"))
	}

	waitForLLMAuditLogCount(t, c, 3)
	cancel()
	<-done

	var stored types.LLMAuditLog
	if err := c.db.WithContext(t.Context()).First(&stored).Error; err != nil {
		t.Fatalf("failed to get LLM audit log: %v", err)
	}
	if !stored.Encrypted {
		t.Fatal("expected writer to encrypt persisted audit logs")
	}
	if err := c.decryptLLMAuditLog(t.Context(), &stored); err != nil {
		t.Fatalf("failed to decrypt LLM audit log: %v", err)
	}
	if stored.ResponseID != "resp_async" {
		t.Fatalf("expected async response aggregation, got response ID %q", stored.ResponseID)
	}
}

func TestRunLLMAuditPersistenceLoopFlushesOnInterval(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.runLLMAuditPersistenceLoop(ctx, 3, 20*time.Millisecond)
	}()

	c.LogLLMAuditEntry(types.LLMAuditLog{ID: uuid.NewString(), CreatedAt: time.Now().UTC()}, nil)
	waitForLLMAuditLogCount(t, c, 1)
	cancel()
	<-done
}

func TestRunLLMAuditPersistenceLoopDrainsOnShutdown(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.runLLMAuditPersistenceLoop(ctx, 3, time.Hour)
	}()

	c.LogLLMAuditEntry(types.LLMAuditLog{ID: uuid.NewString(), CreatedAt: time.Now().UTC()}, nil)
	c.LogLLMAuditEntry(types.LLMAuditLog{ID: uuid.NewString(), CreatedAt: time.Now().UTC()}, nil)
	cancel()
	<-done

	waitForLLMAuditLogCount(t, c, 2)
}

func waitForLLMAuditLogCount(t *testing.T, c *Client, want int64) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d LLM audit logs, got %d", want, countLLMAuditLogs(t, c))
		case <-tick.C:
			if got := countLLMAuditLogs(t, c); got == want {
				return
			}
		}
	}
}
