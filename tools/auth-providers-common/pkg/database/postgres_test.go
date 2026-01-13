package database

import (
	"strings"
	"testing"
)

func TestValidatePostgresConnection_InvalidDSN(t *testing.T) {
	tests := []struct {
		name          string
		dsn           string
		errorContains string
	}{
		{
			name:          "empty dsn",
			dsn:           "",
			errorContains: "failed to ping database",
		},
		{
			name:          "invalid dsn format",
			dsn:           "not-a-valid-dsn",
			errorContains: "failed to ping database",
		},
		{
			name:          "unreachable host",
			dsn:           "postgres://user:pass@192.0.2.1:5432/db?sslmode=disable&connect_timeout=1",
			errorContains: "failed to ping database",
		},
		{
			name:          "invalid port",
			dsn:           "postgres://user:pass@localhost:99999/db",
			errorContains: "failed to ping database",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePostgresConnection(tt.dsn)

			if err == nil {
				t.Errorf("ValidatePostgresConnection() expected error for invalid DSN, got nil")
			}

			if !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("ValidatePostgresConnection() error = %v, want error containing %q", err, tt.errorContains)
			}
		})
	}
}

func TestGetSessionStorageHealth_InvalidDSN(t *testing.T) {
	tests := []struct {
		name          string
		dsn           string
		tablePrefix   string
		errorContains string
	}{
		{
			name:          "empty dsn",
			dsn:           "",
			tablePrefix:   "test_",
			errorContains: "failed to check session table",
		},
		{
			name:          "invalid dsn format",
			dsn:           "not-a-valid-dsn",
			tablePrefix:   "test_",
			errorContains: "failed to check session table",
		},
		{
			name:          "unreachable host",
			dsn:           "postgres://user:pass@192.0.2.1:5432/db?sslmode=disable&connect_timeout=1",
			tablePrefix:   "test_",
			errorContains: "failed to check session table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := GetSessionStorageHealth(tt.dsn, tt.tablePrefix)

			if err == nil {
				t.Errorf("GetSessionStorageHealth() expected error for invalid DSN, got nil")
			}

			if !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("GetSessionStorageHealth() error = %v, want error containing %q", err, tt.errorContains)
			}
		})
	}
}

// Note: Testing with a real PostgreSQL connection requires integration testing infrastructure.
// The tests above verify error handling for invalid connections.
// For testing successful connections, see docs/AUTHENTICATION_SECURITY_IMPLEMENTATION_GUIDE.md
// Integration Testing Requirements section.
