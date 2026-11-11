package client

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/principal"
)

func TestParseAPIKey(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		wantOwnerKind principal.Kind
		wantOwnerID   uint
		wantKeyID     uint
		wantSecret    string
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:          "valid key",
			key:           "ok1-123-456-secretvalue",
			wantOwnerKind: principal.KindUser,
			wantOwnerID:   123,
			wantKeyID:     456,
			wantSecret:    "secretvalue",
			wantErr:       false,
		},
		{
			name:          "valid key with base64 secret",
			key:           "ok1-1-1-dGVzdHNlY3JldA",
			wantOwnerKind: principal.KindUser,
			wantOwnerID:   1,
			wantKeyID:     1,
			wantSecret:    "dGVzdHNlY3JldA",
			wantErr:       false,
		},
		{
			name:          "valid key with large IDs",
			key:           "ok1-999999-888888-longsecretvalue123",
			wantOwnerKind: principal.KindUser,
			wantOwnerID:   999999,
			wantKeyID:     888888,
			wantSecret:    "longsecretvalue123",
			wantErr:       false,
		},
		{
			name:          "valid key with underscore in secret",
			key:           "ok1-1-2-secret_with_underscores",
			wantOwnerKind: principal.KindUser,
			wantOwnerID:   1,
			wantKeyID:     2,
			wantSecret:    "secret_with_underscores",
			wantErr:       false,
		},
		{
			name:          "secret starts with a dash",
			key:           "ok1-1-3--secret",
			wantOwnerKind: principal.KindUser,
			wantOwnerID:   1,
			wantKeyID:     3,
			wantSecret:    "-secret",
			wantErr:       false,
		},
		{
			name:          "valid agent key",
			key:           "oak1-123-456-secretvalue",
			wantOwnerKind: principal.KindAgent,
			wantOwnerID:   123,
			wantKeyID:     456,
			wantSecret:    "secretvalue",
		},
		{
			name:          "agent key whose secret contains dashes",
			key:           "oak1-5-7--a-b-",
			wantOwnerKind: principal.KindAgent,
			wantOwnerID:   5,
			wantKeyID:     7,
			wantSecret:    "-a-b-",
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
			wantErrSubstr: "invalid API key prefix",
		},
		{
			name:          "prefix in another case",
			key:           "OK1-123-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key prefix",
		},
		{
			name:          "agent prefix in another case",
			key:           "OAK1-123-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key prefix",
		},
		{
			name:          "service account prefix",
			key:           "osa1-123-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key prefix",
		},
		{
			name:          "agent key missing secret",
			key:           "oak1-123-456-",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "agent key with non-numeric agent ID",
			key:           "oak1-agent:5-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "signed user ID",
			key:           "ok1-+5-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "user ID with a space",
			key:           "ok1- 5-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid API key format",
		},
		{
			name:          "user ID overflows",
			key:           "ok1-99999999999999999999999-456-secret",
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
			ownerKind, ownerID, keyID, secret, err := ParseAPIKey(tt.key)

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

			if ownerKind != tt.wantOwnerKind {
				t.Errorf("ParseAPIKey(%q) owner kind = %q, want %q", tt.key, ownerKind, tt.wantOwnerKind)
			}
			if ownerID != tt.wantOwnerID {
				t.Errorf("ParseAPIKey(%q) owner ID = %d, want %d", tt.key, ownerID, tt.wantOwnerID)
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

func TestParseRedactedAPIKey(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		wantOwnerKind principal.Kind
		wantOwnerID   uint
		wantKeyID     uint
		wantErr       bool
	}{
		{
			name:          "valid redacted key",
			key:           "ok1-123-456-",
			wantOwnerKind: principal.KindUser,
			wantOwnerID:   123,
			wantKeyID:     456,
		},
		{
			name:          "valid redacted key with short ID fragment",
			key:           "ok1-7-1-abcd",
			wantOwnerKind: principal.KindUser,
			wantOwnerID:   7,
			wantKeyID:     1,
		},
		{
			name:          "valid redacted agent key",
			key:           "oak1-123-456-",
			wantOwnerKind: principal.KindAgent,
			wantOwnerID:   123,
			wantKeyID:     456,
		},
		{
			name:          "valid redacted agent key with short ID fragment",
			key:           "oak1-7-1-abc",
			wantOwnerKind: principal.KindAgent,
			wantOwnerID:   7,
			wantKeyID:     1,
		},
		{
			name:    "full agent key is rejected",
			key:     "oak1-123-456-secret",
			wantErr: true,
		},
		{
			name:    "agent key redacted to the user key length",
			key:     "oak1-7-1-abcd",
			wantErr: true,
		},
		{
			name:    "full key is rejected",
			key:     "ok1-123-456-secret",
			wantErr: true,
		},
		{
			name:    "wrong prefix",
			key:     "ok2-123-456-",
			wantErr: true,
		},
		{
			name:    "missing user ID",
			key:     "ok1--456-",
			wantErr: true,
		},
		{
			name:    "missing key ID",
			key:     "ok1-123--",
			wantErr: true,
		},
		{
			name:    "zero user ID",
			key:     "ok1-0-456-",
			wantErr: true,
		},
		{
			name:    "zero key ID",
			key:     "ok1-123-0-",
			wantErr: true,
		},
		{
			name:    "not an API key",
			key:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ownerKind, ownerID, keyID, err := ParseRedactedAPIKey(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRedactedAPIKey(%q) expected error", tt.key)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRedactedAPIKey(%q): %v", tt.key, err)
			}
			if ownerKind != tt.wantOwnerKind || ownerID != tt.wantOwnerID || keyID != tt.wantKeyID {
				t.Fatalf("ParseRedactedAPIKey(%q) = (%q, %d, %d), want (%q, %d, %d)", tt.key, ownerKind, ownerID, keyID, tt.wantOwnerKind, tt.wantOwnerID, tt.wantKeyID)
			}
		})
	}
}
