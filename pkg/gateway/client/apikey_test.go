package client

import (
	"context"
	"strings"
	"testing"
)

func TestParseAPIKey(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		wantPrefix    string
		wantUserID    uint
		wantKeyID     uint
		wantSecret    string
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:       "valid key",
			key:        "ok1-123-456-secretvalue",
			wantPrefix: "ok1",
			wantUserID: 123,
			wantKeyID:  456,
			wantSecret: "secretvalue",
			wantErr:    false,
		},
		{
			name:       "valid key with base64 secret",
			key:        "ok1-1-1-dGVzdHNlY3JldA",
			wantPrefix: "ok1",
			wantUserID: 1,
			wantKeyID:  1,
			wantSecret: "dGVzdHNlY3JldA",
			wantErr:    false,
		},
		{
			name:       "valid key with large IDs",
			key:        "ok1-999999-888888-longsecretvalue123",
			wantPrefix: "ok1",
			wantUserID: 999999,
			wantKeyID:  888888,
			wantSecret: "longsecretvalue123",
			wantErr:    false,
		},
		{
			name:       "valid key with underscore in secret",
			key:        "ok1-1-2-secret_with_underscores",
			wantPrefix: "ok1",
			wantUserID: 1,
			wantKeyID:  2,
			wantSecret: "secret_with_underscores",
			wantErr:    false,
		},
		{
			name:       "secret starts with a dash",
			key:        "ok1-1-3--secret",
			wantPrefix: "ok1",
			wantUserID: 1,
			wantKeyID:  3,
			wantSecret: "-secret",
			wantErr:    false,
		},
		{
			name:          "empty key",
			key:           "",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "missing prefix",
			key:           "123-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "invalid prefix",
			key:           "ok2-123-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key prefix",
		},
		{
			name:          "wrong prefix length",
			key:           "ok10-123-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "missing user ID",
			key:           "ok1--456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "missing key ID",
			key:           "ok1-123--secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "missing secret",
			key:           "ok1-123-456-",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "non-numeric user ID",
			key:           "ok1-abc-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "non-numeric key ID",
			key:           "ok1-123-xyz-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "only prefix",
			key:           "ok1",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "only prefix with dash",
			key:           "ok1-",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "partial key - missing parts",
			key:           "ok1-123",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "partial key - two parts",
			key:           "ok1-123-456",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "negative user ID",
			key:           "ok1--1-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, userID, keyID, secret, err := ParseAPIKey(tt.key)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseAPIKey(%q) expected error containing %q, got nil", tt.key, tt.wantErrSubstr)
					return
				}
				if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("ParseAPIKey(%q) error = %q, want error containing %q", tt.key, err.Error(), tt.wantErrSubstr)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseAPIKey(%q) unexpected error: %v", tt.key, err)
				return
			}

			if prefix != tt.wantPrefix {
				t.Errorf("ParseAPIKey(%q) prefix = %q, want %q", tt.key, prefix, tt.wantPrefix)
			}
			if userID != tt.wantUserID {
				t.Errorf("ParseAPIKey(%q) userID = %d, want %d", tt.key, userID, tt.wantUserID)
			}
			if keyID != tt.wantKeyID {
				t.Errorf("ParseAPIKey(%q) keyID = %d, want %d", tt.key, keyID, tt.wantKeyID)
			}
			if secret != tt.wantSecret {
				t.Errorf("ParseAPIKey(%q) secret = %q, want %q", tt.key, secret, tt.wantSecret)
			}
		})
	}
}

func TestCreateAPIKeyPersistsAuditLogsAppendPermission(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	created, err := c.CreateAPIKey(ctx, 7, "audit", "audit append key", nil, nil, false, true)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}
	if created.Key == "" {
		t.Fatal("expected plaintext key to be returned once")
	}
	if created.CanAccessSkills {
		t.Fatal("expected skills access to be disabled")
	}
	if !created.CanAppendAuditLogs {
		t.Fatal("expected audit append access to be enabled")
	}

	got, err := c.GetAPIKey(ctx, 7, created.ID)
	if err != nil {
		t.Fatalf("failed to get API key: %v", err)
	}
	if got.CanAccessSkills {
		t.Fatal("expected persisted skills access to be disabled")
	}
	if !got.CanAppendAuditLogs {
		t.Fatal("expected persisted audit append access to be enabled")
	}
	if len(got.MCPServerIDs) != 0 {
		t.Fatalf("expected no MCP server scopes, got %v", got.MCPServerIDs)
	}

	validated, err := c.ValidateAPIKey(ctx, created.Key)
	if err != nil {
		t.Fatalf("failed to validate API key: %v", err)
	}
	if validated.CanAccessSkills {
		t.Fatal("expected validated skills access to be disabled")
	}
	if !validated.CanAppendAuditLogs {
		t.Fatal("expected validated audit append access to be enabled")
	}
}
