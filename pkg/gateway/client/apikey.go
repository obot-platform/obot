package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	apiKeySecretLength       = 32 // 32 bytes = 256 bits of entropy
	apiKeyValidationCacheTTL = 15 * time.Second
	apiKeyCacheCleanupPeriod = 5 * time.Minute
	expirationDur            = 7 * 24 * time.Hour
)

var (
	// ErrInvalidAPIKey reports that a presented API key is not a valid credential: it is malformed, unknown, wrong,
	// expired, or revoked. ValidateAPIKey returns other errors when it could not read the key or its owner.
	ErrInvalidAPIKey = errors.New("invalid API key")

	// ErrInvalidAgentAPIKeyRequest reports that CreateAgentAPIKey was asked for a key it must not create, such as one
	// scoped to an unknown vMCP.
	ErrInvalidAgentAPIKeyRequest = errors.New("invalid agent API key request")

	// apiKeyOwnerKinds describe how the keys of each kind of owner are written and looked up.
	apiKeyOwnerKinds = map[principal.Kind]apiKeyOwnerKind{
		principal.KindUser: {
			prefix: system.APIKeyPrefix,
			column: "api_keys.user_id",
			join:   "LEFT JOIN users ON users.id = api_keys.user_id",
			ownerColumns: []string{
				"users.id AS owner_id",
				"users.deleted_at AS owner_deleted_at",
				"users.disabled_at AS owner_disabled_at",
			},
		},
		principal.KindAgent: {
			prefix: system.AgentAPIKeyPrefix,
			column: "api_keys.agent_id",
			// A raw join is not limited to undeleted agents, so a deleted agent's row is read with its deletion
			// time, and the agent is reported as deleted rather than as missing.
			join: "LEFT JOIN agents ON agents.id = api_keys.agent_id",
			ownerColumns: []string{
				"agents.id AS owner_id",
				"agents.deleted_at AS owner_deleted_at",
				"agents.disabled_at AS owner_disabled_at",
			},
		},
	}
)

// apiKeyOwnerKind describes the API keys of one kind of owner.
type apiKeyOwnerKind struct {
	// prefix starts the keys of the kind's owners.
	prefix string
	// column is the api_keys column that holds the owner's ID. A key has exactly one owner, so the keys of the
	// kind's owners are exactly those with this column set.
	column string
	// join joins an API key to its owner, whose lifecycle state ownerColumns select as an APIKeyOwnerLifecycle.
	join         string
	ownerColumns []string
}

type apiKeyValidationCacheEntry struct {
	apiKey    types.APIKey
	expiresAt time.Time
	keyID     uint
}

// APIKeyListOptions controls whether retained revoked keys are included.
type APIKeyListOptions struct {
	ShowRevoked bool
	// OwnerKind limits ListAllAPIKeys to the keys of users or of agents. Empty lists the keys of both.
	OwnerKind principal.Kind
}

// apiKeyOwner returns the kind and ID of the owner of API keys. It reports false for a principal that is neither a
// user nor an agent, which owns no API key.
func apiKeyOwner(owner principal.Principal) (apiKeyOwnerKind, uint, bool) {
	if id, ok := owner.UserID(); ok {
		return apiKeyOwnerKinds[principal.KindUser], id, true
	}
	if id, ok := owner.AgentID(); ok {
		return apiKeyOwnerKinds[principal.KindAgent], id, true
	}
	return apiKeyOwnerKind{}, 0, false
}

// whereAPIKeyOwner restricts db to the API keys of owner. A principal that is neither a user nor an agent matches
// no key.
func whereAPIKeyOwner(db *gorm.DB, owner principal.Principal) *gorm.DB {
	kind, id, ok := apiKeyOwner(owner)
	if !ok {
		return db.Where("1 = 0")
	}
	return db.Where(kind.column+" = ?", id)
}

// cloneAPIKey creates a deep copy of the provided APIKey, so that it's safe to return without corrupting the cache
func cloneAPIKey(apiKey types.APIKey) types.APIKey {
	cloned := apiKey
	// The owner is copied too: a caller writing through a shared owner pointer would change whose key the cached
	// record is.
	if apiKey.UserID != nil {
		cloned.UserID = new(*apiKey.UserID)
	}
	if apiKey.AgentID != nil {
		cloned.AgentID = new(*apiKey.AgentID)
	}
	if apiKey.MCPServerIDs != nil {
		cloned.MCPServerIDs = slices.Clone(apiKey.MCPServerIDs)
	}
	if apiKey.LastUsedAt != nil {
		cloned.LastUsedAt = new(*apiKey.LastUsedAt)
	}
	if apiKey.ExpiresAt != nil {
		cloned.ExpiresAt = new(*apiKey.ExpiresAt)
	}
	if apiKey.RevokedAt != nil {
		cloned.RevokedAt = new(*apiKey.RevokedAt)
	}
	return cloned
}

func apiKeyCacheFingerprint(key string) [32]byte {
	return sha256.Sum256([]byte(key))
}

func (c *Client) getValidatedAPIKeyFromCache(key string, now time.Time) (*types.APIKey, bool) {
	if c.apiKeyCacheTTL <= 0 {
		return nil, false
	}

	fingerprint := apiKeyCacheFingerprint(key)

	c.apiKeyCacheLock.RLock()
	entry, ok := c.apiKeyCache[fingerprint]
	if !ok {
		c.apiKeyCacheLock.RUnlock()
		return nil, false
	}

	// Fast path: entry appears valid under the read lock.
	entryExpired := entry.apiKey.RevokedAt != nil || now.After(entry.expiresAt) || (entry.apiKey.ExpiresAt != nil && entry.apiKey.ExpiresAt.Before(now))
	if !entryExpired {
		cachedAPIKey := entry.apiKey
		c.apiKeyCacheLock.RUnlock()
		apiKey := cloneAPIKey(cachedAPIKey)
		return &apiKey, true
	}

	// Slow path: entry appears expired; re-check under write lock before deleting
	c.apiKeyCacheLock.RUnlock()

	c.apiKeyCacheLock.Lock()
	entry, ok = c.apiKeyCache[fingerprint]
	if !ok {
		c.apiKeyCacheLock.Unlock()
		return nil, false
	}

	if entry.apiKey.RevokedAt != nil || now.After(entry.expiresAt) || (entry.apiKey.ExpiresAt != nil && entry.apiKey.ExpiresAt.Before(now)) {
		delete(c.apiKeyCache, fingerprint)
		c.apiKeyCacheLock.Unlock()
		return nil, false
	}

	cachedAPIKey := entry.apiKey
	c.apiKeyCacheLock.Unlock()
	apiKey := cloneAPIKey(cachedAPIKey)
	return &apiKey, true
}

func (c *Client) putValidatedAPIKeyInCache(key string, apiKey *types.APIKey, now time.Time) {
	if c.apiKeyCacheTTL <= 0 || apiKey == nil {
		return
	}

	c.apiKeyCacheLock.Lock()
	c.apiKeyCache[apiKeyCacheFingerprint(key)] = apiKeyValidationCacheEntry{
		apiKey:    cloneAPIKey(*apiKey),
		expiresAt: now.Add(c.apiKeyCacheTTL),
		keyID:     apiKey.ID,
	}
	c.apiKeyCacheLock.Unlock()
}

func (c *Client) pruneExpiredValidatedAPIKeys(now time.Time) {
	c.apiKeyCacheLock.Lock()
	defer c.apiKeyCacheLock.Unlock()

	for fingerprint, entry := range c.apiKeyCache {
		if entry.apiKey.RevokedAt != nil || now.After(entry.expiresAt) || (entry.apiKey.ExpiresAt != nil && entry.apiKey.ExpiresAt.Before(now)) {
			delete(c.apiKeyCache, fingerprint)
		}
	}
}

func (c *Client) runAPIKeyCacheCleanup(ctx context.Context) {
	ticker := time.NewTicker(apiKeyCacheCleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			c.pruneExpiredValidatedAPIKeys(now)
			c.pruneExpiredValidatedServiceAccountAPIKeys(now)
			c.pruneExpiredAgentDelegationValidations(now)
		}
	}
}

func (c *Client) invalidateValidatedAPIKeysByID(keyID uint) {
	c.apiKeyCacheLock.Lock()
	defer c.apiKeyCacheLock.Unlock()

	for fingerprint, entry := range c.apiKeyCache {
		if entry.keyID == keyID {
			delete(c.apiKeyCache, fingerprint)
		}
	}
}

// CreateAPIKey generates a new API key for the given user.
// Returns the full key only once in the response.
func (c *Client) CreateAPIKey(ctx context.Context, userID uint, name, description string, expiresAt *time.Time, scopes types.APIKeyScopes) (*types.APIKeyCreateResponse, error) {
	var resp *types.APIKeyCreateResponse
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error

		resp, err = c.createAPIKey(tx, types.APIKey{UserID: &userID, Name: name, Description: description, ExpiresAt: expiresAt, APIKeyScopes: scopes})
		return err
	}); err != nil {
		return nil, err
	}

	return resp, nil
}

// CreateAPIKeyFromSetupTokenRequest creates an API key from a token request.
func (c *Client) CreateAPIKeyFromSetupTokenRequest(ctx context.Context, userID uint, tr *types.TokenRequest) (*types.APIKeyCreateResponse, error) {
	if tr.Purpose != types.TokenRequestPurposeSetup {
		return nil, fmt.Errorf("token request %q is not a setup request", tr.ID)
	}

	var resp *types.APIKeyCreateResponse
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		resp, err = c.createAPIKeyFromTokenRequest(tx, userID, tr)
		return err
	}); err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *Client) createAPIKeyFromTokenRequest(tx *gorm.DB, userID uint, tr *types.TokenRequest) (*types.APIKeyCreateResponse, error) {
	var expiresAt *time.Time
	if !tr.NoExpiration {
		expiresAt = new(time.Now().Add(expirationDur))
	}

	resp, err := c.createAPIKey(tx, types.APIKey{UserID: &userID, Name: tr.Name, Description: tr.Description, ExpiresAt: expiresAt, APIKeyScopes: tr.Scopes})
	if err != nil {
		return nil, err
	}

	if resp.ExpiresAt != nil {
		tr.ExpiresAt = *resp.ExpiresAt
	} else {
		tr.ExpiresAt = time.Time{}
	}
	tr.Token = resp.Key

	if err := tx.Model(tr).Updates(map[string]any{
		"expires_at": tr.ExpiresAt,
		"token":      tr.Token,
	}).Error; err != nil {
		return nil, err
	}

	return resp, nil
}

// ListAPIKeys returns active API keys for a user (without the secrets).
func (c *Client) ListAPIKeys(ctx context.Context, userID uint) ([]types.APIKey, error) {
	return c.ListAPIKeysWithOptions(ctx, userID, APIKeyListOptions{})
}

// ListAPIKeysWithOptions returns API keys for a user according to opts.
func (c *Client) ListAPIKeysWithOptions(ctx context.Context, userID uint, opts APIKeyListOptions) ([]types.APIKey, error) {
	return c.listAPIKeys(ctx, principal.User(userID), opts)
}

// GetAPIKey retrieves a single API key of a user by ID.
func (c *Client) GetAPIKey(ctx context.Context, userID uint, keyID uint) (*types.APIKey, error) {
	return c.getAPIKey(ctx, principal.User(userID), keyID)
}

// RevokeAPIKey immediately disables an API key while retaining its metadata.
// Repeated revocation is idempotent and preserves the original revocation time.
func (c *Client) RevokeAPIKey(ctx context.Context, userID uint, keyID uint) error {
	return c.revokeAPIKey(ctx, keyID, new(principal.User(userID)))
}

// CreateAgentAPIKey generates a new API key for an agent, with the restrictions of req. It returns the full key,
// which is only shown this once. A missing or deleted agent is gorm.ErrRecordNotFound. A disabled agent gets a key,
// which works once the agent is enabled again.
//
// The vMCPs req names must exist, but need not be granted to the agent: what the key reaches is computed whenever it
// is used, so a vMCP the agent is not granted is only reached once a grant appears. A request the key cannot be
// created for is ErrInvalidAgentAPIKeyRequest.
func (c *Client) CreateAgentAPIKey(ctx context.Context, agentID uint, req types.AgentAPIKeyCreateRequest) (*types.APIKeyCreateResponse, error) {
	scopes := req.Scopes()
	if !scopes.HasSomeScope() {
		return nil, fmt.Errorf("%w: the key must allow a vMCP, the LLM proxy, or Skills", ErrInvalidAgentAPIKeyRequest)
	}
	if err := c.checkAgentAPIKeyVMCPs(ctx, scopes.MCPServerIDs); err != nil {
		return nil, err
	}

	var resp *types.APIKeyCreateResponse
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		resp, err = c.createAPIKey(tx, types.APIKey{AgentID: &agentID, Name: req.Name, Description: req.Description, ExpiresAt: req.ExpiresAt, APIKeyScopes: scopes})
		if err != nil {
			return err
		}
		// The secret is in resp.Key, which the event never sees.
		details := map[string]any{
			"name":              resp.Name,
			"canAccessSkills":   scopes.CanAccessSkills,
			"canAccessLLMProxy": scopes.CanAccessLLMProxy,
			"mcpServerIDs":      scopes.MCPServerIDs,
		}
		if resp.ExpiresAt != nil {
			details["expiresAt"] = resp.ExpiresAt.UTC()
		}
		return recordContextAdminAuditEventTx(ctx, tx, AdminAuditEventInput{
			Action:       types.AdminAuditActionAgentKeyCreate,
			ResourceKind: types.AdminAuditResourceAPIKey,
			ResourceID:   strconv.FormatUint(uint64(resp.ID), 10),
			ParentID:     strconv.FormatUint(uint64(agentID), 10),
			Details:      details,
		})
	}); err != nil {
		return nil, err
	}
	return resp, nil
}

// checkAgentAPIKeyVMCPs returns an ErrInvalidAgentAPIKeyRequest unless each of ids is "*" or an existing vMCP. Agents
// only connect through vMCPs, so an agent's key cannot name any other MCP server.
func (c *Client) checkAgentAPIKeyVMCPs(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if id == "*" {
			continue
		}
		if !system.IsVMCPID(id) {
			return fmt.Errorf("%w: %q is not a vMCP", ErrInvalidAgentAPIKeyRequest, id)
		}
		if err := c.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: id}, new(v1.VMCP)); apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: vMCP %q not found", ErrInvalidAgentAPIKeyRequest, id)
		} else if err != nil {
			return fmt.Errorf("failed to check vMCP %q: %w", id, err)
		}
	}
	return nil
}

// ListAgentAPIKeys returns the API keys of an agent (without the secrets) according to opts.
func (c *Client) ListAgentAPIKeys(ctx context.Context, agentID uint, opts APIKeyListOptions) ([]types.APIKey, error) {
	return c.listAPIKeys(ctx, principal.Agent(agentID), opts)
}

// GetAgentAPIKey retrieves a single API key of an agent by ID. A key of another owner is gorm.ErrRecordNotFound.
func (c *Client) GetAgentAPIKey(ctx context.Context, agentID uint, keyID uint) (*types.APIKey, error) {
	return c.getAPIKey(ctx, principal.Agent(agentID), keyID)
}

// RevokeAgentAPIKey immediately disables an API key of an agent while retaining its metadata. Repeated revocation is
// idempotent and preserves the original revocation time.
func (c *Client) RevokeAgentAPIKey(ctx context.Context, agentID uint, keyID uint) error {
	return c.revokeAPIKey(ctx, keyID, new(principal.Agent(agentID)))
}

func (c *Client) listAPIKeys(ctx context.Context, owner principal.Principal, opts APIKeyListOptions) ([]types.APIKey, error) {
	var keys []types.APIKey
	db := whereAPIKeyOwner(c.db.WithContext(ctx), owner)
	if !opts.ShowRevoked {
		db = db.Where("revoked_at IS NULL")
	}
	if err := db.Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	return keys, nil
}

func (c *Client) getAPIKey(ctx context.Context, owner principal.Principal, keyID uint) (*types.APIKey, error) {
	var key types.APIKey
	if err := whereAPIKeyOwner(c.db.WithContext(ctx).Where("id = ?", keyID), owner).First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// ValidateAPIKey validates an API key and returns the associated APIKey record, with the current lifecycle status of
// its owner. A disabled or deleted owner does not make the key invalid: the caller must deny such an owner access.
// The key format is ok1-<user_id>-<key_id>-<secret> for a user's key, and oak1-<agent_id>-<key_id>-<secret> for an
// agent's key. Lookup is done by key ID and by the owner the prefix and owner ID name, so a key is never read as the
// key of another owner, even one with the same numeric ID. Then bcrypt is used to verify the secret.
// Cache hits avoid repeating bcrypt but still check persisted lifecycle state so
// revocation on one server replica takes effect on every replica immediately.
// On cache misses, last_used_at is updated only if more than a minute has elapsed.
func (c *Client) ValidateAPIKey(ctx context.Context, key string) (*types.APIKey, error) {
	cacheNow := time.Now()
	if cachedAPIKey, ok := c.getValidatedAPIKeyFromCache(key, cacheNow); ok {
		// The cache is keyed by the whole key, so the cached record was read for the owner this key names.
		ownerKind, ownerID, ok := apiKeyOwner(cachedAPIKey.Owner())
		if !ok {
			c.invalidateValidatedAPIKeysByID(cachedAPIKey.ID)
			return nil, fmt.Errorf("%w: API key has no owner", ErrInvalidAPIKey)
		}
		var lifecycle struct {
			ExpiresAt *time.Time
			RevokedAt *time.Time
			types.APIKeyOwnerLifecycle
		}
		if err := c.db.WithContext(ctx).Model(&types.APIKey{}).
			Select(append([]string{"api_keys.expires_at", "api_keys.revoked_at"}, ownerKind.ownerColumns...)).
			Joins(ownerKind.join).
			Where("api_keys.id = ?", cachedAPIKey.ID).
			Where(ownerKind.column+" = ?", ownerID).
			Take(&lifecycle).Error; err != nil {
			c.invalidateValidatedAPIKeysByID(cachedAPIKey.ID)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("%w: %w", ErrInvalidAPIKey, err)
			}
			return nil, err
		}
		if lifecycle.RevokedAt != nil {
			c.invalidateValidatedAPIKeysByID(cachedAPIKey.ID)
			return nil, fmt.Errorf("%w: API key has been revoked", ErrInvalidAPIKey)
		}
		if lifecycle.ExpiresAt != nil && lifecycle.ExpiresAt.Before(cacheNow) {
			c.invalidateValidatedAPIKeysByID(cachedAPIKey.ID)
			return nil, fmt.Errorf("%w: API key has expired", ErrInvalidAPIKey)
		}
		cachedAPIKey.OwnerStatus = lifecycle.Status()
		return cachedAPIKey, nil
	}

	// Parse the key to extract components
	kind, ownerID, keyID, secret, err := ParseAPIKey(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidAPIKey, err)
	}
	ownerKind := apiKeyOwnerKinds[kind]

	var apiKey types.APIKey
	err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Look up by key ID and owner, with the lifecycle state of the key's owner.
		var row struct {
			types.APIKey
			types.APIKeyOwnerLifecycle
		}
		if err := tx.Model(&types.APIKey{}).
			Select(append([]string{"api_keys.*"}, ownerKind.ownerColumns...)).
			Joins(ownerKind.join).
			Where("api_keys.id = ?", keyID).
			Where(ownerKind.column+" = ?", ownerID).
			Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: %w", ErrInvalidAPIKey, err)
		} else if err != nil {
			return err
		}
		apiKey = row.APIKey
		apiKey.OwnerStatus = row.Status()
		// The database refuses a key with two owners. Should one exist anyway, it is not a key of either.
		if apiKey.Owner().Kind != kind {
			return fmt.Errorf("%w: API key does not have exactly one owner", ErrInvalidAPIKey)
		}

		// Verify the secret using bcrypt
		if err := bcrypt.CompareHashAndPassword([]byte(apiKey.HashedSecret), []byte(secret)); err != nil {
			return ErrInvalidAPIKey
		}

		// Check expiration
		if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
			return fmt.Errorf("%w: API key has expired", ErrInvalidAPIKey)
		}
		if apiKey.RevokedAt != nil {
			return fmt.Errorf("%w: API key has been revoked", ErrInvalidAPIKey)
		}

		// Update last used timestamp if more than a minute has elapsed
		lastUsedAtNow := time.Now()
		if apiKey.LastUsedAt == nil || lastUsedAtNow.Sub(*apiKey.LastUsedAt) > time.Minute {
			apiKey.LastUsedAt = &lastUsedAtNow
			return tx.Model(&apiKey).Update("last_used_at", lastUsedAtNow).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	c.putValidatedAPIKeyInCache(key, &apiKey, cacheNow)
	return &apiKey, nil
}

// ParseAPIKey parses an API key string and extracts its components: the kind of the key's owner, which the prefix
// names, the owner's ID, the key's ID, and its secret. It returns an error if the format or the prefix is invalid.
func ParseAPIKey(key string) (ownerKind principal.Kind, ownerID uint, keyID uint, secret string, err error) {
	parts := strings.SplitN(key, "-", 4)
	if len(parts) != 4 {
		return "", 0, 0, "", fmt.Errorf("invalid API key format")
	}
	ownerKind, ok := apiKeyPrefixOwnerKind(parts[0])
	if !ok {
		return "", 0, 0, "", fmt.Errorf("invalid API key prefix")
	}
	parsedOwnerID, err := strconv.ParseUint(parts[1], 10, 0)
	if err != nil {
		return "", 0, 0, "", fmt.Errorf("invalid API key format")
	}
	parsedKeyID, err := strconv.ParseUint(parts[2], 10, 0)
	if err != nil || parts[3] == "" {
		return "", 0, 0, "", fmt.Errorf("invalid API key format")
	}
	return ownerKind, uint(parsedOwnerID), uint(parsedKeyID), parts[3], nil
}

// IsAgentAPIKey reports whether value has the prefix of an agent's API key. It does not check that the rest of value is
// well-formed, so that a malformed agent key is still recognized as one.
func IsAgentAPIKey(value string) bool {
	return strings.HasPrefix(value, system.AgentAPIKeyPrefix+"-")
}

// ParseRedactedAPIKey extracts the owner and key IDs from the non-secret API
// key prefix emitted by nanobot audit logs. Nanobot retains the first three
// hyphen-delimited components or 12 characters, whichever is longer, so short
// numeric components can be followed by a small fragment of the redacted
// secret. Full bearer tokens are deliberately rejected.
func ParseRedactedAPIKey(key string) (ownerKind principal.Kind, ownerID uint, keyID uint, err error) {
	parts := strings.SplitN(key, "-", 4)
	if len(parts) != 4 {
		return "", 0, 0, fmt.Errorf("invalid redacted API key format")
	}
	ownerKind, ok := apiKeyPrefixOwnerKind(parts[0])
	if !ok {
		return "", 0, 0, fmt.Errorf("invalid API key prefix")
	}
	prefixLength := len(parts[0]) + len(parts[1]) + len(parts[2]) + 3
	if len(key) != max(prefixLength, 12) {
		return "", 0, 0, fmt.Errorf("invalid redacted API key format")
	}

	parsedOwnerID, err := strconv.ParseUint(parts[1], 10, 0)
	if err != nil || parsedOwnerID == 0 {
		return "", 0, 0, fmt.Errorf("invalid redacted API key format")
	}
	parsedKeyID, err := strconv.ParseUint(parts[2], 10, 0)
	if err != nil || parsedKeyID == 0 {
		return "", 0, 0, fmt.Errorf("invalid redacted API key format")
	}

	return ownerKind, uint(parsedOwnerID), uint(parsedKeyID), nil
}

// apiKeyPrefixOwnerKind returns the kind of owner whose API keys start with prefix.
func apiKeyPrefixOwnerKind(prefix string) (principal.Kind, bool) {
	for kind, ownerKind := range apiKeyOwnerKinds {
		if ownerKind.prefix == prefix {
			return kind, true
		}
	}
	return "", false
}

// Admin methods - no owner filtering

// ListAllAPIKeys returns the API keys of every user and agent according to opts. Each key's Owner tells whose it is.
func (c *Client) ListAllAPIKeys(ctx context.Context, opts APIKeyListOptions) ([]types.APIKey, error) {
	var keys []types.APIKey
	db := c.db.WithContext(ctx)
	if !opts.ShowRevoked {
		db = db.Where("revoked_at IS NULL")
	}
	if opts.OwnerKind != "" {
		ownerKind, ok := apiKeyOwnerKinds[opts.OwnerKind]
		if !ok {
			return nil, fmt.Errorf("unknown API key owner kind %q", opts.OwnerKind)
		}
		// A key has exactly one owner, so the keys of a kind's owners are those with its owner column set.
		db = db.Where(ownerKind.column + " IS NOT NULL")
	}
	if err := db.Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	return keys, nil
}

// GetAPIKeyByID retrieves an API key by ID without owner filtering (for admin use). It may be a user's or an agent's.
func (c *Client) GetAPIKeyByID(ctx context.Context, keyID uint) (*types.APIKey, error) {
	var key types.APIKey
	if err := c.db.WithContext(ctx).Where("id = ?", keyID).First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// RevokeAPIKeyByID immediately disables an API key without owner filtering
// while retaining its metadata for audit history.
func (c *Client) RevokeAPIKeyByID(ctx context.Context, keyID uint) error {
	return c.revokeAPIKey(ctx, keyID, nil)
}

// revokeAPIKey revokes the API key with keyID, if owner owns it. A nil owner revokes the key whoever owns it. Revoking
// a key of an agent as an administrator (WithAdminActor) records the revocation in the same transaction. Revoking a
// key that is revoked, or missing, succeeds and records nothing.
func (c *Client) revokeAPIKey(ctx context.Context, keyID uint, owner *principal.Principal) error {
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		db := tx.Model(&types.APIKey{}).Where("id = ?", keyID)
		if owner != nil {
			db = whereAPIKeyOwner(db, *owner)
		}
		var key types.APIKey
		if err := db.Select("id", "agent_id", "name").Take(&key).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return fmt.Errorf("failed to revoke API key: %w", err)
		}

		result := tx.Model(&types.APIKey{}).Where("id = ? AND revoked_at IS NULL", keyID).Update("revoked_at", time.Now().UTC())
		if result.Error != nil {
			return fmt.Errorf("failed to revoke API key: %w", result.Error)
		}
		if result.RowsAffected == 0 || key.AgentID == nil {
			return nil
		}
		return recordContextAdminAuditEventTx(ctx, tx, AdminAuditEventInput{
			Action:       types.AdminAuditActionAgentKeyRevoke,
			ResourceKind: types.AdminAuditResourceAPIKey,
			ResourceID:   strconv.FormatUint(uint64(keyID), 10),
			ParentID:     strconv.FormatUint(uint64(*key.AgentID), 10),
			Details:      map[string]any{"name": key.Name},
		})
	}); err != nil {
		return err
	}
	c.invalidateValidatedAPIKeysByID(keyID)
	return nil
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp for an API key
// if more than a minute has elapsed since the previous timestamp.
func (c *Client) UpdateAPIKeyLastUsed(ctx context.Context, key *types.APIKey) error {
	now := time.Now()
	if key.LastUsedAt != nil && now.Sub(*key.LastUsedAt) <= time.Minute {
		return nil
	}

	result := c.db.WithContext(ctx).Model(&types.APIKey{}).Where("id = ?", key.ID).Update("last_used_at", now)
	if result.Error != nil {
		return fmt.Errorf("failed to update API key last used time: %w", result.Error)
	}
	return nil
}

// createAPIKey inserts key with a new secret. The caller supplies ownership, metadata, scopes, and expiration.
func (c *Client) createAPIKey(tx *gorm.DB, key types.APIKey) (*types.APIKeyCreateResponse, error) {
	// Every key is created here, including by paths that finish without the user present, such as device login
	// polling and hosted agent reconciliation. Checking in the same transaction means none of them can issue a key
	// to a user who is disabled or deleted, or to an agent that is deleted.
	owner := key.Owner()
	ownerKind, ownerID, ok := apiKeyOwner(owner)
	if !ok {
		return nil, errors.New("an API key must have exactly one owner, a user or an agent")
	}
	switch owner.Kind {
	case principal.KindUser:
		if err := checkCredentialOwner(tx, ownerID); err != nil {
			return nil, err
		}
	case principal.KindAgent:
		// Unlike a user, a missing agent is refused too. A disabled agent gets the key, which works once the agent
		// is enabled. The lock makes a concurrent deletion wait for the key, which it then revokes, or the key wait
		// for the deletion, which leaves no agent to issue it to.
		if _, err := lockAgent(tx, ownerID); err != nil {
			return nil, fmt.Errorf("failed to find agent %d: %w", ownerID, err)
		}
	}

	// Generate cryptographically secure random secret
	secretBytes := make([]byte, apiKeySecretLength)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)

	// Hash the secret with bcrypt for storage
	hashedSecret, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash secret: %w", err)
	}

	// Create the API key record
	apiKey := &key
	apiKey.HashedSecret = string(hashedSecret)
	apiKey.CreatedAt = time.Now()

	if err := tx.Create(apiKey).Error; err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	// Construct the full key with the auto-generated ID
	fullKey := fmt.Sprintf("%s-%d-%d-%s", ownerKind.prefix, ownerID, apiKey.ID, secret)

	return &types.APIKeyCreateResponse{
		APIKey: *apiKey,
		Key:    fullKey,
	}, nil
}
