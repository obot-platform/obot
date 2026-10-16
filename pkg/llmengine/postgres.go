package llmengine

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

type credentialKeyStore struct {
	ctx    context.Context
	client *gatewayclient.Client
}

// DerivePostgresDSN returns a DSN for a database named after Obot's own, on the same Postgres
// server, creating the database if it doesn't exist. Bifrost's tables can't share Obot's
// database, but sharing the server keeps every Obot replica on the same gateway state
// without extra configuration. Connection failures are retried for up to a minute; a database
// that can't be created fails at once, with an error that says how to create it.
func DerivePostgresDSN(ctx context.Context, obotDSN string) (string, error) {
	deadline := time.Now().Add(time.Minute)
	for delay := time.Second; ; delay = min(delay*2, 10*time.Second) {
		dsn, err := derivePostgresDSN(ctx, obotDSN)
		var pgErr *pgconn.PgError
		if err == nil || errors.As(err, &pgErr) || time.Now().Add(delay).After(deadline) {
			return dsn, err
		}
		select {
		case <-ctx.Done():
			return "", err
		case <-time.After(delay):
		}
	}
}

func derivePostgresDSN(ctx context.Context, obotDSN string) (string, error) {
	u, err := url.Parse(strings.Replace(obotDSN, "postgresql://", "postgres://", 1))
	if err != nil {
		return "", fmt.Errorf("invalid Postgres DSN: %w", err)
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		dbName = "obot"
	}
	gatewayDB := dbName + "_llm_gateway"

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, u.String())
	if err != nil {
		return "", fmt.Errorf("failed to connect to Postgres: %w", err)
	}
	defer conn.Close(context.Background())

	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", gatewayDB).Scan(&exists); err != nil {
		return "", fmt.Errorf("failed to look up database %q: %w", gatewayDB, err)
	}
	if !exists {
		if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{gatewayDB}.Sanitize()); err != nil {
			// Another replica may have created it at the same time. Postgres reports that as
			// duplicate_database, or as a unique violation on pg_database when the two creates
			// race, so look again rather than trusting the error code.
			if !databaseExists(ctx, conn, gatewayDB) {
				return "", fmt.Errorf("failed to create the LLM gateway's database %q: %w. Create it yourself, owned by Obot's database user (CREATE DATABASE %s OWNER %s), or set --llm-gateway-postgres-dsn to a database for the gateway",
					gatewayDB, err, pgx.Identifier{gatewayDB}.Sanitize(), pgx.Identifier{u.User.Username()}.Sanitize())
			}
		}
	}

	u.Path = "/" + gatewayDB
	return u.String(), nil
}

// CredentialKeyStore keeps the shared encryption key in Obot's credential store, which is
// encrypted with Obot's own encryption configuration when one is set.
func CredentialKeyStore(ctx context.Context, c *gatewayclient.Client) SharedKeyStore {
	return credentialKeyStore{ctx: ctx, client: c}
}

func (s credentialKeyStore) EnsureEncryptionKey(key string) (string, error) {
	cred, err := s.client.EnsureCredential(s.ctx, gatewaytypes.Credential{
		Context: "llm-gateway",
		Name:    "encryption-key",
		Secrets: map[string]string{"key": key},
	})
	if err != nil {
		return "", err
	}
	if cred.Secrets["key"] == "" {
		return "", errors.New("the stored LLM gateway encryption key is empty")
	}
	return cred.Secrets["key"], nil
}

// databaseExists reports whether a database exists, treating a failed lookup as not existing.
func databaseExists(ctx context.Context, conn *pgx.Conn, name string) bool {
	var exists bool
	return conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists) == nil && exists
}
