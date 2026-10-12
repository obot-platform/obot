package client

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// scimModeLockID serializes changes to an auth provider's SCIM mode with the writes that depend on it, such as
	// just-in-time user creation and auth provider cleanup. Those writers take it shared and check the mode inside
	// their transaction, so a mode change either commits before they read it or waits for them.
	scimModeLockID int64 = 0x6f626f7453434d4d // "obotSCMM"

	// scimWriteLockID serializes SCIM writes. There is at most one SCIM connection, so one lock covers every SCIM
	// resource.
	scimWriteLockID int64 = 0x6f626f7453434d57 // "obotSCMW"

	scimTokenPrefix = "obot_scim_"
	scimTokenBytes  = 32

	// scimPreviousTokenLifetime is how long a rotated-out token is still accepted, so that the identity provider can
	// be switched to the new token without failed requests.
	scimPreviousTokenLifetime = 24 * time.Hour
)

var (
	// ErrSCIMConnectionNotFound reports that no SCIM connection has the requested ID, or that none exists.
	ErrSCIMConnectionNotFound = errors.New("SCIM connection not found")
	// ErrMultipleSCIMConnections reports that more than one SCIM connection exists, which only one may. No SCIM request
	// is served then, because none can tell which connection it is for.
	ErrMultipleSCIMConnections = errors.New("more than one SCIM connection exists")
	// ErrSCIMManagedGroupData reports an attempt to delete the group data of an auth provider whose group ID prefix the
	// SCIM connection of another auth provider manages.
	ErrSCIMManagedGroupData = errors.New("the group data is managed by a SCIM connection")
	// ErrSCIMConnectionHasToken reports an attempt to issue the first bearer token of a connection that has one.
	ErrSCIMConnectionHasToken = errors.New("the SCIM connection already has a token")
	// ErrSCIMGroupDeletionInProgress reports an attempt to enforce SCIM while another deletion of unreferenced groups
	// holds marks for deletion.
	ErrSCIMGroupDeletionInProgress = errors.New("another deletion of unreferenced groups is in progress")
)

// SCIMConnectionExistsError reports an attempt to create a second SCIM connection.
type SCIMConnectionExistsError struct {
	ConnectionID string
}

// SCIMAuthenticationError reports a bearer token that does not authenticate the SCIM connection it was presented to,
// including any token presented to a connection that has none.
type SCIMAuthenticationError struct{}

// CreateSCIMConnectionOptions describes the auth provider a new SCIM connection manages.
type CreateSCIMConnectionOptions struct {
	AuthProviderNamespace string
	AuthProviderName      string
	GroupIDPrefix         string
	Issuer                string
	Origin                types.SCIMConnectionOrigin
	// IssueToken issues the connection's first bearer token. Without it, the connection has no token, and every
	// request to it is refused until one is issued.
	IssueToken bool
	// RequireNoGroupData refuses to create the connection with *SCIMResidualGroupDataError while the auth provider
	// has any group data in the gateway database: groups, or memberships or group role assignments of group IDs
	// with its group ID prefix. A connection created without directory credentials must start without groups,
	// because nothing but SCIM could ever correct them.
	RequireNoGroupData bool
}

func (e *SCIMConnectionExistsError) Error() string {
	return fmt.Sprintf("SCIM connection %s already exists; only one SCIM connection is supported", e.ConnectionID)
}

func (*SCIMAuthenticationError) Error() string {
	return "invalid SCIM bearer token"
}

// CreateSCIMConnection creates the SCIM connection of an auth provider in the connected state. From this moment SCIM
// replaces login-time directory synchronization for the provider, and there is no way back. The adapter is chosen by
// the auth provider's name, and recorded on the connection.
//
// When opts.IssueToken is set, it also returns the connection's bearer token, which is not stored and cannot be
// retrieved again.
func (c *Client) CreateSCIMConnection(ctx context.Context, opts CreateSCIMConnectionOptions) (*types.SCIMConnection, string, error) {
	if opts.AuthProviderNamespace == "" || opts.AuthProviderName == "" {
		return nil, "", errors.New("auth provider namespace and name are required")
	}
	a, ok := adapter.ForAuthProvider(opts.AuthProviderName)
	if !ok {
		return nil, "", fmt.Errorf("auth provider %s/%s does not support SCIM", opts.AuthProviderNamespace, opts.AuthProviderName)
	}
	if opts.GroupIDPrefix == "" {
		return nil, "", fmt.Errorf("auth provider %s/%s declares no group ID prefix", opts.AuthProviderNamespace, opts.AuthProviderName)
	}
	switch opts.Origin {
	case types.SCIMConnectionOriginSCIMFirst, types.SCIMConnectionOriginMigrated:
	default:
		return nil, "", fmt.Errorf("invalid SCIM connection origin %q", opts.Origin)
	}

	now := time.Now()
	conn := &types.SCIMConnection{
		ID:                    uuid.NewV4().String(),
		AdapterType:           a.Type(),
		Origin:                opts.Origin,
		AuthProviderNamespace: opts.AuthProviderNamespace,
		AuthProviderName:      opts.AuthProviderName,
		GroupIDPrefix:         opts.GroupIDPrefix,
		Issuer:                opts.Issuer,
		State:                 types.SCIMConnectionStateConnected,
		EnabledAt:             now,
	}

	var token string
	if opts.IssueToken {
		var (
			verifier string
			err      error
		)
		if token, verifier, err = newSCIMToken(); err != nil {
			return nil, "", err
		}
		conn.TokenVerifier = verifier
		conn.TokenIssuedAt = &now
	}

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSCIMMode(tx, true); err != nil {
			return err
		}
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}

		var existing []types.SCIMConnection
		if err := tx.Select("id").Limit(1).Find(&existing).Error; err != nil {
			return fmt.Errorf("failed to check for existing SCIM connections: %w", err)
		}
		if len(existing) > 0 {
			return &SCIMConnectionExistsError{
				ConnectionID: existing[0].ID,
			}
		}

		if opts.RequireNoGroupData {
			data, err := authProviderGroupDataTx(tx, opts.AuthProviderNamespace, opts.AuthProviderName, opts.GroupIDPrefix)
			if err != nil {
				return err
			}
			if !data.Empty() {
				return &SCIMResidualGroupDataError{
					AuthProviderName: opts.AuthProviderName,
					Data:             *data,
				}
			}
		}

		if err := tx.Create(conn).Error; err != nil {
			return fmt.Errorf("failed to create SCIM connection: %w", err)
		}
		return nil
	}); err != nil {
		return nil, "", err
	}

	return conn, token, nil
}

// SCIMConnections returns every SCIM connection: zero or one.
func (c *Client) SCIMConnections(ctx context.Context) ([]types.SCIMConnection, error) {
	var conns []types.SCIMConnection
	if err := c.db.WithContext(ctx).Order("created_at, id").Find(&conns).Error; err != nil {
		return nil, fmt.Errorf("failed to list SCIM connections: %w", err)
	}
	return conns, nil
}

// SCIMConnection returns the SCIM connection with the given ID, or ErrSCIMConnectionNotFound.
func (c *Client) SCIMConnection(ctx context.Context, id string) (*types.SCIMConnection, error) {
	return scimConnectionTx(c.db.WithContext(ctx), id, false)
}

// SCIMConnectionForAuthProvider returns the SCIM connection of the auth provider, or nil when the provider has none and
// still synchronizes its directory at sign-in. Callers must treat an error as unknown, and never fall back to the
// directory.
func (c *Client) SCIMConnectionForAuthProvider(ctx context.Context, namespace, name string) (*types.SCIMConnection, error) {
	return scimConnectionForAuthProviderTx(c.db.WithContext(ctx), namespace, name)
}

// AuthenticateSCIMConnection returns the SCIM connection if token is its current bearer token or its previous one,
// while that token is still accepted. There is at most one connection, which serves every SCIM request. It returns
// ErrSCIMConnectionNotFound when there is none, ErrMultipleSCIMConnections when there is more than one, and
// *SCIMAuthenticationError for any other token, including an expired one, every token presented to a connection that
// has none, and one issued for an earlier connection.
func (c *Client) AuthenticateSCIMConnection(ctx context.Context, token string) (*types.SCIMConnection, error) {
	var conns []types.SCIMConnection
	if err := c.db.WithContext(ctx).Limit(2).Find(&conns).Error; err != nil {
		return nil, fmt.Errorf("failed to get the SCIM connection: %w", err)
	}
	if len(conns) == 0 {
		return nil, ErrSCIMConnectionNotFound
	}
	if len(conns) > 1 {
		return nil, ErrMultipleSCIMConnections
	}
	conn := &conns[0]

	verifier := []byte(hash.String(token))
	current := subtle.ConstantTimeCompare(verifier, []byte(conn.TokenVerifier)) == 1
	previous := subtle.ConstantTimeCompare(verifier, []byte(conn.PreviousTokenVerifier)) == 1
	now := time.Now()
	accepted := (current && conn.TokenAccepted(now)) || (previous && conn.PreviousTokenAccepted(now))
	if token == "" || !conn.HasToken() || !accepted {
		return nil, new(SCIMAuthenticationError)
	}

	return conn, nil
}

// IssueFirstSCIMConnectionToken issues the bearer token of a connection that has none, and returns the connection
// with it. It returns ErrSCIMConnectionHasToken when the connection has a token already, so that of several callers
// only one is given a first token.
func (c *Client) IssueFirstSCIMConnectionToken(ctx context.Context, id string) (*types.SCIMConnection, string, error) {
	token, verifier, err := newSCIMToken()
	if err != nil {
		return nil, "", err
	}

	var conn *types.SCIMConnection
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if conn, err = scimConnectionTx(tx, id, true); err != nil {
			return err
		}
		if conn.HasToken() {
			return ErrSCIMConnectionHasToken
		}

		now := time.Now()
		if err := tx.Model(conn).UpdateColumns(map[string]any{
			"token_verifier":  verifier,
			"token_issued_at": now,
			"updated_at":      now,
		}).Error; err != nil {
			return fmt.Errorf("failed to issue the first token of SCIM connection %s: %w", id, err)
		}
		conn.TokenVerifier = verifier
		conn.TokenIssuedAt = &now
		conn.UpdatedAt = now
		return nil
	}); err != nil {
		return nil, "", err
	}

	return conn, token, nil
}

// RotateSCIMConnectionToken issues a new bearer token for the connection and returns the updated connection with it.
// This also issues the first token of a connection that has none. The token it replaces is still accepted for a day,
// or until it expires, RevokePreviousSCIMConnectionToken is called, or the token is rotated again, so the identity
// provider can be switched over without failed requests.
func (c *Client) RotateSCIMConnectionToken(ctx context.Context, id string) (*types.SCIMConnection, string, error) {
	return c.replaceSCIMConnectionToken(ctx, id, true)
}

// RevokeCurrentSCIMConnectionToken replaces a leaked bearer token: in one step, it issues a new token, which it
// returns with the updated connection, and stops accepting both the current token and the previous one.
func (c *Client) RevokeCurrentSCIMConnectionToken(ctx context.Context, id string) (*types.SCIMConnection, string, error) {
	return c.replaceSCIMConnectionToken(ctx, id, false)
}

// RevokePreviousSCIMConnectionToken stops accepting the token that the last rotation replaced.
func (c *Client) RevokePreviousSCIMConnectionToken(ctx context.Context, id string) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		conn, err := scimConnectionTx(tx, id, true)
		if err != nil {
			return err
		}

		if err := tx.Model(conn).UpdateColumns(map[string]any{
			"previous_token_verifier":   "",
			"previous_token_expires_at": nil,
			"updated_at":                time.Now(),
		}).Error; err != nil {
			return fmt.Errorf("failed to revoke the previous token of SCIM connection %s: %w", id, err)
		}
		return nil
	})
}

// replaceSCIMConnectionToken issues a new current token for the connection, and returns the updated connection with
// it. When keepPrevious is set, the token it replaces becomes the previous token, which is accepted for a limited
// time. Otherwise it stops working at once, and so does any previous token.
func (c *Client) replaceSCIMConnectionToken(ctx context.Context, id string, keepPrevious bool) (*types.SCIMConnection, string, error) {
	token, verifier, err := newSCIMToken()
	if err != nil {
		return nil, "", err
	}

	var conn *types.SCIMConnection
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if conn, err = scimConnectionTx(tx, id, true); err != nil {
			return err
		}

		var (
			now                    = time.Now()
			previousTokenVerifier  string
			previousTokenExpiresAt *time.Time
		)
		if keepPrevious && conn.HasToken() {
			// The token it replaces is accepted for a little longer, but never past its own expiry, so an expired
			// token is not accepted again.
			expiresAt := now.Add(scimPreviousTokenLifetime)
			if tokenExpiresAt := conn.TokenExpiresAt(); tokenExpiresAt != nil && tokenExpiresAt.Before(expiresAt) {
				expiresAt = *tokenExpiresAt
			}
			if expiresAt.After(now) {
				previousTokenVerifier = conn.TokenVerifier
				previousTokenExpiresAt = &expiresAt
			}
		}

		if err := tx.Model(conn).UpdateColumns(map[string]any{
			"token_verifier":            verifier,
			"token_issued_at":           now,
			"previous_token_verifier":   previousTokenVerifier,
			"previous_token_expires_at": previousTokenExpiresAt,
			"updated_at":                now,
		}).Error; err != nil {
			return fmt.Errorf("failed to replace the token of SCIM connection %s: %w", id, err)
		}
		conn.TokenVerifier = verifier
		conn.TokenIssuedAt = &now
		conn.PreviousTokenVerifier = previousTokenVerifier
		conn.PreviousTokenExpiresAt = previousTokenExpiresAt
		conn.UpdatedAt = now
		return nil
	}); err != nil {
		return nil, "", err
	}

	return conn, token, nil
}

// scimConnectionTx returns the SCIM connection with the given ID, locking it when forUpdate is set, or
// ErrSCIMConnectionNotFound.
func scimConnectionTx(tx *gorm.DB, id string, forUpdate bool) (*types.SCIMConnection, error) {
	query := tx
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}

	var conns []types.SCIMConnection
	if err := query.Where("id = ?", id).Limit(1).Find(&conns).Error; err != nil {
		return nil, fmt.Errorf("failed to get SCIM connection %s: %w", id, err)
	}
	if len(conns) == 0 {
		return nil, ErrSCIMConnectionNotFound
	}
	return &conns[0], nil
}

func scimConnectionForAuthProviderTx(tx *gorm.DB, namespace, name string) (*types.SCIMConnection, error) {
	if namespace == "" || name == "" {
		return nil, nil
	}

	var conns []types.SCIMConnection
	if err := tx.Where("auth_provider_namespace = ? AND auth_provider_name = ?", namespace, name).Limit(1).Find(&conns).Error; err != nil {
		return nil, fmt.Errorf("failed to check the SCIM connection of auth provider %s/%s: %w", namespace, name, err)
	}
	if len(conns) == 0 {
		return nil, nil
	}
	return &conns[0], nil
}

// scimConnectionForGroupDataTx returns the SCIM connection that owns an auth provider's group data, or nil when none
// does. A connection owns the groups of its own auth provider and every group ID with its group ID prefix, so it also
// owns the data that another provider with the same prefix would clean up.
func scimConnectionForGroupDataTx(tx *gorm.DB, namespace, name, groupIDPrefix string) (*types.SCIMConnection, error) {
	var conns []types.SCIMConnection
	if err := tx.Where("(auth_provider_namespace = ? AND auth_provider_name = ?) OR (group_id_prefix <> '' AND group_id_prefix = ?)", namespace, name, groupIDPrefix).
		Limit(1).Find(&conns).Error; err != nil {
		return nil, fmt.Errorf("failed to check the SCIM connection for the group data of auth provider %s/%s: %w", namespace, name, err)
	}
	if len(conns) == 0 {
		return nil, nil
	}
	return &conns[0], nil
}

// scimConnectionForAuthProviderLockedTx is scimConnectionForAuthProviderTx for a transaction that is about to write
// data whose correctness depends on the answer. It takes the mode lock shared first, so that the answer holds until
// the transaction ends.
func scimConnectionForAuthProviderLockedTx(tx *gorm.DB, namespace, name string) (*types.SCIMConnection, error) {
	if namespace == "" || name == "" {
		return nil, nil
	}
	if err := lockSCIMMode(tx, false); err != nil {
		return nil, err
	}
	return scimConnectionForAuthProviderTx(tx, namespace, name)
}

// lockSCIMMode takes the SCIM mode lock for the rest of the transaction. SQLite needs no lock, because its single
// connection already serializes transactions.
func lockSCIMMode(tx *gorm.DB, exclusive bool) error {
	if tx.Name() != "postgres" {
		return nil
	}

	lock := "pg_advisory_xact_lock_shared"
	if exclusive {
		lock = "pg_advisory_xact_lock"
	}
	if err := tx.Exec("SELECT "+lock+"(?)", scimModeLockID).Error; err != nil {
		return fmt.Errorf("failed to lock SCIM mode: %w", err)
	}
	return nil
}

// lockSCIMWrites serializes SCIM writes for the rest of the transaction.
func lockSCIMWrites(tx *gorm.DB) error {
	if tx.Name() != "postgres" {
		return nil
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", scimWriteLockID).Error; err != nil {
		return fmt.Errorf("failed to lock SCIM writes: %w", err)
	}
	return nil
}

// lockSCIMConnectionWrites serializes SCIM writes for the rest of the transaction, then reads the connection again
// under that lock, or returns ErrSCIMConnectionNotFound. Callers hold a connection read before the lock, which
// deconfiguring or unstaging its auth provider may have deleted meanwhile under the same lock. A binding or mark
// written for a deleted connection would leave its user or group managed by a connection that no request can reach.
func lockSCIMConnectionWrites(tx *gorm.DB, id string) (*types.SCIMConnection, error) {
	if err := lockSCIMWrites(tx); err != nil {
		return nil, err
	}
	return scimConnectionTx(tx, id, false)
}

// newSCIMToken returns a new bearer token and its verifier, which is all that is stored.
func newSCIMToken() (string, string, error) {
	b := make([]byte, scimTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("failed to generate SCIM token: %w", err)
	}

	token := scimTokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return token, hash.String(token), nil
}
