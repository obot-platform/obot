package client

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/utils"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The OAuth grants of vMCP profiles are MCP OAuth tokens whose owner (UserID) is a principal.ProfileGrant and whose
// MCPID names a vMCP component. Many agents, on every replica, use one grant, so these methods differ from the
// per-user ones in three ways: a grant is written with an ID chosen beforehand and with who authorized it, a grant can
// be marked invalid for everyone at once, and refreshing one is serialized across replicas
// (WithMCPOAuthRefreshLock). None of them triggers an MCPServer: no MCPServer is named by a grant's MCPID.

const (
	// mcpOAuthRefreshHolders is how many shared grants a process refreshes at once, each holding one database
	// connection while it does. The default pool has five.
	mcpOAuthRefreshHolders = 2
)

var (
	// ErrNotProfileGrant refuses to write a grant for an owner that is not a vMCP profile.
	ErrNotProfileGrant = errors.New("owner is not a vMCP profile grant")
)

// MCPOAuthRefresh is a shared grant being refreshed while its refresh lock is held (WithMCPOAuthRefreshLock). It reads
// and saves the grant through the transaction that holds the lock, if there is one, so the refresh holds one database
// connection however long the provider takes.
type MCPOAuthRefresh struct {
	client       *Client
	db           *gorm.DB
	owner, mcpID string
}

// keyedLocks are process-local locks by key, created on first use and dropped when no one holds or waits for them.
// The zero value is ready to use.
type keyedLocks struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	slot  chan struct{}
	users int
}

// requireProfileGrantOwner returns ErrNotProfileGrant for an owner that is not a vMCP profile's grant.
func requireProfileGrantOwner(owner string) error {
	if _, _, ok := principal.ParseProfileGrant(owner); !ok {
		return fmt.Errorf("%w: %q", ErrNotProfileGrant, owner)
	}
	return nil
}

// ReplaceMCPOAuthGrant stores the grant that authorizedBy, an administrator's encoded principal, authorized for a vMCP
// profile's component, replacing any grant stored for it before, along with its state: a replaced grant that was
// invalid is valid again. grantID identifies the new grant; the caller chooses it, so that it can record the grant
// before writing it, and delete exactly it if the write must be undone (DeleteMCPOAuthGrants).
func (c *Client) ReplaceMCPOAuthGrant(ctx context.Context, owner, mcpID, url, grantID, authorizedBy string, oauthConf *oauth2.Config, token *oauth2.Token) error {
	if err := requireProfileGrantOwner(owner); err != nil {
		return err
	}
	if grantID == "" || authorizedBy == "" {
		return fmt.Errorf("a vMCP profile grant needs an ID and the administrator who authorized it")
	}
	now := time.Now()
	t := &types.MCPOAuthToken{
		GrantID:      grantID,
		UserID:       owner,
		MCPID:        mcpID,
		URL:          url,
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
		ExpiresIn:    token.ExpiresIn,
		ClientID:     oauthConf.ClientID,
		ClientSecret: oauthConf.ClientSecret,
		Endpoint:     oauthConf.Endpoint,
		RedirectURL:  oauthConf.RedirectURL,
		Scopes:       strings.Join(oauthConf.Scopes, " "),
		AuthorizedBy: authorizedBy,
		AuthorizedAt: &now,
	}
	if err := c.encryptMCPOAuthToken(ctx, t); err != nil {
		return fmt.Errorf("failed to encrypt token: %w", err)
	}
	// An administrator's authorization replaces the grant outright, scopes included: unlike a user's callback, it
	// cannot be a stale one, because its pending state is bound to the component definition it was started for.
	return c.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(t).Error
}

// MCPOAuthGrantMetadata returns what is stored about the tokens of owners for mcpIDs, without any token or client
// credential, with one query.
func (c *Client) MCPOAuthGrantMetadata(ctx context.Context, owners, mcpIDs []string) ([]types.MCPOAuthGrantMetadata, error) {
	if len(owners) == 0 || len(mcpIDs) == 0 {
		return nil, nil
	}
	var result []types.MCPOAuthGrantMetadata
	if err := c.db.WithContext(ctx).Model(&types.MCPOAuthToken{}).
		Select("MCPID", "UserID", "URL", "GrantID", "Scopes", "AuthorizedBy", "AuthorizedAt", "InvalidatedAt", "InvalidReason").
		Where("mcp_id IN ? AND user_id IN ?", mcpIDs, owners).
		Order("mcp_id").Order("user_id").
		Scan(&result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

// MCPOAuthGrantOwners returns the owners, among owners, that have any token stored, with one query.
func (c *Client) MCPOAuthGrantOwners(ctx context.Context, owners []string) ([]string, error) {
	if len(owners) == 0 {
		return nil, nil
	}
	var result []string
	if err := c.db.WithContext(ctx).Model(&types.MCPOAuthToken{}).
		Distinct("user_id").
		Where("user_id IN ?", owners).
		Order("user_id").
		Pluck("user_id", &result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteMCPOAuthGrants deletes the token of owner for mcpID if it is one of grantIDs, and nothing else: a grant
// written since the caller listed grantIDs has another ID and is kept. Deleting what is already gone succeeds.
func (c *Client) DeleteMCPOAuthGrants(ctx context.Context, owner, mcpID string, grantIDs []string) error {
	if len(grantIDs) == 0 {
		return nil
	}
	return c.db.WithContext(ctx).Delete(&types.MCPOAuthToken{}, "user_id = ? AND mcp_id = ? AND grant_id IN ?", owner, mcpID, grantIDs).Error
}

// DeleteMCPOAuthGrantsForOwners deletes every token of owners, with one statement. Only vMCP profile grant owners are
// accepted, so it can never delete the tokens of a user or an agent. Deleting what is already gone succeeds.
func (c *Client) DeleteMCPOAuthGrantsForOwners(ctx context.Context, owners []string) error {
	if len(owners) == 0 {
		return nil
	}
	for _, owner := range owners {
		if err := requireProfileGrantOwner(owner); err != nil {
			return err
		}
	}
	return c.db.WithContext(ctx).Delete(&types.MCPOAuthToken{}, "user_id IN ?", owners).Error
}

// MCPOAuthGrantDiscoveryRevision separates cached discovery by the grants keys name, as MCPOAuthDiscoveryRevision
// does by a user's own: removal, reauthorization, a scope change and invalidation all change it. Token values and
// expiry are excluded, and no secret is read.
func (c *Client) MCPOAuthGrantDiscoveryRevision(ctx context.Context, keys []types.MCPOAuthTokenKey) (string, error) {
	var owners, mcpIDs []string
	for _, key := range keys {
		owners = append(owners, key.UserID)
		mcpIDs = append(mcpIDs, key.MCPID)
	}
	grants, err := c.MCPOAuthGrantMetadata(ctx, owners, mcpIDs)
	if err != nil {
		return "", err
	}
	type revisionEntry struct {
		Key           types.MCPOAuthTokenKey
		URL           string
		GrantID       string
		Scopes        string
		InvalidatedAt *time.Time
	}
	var entries []revisionEntry
	for _, key := range keys {
		entry := revisionEntry{Key: key}
		for _, grant := range grants {
			if grant.UserID == key.UserID && grant.MCPID == key.MCPID {
				entry.URL, entry.GrantID, entry.Scopes, entry.InvalidatedAt = grant.URL, grant.GrantID, grant.Scopes, grant.InvalidatedAt
			}
		}
		entries = append(entries, entry)
	}
	return utils.Digest([]any{"mcp-oauth-grants/v1", entries}), nil
}

// CreateMCPOAuthGrantPendingState stores the pending authorization of a vMCP profile's grant, bound to the
// administrator who started it (initiatorUserID, their encoded principal) and to the definition of the component it
// is for (definitionHash). The callback refuses anyone else, and a component whose definition changed.
func (c *Client) CreateMCPOAuthGrantPendingState(ctx context.Context, owner, mcpID, mcpURL, tunnelName, initiatorUserID, definitionHash, state, verifier, resourceURL string, oauthConf *oauth2.Config) error {
	if err := requireProfileGrantOwner(owner); err != nil {
		return err
	}
	if initiatorUserID == "" || definitionHash == "" {
		return fmt.Errorf("a pending vMCP profile grant must be bound to its administrator and component definition")
	}
	ps := &types.MCPOAuthPendingState{
		HashedState:          fmt.Sprintf("%x", sha256.Sum256([]byte(state))),
		State:                state,
		Verifier:             verifier,
		UserID:               owner,
		MCPID:                mcpID,
		URL:                  mcpURL,
		TunnelName:           tunnelName,
		ResourceURL:          resourceURL,
		ClientID:             oauthConf.ClientID,
		ClientSecret:         oauthConf.ClientSecret,
		AuthURL:              oauthConf.Endpoint.AuthURL,
		TokenURL:             oauthConf.Endpoint.TokenURL,
		AuthStyle:            oauthConf.Endpoint.AuthStyle,
		RedirectURL:          oauthConf.RedirectURL,
		Scopes:               strings.Join(oauthConf.Scopes, " "),
		GrantInitiatorUserID: initiatorUserID,
		GrantDefinitionHash:  definitionHash,
	}
	if err := c.encryptMCPOAuthPendingState(ctx, ps); err != nil {
		return fmt.Errorf("failed to encrypt pending state: %w", err)
	}
	return c.db.WithContext(ctx).Create(ps).Error
}

// Token reads the grant as it is stored now, with its tokens.
func (r *MCPOAuthRefresh) Token(ctx context.Context, url string) (*types.MCPOAuthToken, error) {
	return r.client.getMCPOAuthToken(ctx, r.db, r.owner, r.mcpID, url)
}

// Save stores a refreshed token of the grant grantID, which must still be the one stored (RefreshMCPOAuthToken).
func (r *MCPOAuthRefresh) Save(ctx context.Context, url, grantID string, oauthConf *oauth2.Config, token *oauth2.Token) error {
	return r.client.saveMCPOAuthTokenIn(ctx, r.db, r.owner, r.mcpID, url, "", oauthConf, token, &grantID, "")
}

// Invalidate marks the grant grantID invalid, for every agent that uses it, with reason, which must never carry a
// token. A grant authorized since has another ID and is left alone. It reports whether it marked the grant: false when
// the grant is gone, replaced, or already invalid.
func (r *MCPOAuthRefresh) Invalidate(grantID, reason string) (bool, error) {
	result := r.db.Model(&types.MCPOAuthToken{}).
		Where("user_id = ? AND mcp_id = ? AND grant_id = ? AND invalidated_at IS NULL", r.owner, r.mcpID, grantID).
		Updates(map[string]any{"invalidated_at": time.Now(), "invalid_reason": reason})
	if result.Error != nil || result.RowsAffected == 0 {
		return false, result.Error
	}
	// Nobody asked for this: the provider refused the grant. It is recorded as the system's change, with the change
	// itself when r.db is a transaction, so that every agent that stopped working because of it can be explained.
	if vmcpID, profileID, ok := principal.ParseProfileGrant(r.owner); ok {
		componentID, _ := strings.CutPrefix(r.mcpID, vmcpID+"/")
		// A savepoint inside a transaction, so that a failed write cannot abort the transaction that holds the lock.
		if err := r.db.Transaction(func(tx *gorm.DB) error {
			return recordAdminAuditEventTx(tx, AdminAuditEventInput{
				Actor:        types.AdminAuditActorSystem,
				Action:       types.AdminAuditActionVMCPProfileOAuthInvalidate,
				ResourceKind: types.AdminAuditResourceVMCPProfile,
				ResourceID:   profileID,
				ParentID:     vmcpID,
				Details:      map[string]any{"componentID": componentID, "grantID": grantID, "reason": reason},
			})
		}); err != nil {
			// The grant is invalid whether or not this is recorded, and the caller must go on to refuse it.
			slog.Error("Failed to record the invalidation of a vMCP profile OAuth grant", "vmcp", vmcpID, "profile", profileID, "component", componentID, "error", err)
		}
	}
	return true, nil
}

// WithMCPOAuthRefreshLock runs fn while holding the refresh lock of the token of owner for mcpID, across every
// replica, so that only one of the many callers that share a grant refreshes it at a time. A caller that waited reads
// the token again in fn, finds the one the holder stored, and uses it instead of refreshing a refresh token that the
// provider has already rotated.
//
// Within the process, callers of one token wait on a lock of their own, and at most mcpOAuthRefreshHolders of them
// across tokens go on to the database, holding no connection while they wait, so refreshes never take the whole
// connection pool. On Postgres, a caller then takes a transaction-scoped advisory lock keyed on the token, and fn reads
// and saves through that transaction (MCPOAuthRefresh): what it saves is committed, and visible to the next holder,
// when the lock is released. SQLite runs in a single process, so the process lock is all it needs, and fn uses no
// transaction, which would hold its only connection. fn must be short: it is bounded by the provider's token endpoint.
func (c *Client) WithMCPOAuthRefreshLock(ctx context.Context, owner, mcpID string, fn func(context.Context, *MCPOAuthRefresh) error) error {
	key := owner + "\x00" + mcpID
	unlock, err := c.mcpOAuthRefreshLocks.lock(ctx, key)
	if err != nil {
		return err
	}
	defer unlock()

	db := c.db.WithContext(ctx)
	if db.Name() != "postgres" {
		return fn(ctx, &MCPOAuthRefresh{client: c, db: db, owner: owner, mcpID: mcpID})
	}

	select {
	case c.mcpOAuthRefreshHolders() <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.mcpOAuthRefreshHolders() }()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", mcpOAuthRefreshLockID(key)).Error; err != nil {
			return fmt.Errorf("failed to lock MCP OAuth token refresh: %w", err)
		}
		return fn(ctx, &MCPOAuthRefresh{client: c, db: tx, owner: owner, mcpID: mcpID})
	})
}

// mcpOAuthRefreshHolders admits the callers of WithMCPOAuthRefreshLock that may hold a database connection at once.
func (c *Client) mcpOAuthRefreshHolders() chan struct{} {
	c.mcpOAuthRefreshHoldersOnce.Do(func() {
		c.mcpOAuthRefreshHoldersSlot = make(chan struct{}, mcpOAuthRefreshHolders)
	})
	return c.mcpOAuthRefreshHoldersSlot
}

// mcpOAuthRefreshLockID is the advisory lock of one token's refreshes. A collision with another lock only makes two
// unrelated refreshes take turns.
func mcpOAuthRefreshLockID(key string) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte("mcp-oauth-refresh\x00" + key))
	return int64(hash.Sum64())
}

// lock waits for the lock of key, or for ctx to end, and returns the function that releases it.
func (k *keyedLocks) lock(ctx context.Context, key string) (func(), error) {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = map[string]*keyedLock{}
	}
	l := k.locks[key]
	if l == nil {
		l = &keyedLock{slot: make(chan struct{}, 1)}
		k.locks[key] = l
	}
	l.users++
	k.mu.Unlock()

	select {
	case l.slot <- struct{}{}:
		return func() {
			<-l.slot
			k.release(key, l)
		}, nil
	case <-ctx.Done():
		k.release(key, l)
		return nil, ctx.Err()
	}
}

func (k *keyedLocks) release(key string, l *keyedLock) {
	k.mu.Lock()
	defer k.mu.Unlock()
	l.users--
	if l.users == 0 {
		delete(k.locks, key)
	}
}
