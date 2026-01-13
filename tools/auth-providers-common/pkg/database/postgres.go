package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// ValidatePostgresConnection tests PostgreSQL connectivity and basic health.
// Returns an error if the connection cannot be established or if the database is not reachable.
func ValidatePostgresConnection(dsn string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	return nil
}

// GetSessionStorageHealth checks session storage health by verifying the session table exists.
// This is useful for periodic health checks to ensure the session storage is operational.
func GetSessionStorageHealth(dsn, tablePrefix string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	// Check if session table exists
	tableName := tablePrefix + "session_state"
	query := `SELECT EXISTS (
		SELECT FROM information_schema.tables
		WHERE table_name = $1
	)`

	var exists bool
	err = db.QueryRowContext(ctx, query, tableName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check session table: %w", err)
	}

	if !exists {
		return fmt.Errorf("session table '%s' does not exist", tableName)
	}

	return nil
}
