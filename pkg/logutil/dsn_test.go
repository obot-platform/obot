package logutil

import (
	"testing"
)

func TestSanitizeDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "postgres with credentials",
			dsn:  "postgres://user:pass@localhost:5432/mydb",
			want: "postgres://[REDACTED]@localhost:5432/mydb",
		},
		{
			name: "postgresql scheme with credentials",
			dsn:  "postgresql://admin:secret123@db.example.com:5432/database?sslmode=require",
			want: "postgresql://[REDACTED]@db.example.com:5432/database?sslmode=require",
		},
		{
			name: "postgres with special chars in password (no @)",
			dsn:  "postgres://user:p!ssw0rd#$%@host:5432/db",
			want: "postgres://[REDACTED]@host:5432/db",
		},
		{
			name: "postgres with username only (no password)",
			dsn:  "postgres://username@localhost:5432/mydb",
			want: "postgres://[REDACTED]@localhost:5432/mydb",
		},
		{
			name: "postgres without credentials",
			dsn:  "postgres:///mydb",
			want: "postgres:///mydb",
		},
		{
			name: "postgres without @ symbol",
			dsn:  "postgres://localhost:5432/mydb",
			want: "postgres://localhost:5432/mydb",
		},
		{
			name: "postgres with URL-encoded password",
			dsn:  "postgres://user:pass%40word@localhost:5432/db",
			want: "postgres://[REDACTED]@localhost:5432/db",
		},
		{
			name: "postgres with IPv4 address",
			dsn:  "postgres://user:pass@127.0.0.1:5432/db",
			want: "postgres://[REDACTED]@127.0.0.1:5432/db",
		},
		{
			name: "postgres with IPv6 address",
			dsn:  "postgres://user:pass@[::1]:5432/db",
			want: "postgres://[REDACTED]@[::1]:5432/db",
		},
		{
			name: "postgres with multiple query parameters",
			dsn:  "postgres://user:pass@host:5432/db?sslmode=require&connect_timeout=10",
			want: "postgres://[REDACTED]@host:5432/db?sslmode=require&connect_timeout=10",
		},
		{
			name: "sqlite with file path",
			dsn:  "file:./test.db",
			want: "file:./test.db",
		},
		{
			name: "sqlite with absolute path",
			dsn:  "file:/absolute/path/to/db.sqlite",
			want: "file:/absolute/path/to/db.sqlite",
		},
		{
			name: "sqlite memory database",
			dsn:  ":memory:",
			want: ":memory:",
		},
		{
			name: "empty string",
			dsn:  "",
			want: "",
		},
		{
			name: "malformed postgres URL without scheme separator",
			dsn:  "postgresuser:pass@host:5432/db",
			want: "postgresuser:pass@host:5432/db",
		},
		{
			name: "postgres with empty password",
			dsn:  "postgres://user:@host:5432/db",
			want: "postgres://[REDACTED]@host:5432/db",
		},
		{
			name: "postgres with special chars in username",
			dsn:  "postgres://admin%40company:pass@host:5432/db",
			want: "postgres://[REDACTED]@host:5432/db",
		},
		{
			name: "postgres with colon in password",
			dsn:  "postgres://user:pass:word@host:5432/db",
			want: "postgres://[REDACTED]@host:5432/db",
		},
		{
			name: "postgres with no database name",
			dsn:  "postgres://user:pass@host:5432",
			want: "postgres://[REDACTED]@host:5432",
		},
		{
			name: "mysql URL (not sanitized)",
			dsn:  "mysql://user:pass@host:3306/db",
			want: "mysql://user:pass@host:3306/db",
		},
		{
			name: "random string",
			dsn:  "not-a-dsn-string",
			want: "not-a-dsn-string",
		},
		{
			name: "postgres with long password",
			dsn:  "postgres://user:verylongpasswordwithlotsofcharacters123456789@host:5432/db",
			want: "postgres://[REDACTED]@host:5432/db",
		},
		{
			name: "postgresql with @ in password (limitation: uses first @)",
			dsn:  "postgres://user:p@ss@word@host:5432/db",
			want: "postgres://[REDACTED]@ss@word@host:5432/db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeDSN(tt.dsn)
			if got != tt.want {
				t.Errorf("SanitizeDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeDSNDoesNotLeakCredentials(t *testing.T) {
	// Test cases where we want to make absolutely sure no credentials leak
	sensitiveTests := []struct {
		name     string
		dsn      string
		mustNot  string // string that must NOT appear in result
	}{
		{
			name:    "password should not appear",
			dsn:     "postgres://user:secretpassword@host/db",
			mustNot: "secretpassword",
		},
		{
			name:    "username should not appear with colon",
			dsn:     "postgres://adminuser:mypass@host/db",
			mustNot: "adminuser:",
		},
		{
			name:    "complex password should not appear",
			dsn:     "postgres://user:Passw0rd!#$@host/db",
			mustNot: "Passw0rd!#$",
		},
		{
			name:    "URL-encoded credentials should not appear",
			dsn:     "postgres://user:pass%40word@host/db",
			mustNot: "pass%40word",
		},
	}

	for _, tt := range sensitiveTests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeDSN(tt.dsn)
			if got == tt.dsn {
				// If DSN wasn't sanitized, fail the test
				t.Errorf("SanitizeDSN() returned original DSN unchanged, credentials may have leaked")
			}
			if tt.mustNot != "" && contains(got, tt.mustNot) {
				t.Errorf("SanitizeDSN() result contains sensitive string %q: %q", tt.mustNot, got)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) &&
		(s == substr || len(s) > 0 && (s[:len(substr)] == substr ||
			contains(s[1:], substr)))
}
