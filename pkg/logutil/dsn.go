package logutil

import "strings"

// SanitizeDSN removes credentials from a database DSN for safe logging
func SanitizeDSN(dsn string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		// Find the @ symbol that separates credentials from host
		_, hostAndPath, ok := strings.Cut(dsn, "@")
		if !ok {
			return dsn
		}

		schemeEnd := strings.Index(dsn, "://")
		if schemeEnd == -1 {
			return dsn
		}

		// Extract scheme and host+path parts
		schemePrefix := dsn[:schemeEnd+3]
		// Return sanitized version: scheme + [REDACTED] + @ + host+path
		return schemePrefix + "[REDACTED]@" + hostAndPath
	}

	// For SQLite, just return as-is
	return dsn
}
