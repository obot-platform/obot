package client

import (
	"strings"
	"testing"
)

func TestParseObotMCPToken(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		wantPrefix    string
		wantUserID    uint
		wantTokenID   uint
		wantSecret    string
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:        "valid token",
			token:       "mt1-123-456-secretvalue",
			wantPrefix:  "mt1",
			wantUserID:  123,
			wantTokenID: 456,
			wantSecret:  "secretvalue",
			wantErr:     false,
		},
		{
			name:        "valid token with base64 secret",
			token:       "mt1-1-1-dGVzdHNlY3JldA",
			wantPrefix:  "mt1",
			wantUserID:  1,
			wantTokenID: 1,
			wantSecret:  "dGVzdHNlY3JldA",
			wantErr:     false,
		},
		{
			name:        "valid token with large IDs",
			token:       "mt1-999999-888888-longsecretvalue123",
			wantPrefix:  "mt1",
			wantUserID:  999999,
			wantTokenID: 888888,
			wantSecret:  "longsecretvalue123",
			wantErr:     false,
		},
		{
			name:        "valid token with underscore in secret",
			token:       "mt1-1-2-secret_with_underscores",
			wantPrefix:  "mt1",
			wantUserID:  1,
			wantTokenID: 2,
			wantSecret:  "secret_with_underscores",
			wantErr:     false,
		},
		{
			name:        "secret starts with a dash",
			token:       "mt1-1-3--secret",
			wantPrefix:  "mt1",
			wantUserID:  1,
			wantTokenID: 3,
			wantSecret:  "-secret",
			wantErr:     false,
		},
		{
			name:          "empty token",
			token:         "",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "missing prefix",
			token:         "123-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "invalid prefix - wrong value",
			token:         "mt2-123-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token prefix",
		},
		{
			name:          "invalid prefix - ok1 instead of mt1",
			token:         "ok1-123-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token prefix",
		},
		{
			name:          "wrong prefix length",
			token:         "mt10-123-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "missing user ID",
			token:         "mt1--456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "missing token ID",
			token:         "mt1-123--secret",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "missing secret",
			token:         "mt1-123-456-",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "non-numeric user ID",
			token:         "mt1-abc-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "non-numeric token ID",
			token:         "mt1-123-xyz-secret",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "only prefix",
			token:         "mt1",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "only prefix with dash",
			token:         "mt1-",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "partial token - missing parts",
			token:         "mt1-123",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "partial token - two parts",
			token:         "mt1-123-456",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
		{
			name:          "negative user ID",
			token:         "mt1--1-456-secret",
			wantErr:       true,
			wantErrSubstr: "invalid MCP token format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, userID, tokenID, secret, err := ParseObotMCPToken(tt.token)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseObotMCPToken(%q) expected error containing %q, got nil", tt.token, tt.wantErrSubstr)
					return
				}
				if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("ParseObotMCPToken(%q) error = %q, want error containing %q", tt.token, err.Error(), tt.wantErrSubstr)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseObotMCPToken(%q) unexpected error: %v", tt.token, err)
				return
			}

			if prefix != tt.wantPrefix {
				t.Errorf("ParseObotMCPToken(%q) prefix = %q, want %q", tt.token, prefix, tt.wantPrefix)
			}
			if userID != tt.wantUserID {
				t.Errorf("ParseObotMCPToken(%q) userID = %d, want %d", tt.token, userID, tt.wantUserID)
			}
			if tokenID != tt.wantTokenID {
				t.Errorf("ParseObotMCPToken(%q) tokenID = %d, want %d", tt.token, tokenID, tt.wantTokenID)
			}
			if secret != tt.wantSecret {
				t.Errorf("ParseObotMCPToken(%q) secret = %q, want %q", tt.token, secret, tt.wantSecret)
			}
		})
	}
}
