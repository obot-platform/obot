package logutil

import "strings"

// SanitizeDSN removes credentials from a database DSN for safe logging
func SanitizeDSN(dsn string) string {
	// Handle postgresql:// and postgres:// URLs
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		// Find the @ symbol that separates credentials from host
		atIndex := strings.Index(dsn, "@")
		if atIndex == -1 {
			// No credentials in DSN
			return dsn
		}
		
		// Find the :// part
		schemeEnd := strings.Index(dsn, "://")
		if schemeEnd == -1 {
			return dsn
		}
		
		// Extract scheme and host+path parts
		schemePrefix := dsn[:schemeEnd+3]
		hostAndPath := dsn[atIndex+1:]
		
		// Return sanitized version: scheme + [REDACTED] + @ + host+path
		return schemePrefix + "[REDACTED]@" + hostAndPath
	}
	
	// For SQLite or other DSNs, just return as-is (no credentials to hide)
	return dsn
}