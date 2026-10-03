package client

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const (
	// postgresUniqueViolation is the SQLSTATE code of a unique constraint violation.
	postgresUniqueViolation = "23505"
	// sqliteUniqueViolation begins the message of a unique or primary key constraint violation in SQLite, which,
	// unlike PostgreSQL's, is never translated.
	sqliteUniqueViolation = "UNIQUE constraint failed"
)

// IsUniqueViolation reports whether err is a unique constraint violation from PostgreSQL or SQLite. Neither driver
// translates them to gorm.ErrDuplicatedKey unless gorm is configured to. PostgreSQL's messages depend on the
// database's language, so its error code is compared instead.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == postgresUniqueViolation
	}
	return strings.Contains(err.Error(), sqliteUniqueViolation)
}
