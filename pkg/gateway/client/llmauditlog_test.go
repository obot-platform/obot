package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

type testTransformer struct{}

func (testTransformer) TransformToStorage(_ context.Context, data []byte, _ value.Context) ([]byte, error) {
	return append([]byte("encrypted:"), data...), nil
}

func (testTransformer) TransformFromStorage(_ context.Context, data []byte, _ value.Context) ([]byte, bool, error) {
	return bytes.TrimPrefix(data, []byte("encrypted:")), false, nil
}

func TestInsertLLMAuditLogEncryptsSensitiveFields(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			llmAuditLogGroupResource: testTransformer{},
		},
	}

	entry := types.LLMAuditLog{
		ID:                  uuid.NewString(),
		CreatedAt:           time.Now().UTC(),
		UserID:              "user-1",
		RequestHeaders:      `{"Authorization":["[REDACTED]"]}`,
		RequestBody:         `{"prompt":"secret"}`,
		RedactedRequestBody: `{"prompt":"redacted"}`,
		ResponseHeaders:     `{"Content-Type":["application/json"]}`,
		ResponseBody:        `{"id":"resp-1"}`,
		ClientSessionID:     "session-1",
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
	for name, value := range map[string]string{
		"request headers":       stored.RequestHeaders,
		"request body":          stored.RequestBody,
		"redacted request body": stored.RedactedRequestBody,
		"response headers":      stored.ResponseHeaders,
		"response body":         stored.ResponseBody,
	} {
		decoded, err := base64.StdEncoding.DecodeString(value)
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
	if stored.RequestHeaders != want.RequestHeaders {
		t.Fatalf("expected decrypted request headers %q, got %q", want.RequestHeaders, stored.RequestHeaders)
	}
	if stored.RequestBody != want.RequestBody {
		t.Fatalf("expected decrypted request body %q, got %q", want.RequestBody, stored.RequestBody)
	}
	if stored.RedactedRequestBody != want.RedactedRequestBody {
		t.Fatalf("expected decrypted redacted request body %q, got %q", want.RedactedRequestBody, stored.RedactedRequestBody)
	}
	if stored.ResponseHeaders != want.ResponseHeaders {
		t.Fatalf("expected decrypted response headers %q, got %q", want.ResponseHeaders, stored.ResponseHeaders)
	}
	if stored.ResponseBody != want.ResponseBody {
		t.Fatalf("expected decrypted response body %q, got %q", want.ResponseBody, stored.ResponseBody)
	}
}

func TestInsertLLMAuditLogWithoutEncryptionStoresPlaintext(t *testing.T) {
	c := newTestClient(t)
	entry := types.LLMAuditLog{
		ID:             uuid.NewString(),
		CreatedAt:      time.Now().UTC(),
		RequestHeaders: `{"Authorization":["[REDACTED]"]}`,
		RequestBody:    `{"prompt":"secret"}`,
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
	if stored.RequestHeaders != entry.RequestHeaders || stored.RequestBody != entry.RequestBody {
		t.Fatalf("expected plaintext fields, got headers=%q body=%q", stored.RequestHeaders, stored.RequestBody)
	}
	if err := c.decryptLLMAuditLog(t.Context(), &stored); err != nil {
		t.Fatalf("failed to decrypt plaintext LLM audit log: %v", err)
	}
	if stored.RequestHeaders != entry.RequestHeaders || stored.RequestBody != entry.RequestBody {
		t.Fatalf("expected decrypt no-op, got headers=%q body=%q", stored.RequestHeaders, stored.RequestBody)
	}
}
