package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// groupCheckPeriod defines how often the system checks for updates to group information from the auth provider.
	groupCheckPeriod = time.Minute * 10

	// DefaultGroupPageSize is the page size used when a caller does not ask for one.
	DefaultGroupPageSize = 50

	// MaxGroupPageSize matches the ceiling the auth providers enforce, which is set by the
	// smallest page any of the identity providers will serve in a single request.
	MaxGroupPageSize = 100

	// groupProviderTimeout bounds a single call to the auth provider. The API server sets no
	// request deadline, so without this an unresponsive provider holds the handler until the
	// client gives up.
	groupProviderTimeout = 30 * time.Second

	groupCursorVersion = 1
)

var (
	groupProviderClient = &http.Client{Timeout: groupProviderTimeout}
)

// FetchUserGroupsError represents an error that occurs when fetching user groups from the auth provider.
// This error indicates a configuration issue with the auth provider that requires administrator intervention.
type FetchUserGroupsError struct {
	ProviderUserID string
	Message        string
}

type ListAuthGroupsOptions struct {
	NameFilter string
	Limit      int
	Cursor     string
}

type ListAuthGroupsResult struct {
	Groups     []types.Group
	NextCursor string

	// Source is where the items came from: types.GroupSourceProvider or types.GroupSourceCache.
	Source types.GroupSource

	// Degraded is true when the auth provider could not be listed and the response fell back to
	// cached groups.
	Degraded bool

	// Reset is true when the supplied cursor could not be honored and these groups are the first
	// page of a fresh listing rather than the page that was asked for.
	Reset bool
}

type listAuthGroupsPage struct {
	Items      []auth.GroupInfo `json:"items"`
	NextCursor string           `json:"nextCursor"`
}

// groupCursor is the opaque position handed to callers. It wraps the auth provider's own cursor
// rather than exposing it, because the cached fallback pages differently and a position from one
// source is meaningless to the other. Recording which source minted it lets a caller that switches
// sources mid-listing restart cleanly instead of replaying an uninterpretable token.
//
// The JSON names are single letters because the encoded cursor travels in a query string and the
// auth provider cursor it wraps can already be long.
type groupCursor struct {
	Version           int               `json:"v"`
	Source            types.GroupSource `json:"s"`
	FilterFingerprint string            `json:"f,omitempty"`

	// ProviderCursor is the auth provider's cursor, carried verbatim.
	ProviderCursor string `json:"p,omitempty"`

	// LastName and LastID are the keyset position of the last row of the previous page, used when
	// paging the cached listing out of the database.
	LastName string `json:"n,omitempty"`
	LastID   string `json:"i,omitempty"`
}

func (e *FetchUserGroupsError) Error() string {
	return fmt.Sprintf("auth provider failed to check groups for user with ID %s: %s", e.ProviderUserID, e.Message)
}

// encodeGroupCursor renders a position as an opaque token. An empty position stays empty, which is
// what tells a caller it has reached the end.
func encodeGroupCursor(cursor groupCursor) (string, error) {
	if cursor.ProviderCursor == "" && cursor.LastName == "" && cursor.LastID == "" {
		return "", nil
	}

	cursor.Version = groupCursorVersion
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("failed to encode group cursor: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(payload), nil
}

// decodeGroupCursor reads a position back. A cursor that cannot be trusted is reported as absent
// rather than as an error: the caller then starts over at the first page, which is always a
// sensible answer and never surfaces a failure to someone who simply retyped a search.
func decodeGroupCursor(encoded, nameFilter string) (groupCursor, bool) {
	if encoded == "" {
		return groupCursor{}, false
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return groupCursor{}, false
	}

	var cursor groupCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return groupCursor{}, false
	}

	if cursor.Version != groupCursorVersion || cursor.FilterFingerprint != groupFilterFingerprint(nameFilter) {
		// Either the format moved on, or the caller changed the search between pages, which makes
		// the recorded position meaningless.
		return groupCursor{}, false
	}

	return cursor, true
}

// groupFilterFingerprint reduces a name filter to a short token, used to notice that a cursor is
// being replayed against a different search. Only equality matters, so a short non-cryptographic
// hash is enough and keeps the cursor small. It is not a privacy control.
func groupFilterFingerprint(nameFilter string) string {
	if nameFilter == "" {
		return ""
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(nameFilter))

	return strconv.FormatUint(h.Sum64(), 16)
}

// ListAuthGroups returns one page of the auth provider's groups.
//
// The auth provider is the authoritative source. When it cannot be listed, the response falls back
// to the groups table, which holds the groups observed during a user sign-in plus any resolved by
// ID for a policy, and is therefore partial.
func (c *Client) ListAuthGroups(ctx context.Context, authProviderURL, authProviderNamespace, authProviderName string, opts ListAuthGroupsOptions) (ListAuthGroupsResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = DefaultGroupPageSize
	}
	opts.Limit = min(opts.Limit, MaxGroupPageSize)

	cursor, _ := decodeGroupCursor(opts.Cursor, opts.NameFilter)

	if authProviderURL != "" {
		// A cursor minted by the cached listing cannot be replayed against the provider, so drop
		// it and restart from the provider's first page.
		providerCursor := ""
		if cursor.Source == types.GroupSourceProvider {
			providerCursor = cursor.ProviderCursor
		}

		result, status, err := c.listAuthGroupsFromProvider(ctx, authProviderURL, authProviderNamespace, authProviderName, opts, providerCursor)
		switch {
		case err == nil:
			return result, nil
		case status == http.StatusNotFound:
			// The provider does not implement group listing (e.g. github, google, local). The
			// cached groups are all there has ever been, so this is not a degraded response.
			slog.Debug("auth provider does not support group listing, using cached groups",
				"authProviderName", authProviderName, "authProviderNamespace", authProviderNamespace)
		case status == http.StatusBadRequest && providerCursor != "":
			slog.Debug("auth provider rejected the group listing cursor, restarting from the first page",
				"authProviderName", authProviderName, "authProviderNamespace", authProviderNamespace, "error", err)

			result, _, retryErr := c.listAuthGroupsFromProvider(ctx, authProviderURL, authProviderNamespace, authProviderName, opts, "")
			if retryErr == nil {
				return result, nil
			}

			slog.Warn("failed to list groups from auth provider, falling back to cached groups",
				"authProviderName", authProviderName, "authProviderNamespace", authProviderNamespace, "error", retryErr)

			return c.listAuthGroupsFromCache(ctx, authProviderNamespace, authProviderName, opts, cursor, true)
		default:
			// Fall back to cached groups so the UI keeps working when the provider is unreachable
			// or the credentials lack directory-wide read permission.
			slog.Warn("failed to list groups from auth provider, falling back to cached groups",
				"authProviderName", authProviderName, "authProviderNamespace", authProviderNamespace, "error", err)

			return c.listAuthGroupsFromCache(ctx, authProviderNamespace, authProviderName, opts, cursor, true)
		}
	}

	return c.listAuthGroupsFromCache(ctx, authProviderNamespace, authProviderName, opts, cursor, false)
}

// listAuthGroupsFromProvider asks the auth provider for a page of groups. It returns the provider's
// HTTP status alongside the error so the caller can tell "not implemented" and "bad cursor" apart
// from a failure.
func (c *Client) listAuthGroupsFromProvider(ctx context.Context, authProviderURL, authProviderNamespace, authProviderName string, opts ListAuthGroupsOptions, providerCursor string) (ListAuthGroupsResult, int, error) {
	u, err := url.Parse(authProviderURL + "/obot-list-auth-groups")
	if err != nil {
		return ListAuthGroupsResult{}, 0, fmt.Errorf("failed to parse auth provider URL for group search: %w", err)
	}

	q := u.Query()
	if opts.NameFilter != "" {
		q.Set("name", opts.NameFilter)
	}
	q.Set("limit", strconv.Itoa(opts.Limit))
	if providerCursor != "" {
		q.Set("cursor", providerCursor)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ListAuthGroupsResult{}, 0, fmt.Errorf("failed to build group listing request: %w", err)
	}

	resp, err := groupProviderClient.Do(req)
	if err != nil {
		return ListAuthGroupsResult{}, 0, fmt.Errorf("failed to call auth provider group listing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return ListAuthGroupsResult{}, resp.StatusCode, fmt.Errorf("auth provider group listing returned status %d: %s", resp.StatusCode, string(body))
	}

	var page listAuthGroupsPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return ListAuthGroupsResult{}, resp.StatusCode, fmt.Errorf("failed to decode auth provider group listing: %w", err)
	}

	groups := make([]types.Group, 0, len(page.Items))
	for _, info := range page.Items {
		if info.ID == "" {
			continue
		}
		groups = append(groups, types.Group{
			ID:                    info.ID,
			AuthProviderName:      authProviderName,
			AuthProviderNamespace: authProviderNamespace,
			Name:                  info.Name,
			IconURL:               info.IconURL,
		})
	}

	nextCursor, err := encodeGroupCursor(groupCursor{
		Source:            types.GroupSourceProvider,
		FilterFingerprint: groupFilterFingerprint(opts.NameFilter),
		ProviderCursor:    page.NextCursor,
	})
	if err != nil {
		return ListAuthGroupsResult{}, resp.StatusCode, err
	}

	return ListAuthGroupsResult{
		Groups:     groups,
		NextCursor: nextCursor,
		Source:     types.GroupSourceProvider,
		Reset:      opts.Cursor != "" && providerCursor == "",
	}, resp.StatusCode, nil
}

// listAuthGroupsFromCache pages over the groups table, which holds the groups seen during previous
// user sign-ins.
//
// It pages by keyset rather than by offset: there is no total to page against, and a keyset seek
// stays on the (name, id) ordering index no matter how deep the listing goes.
func (c *Client) listAuthGroupsFromCache(ctx context.Context, authProviderNamespace, authProviderName string, opts ListAuthGroupsOptions, cursor groupCursor, degraded bool) (ListAuthGroupsResult, error) {
	result := ListAuthGroupsResult{
		Groups:   []types.Group{},
		Source:   types.GroupSourceCache,
		Degraded: degraded,
	}

	if authProviderNamespace == "" || authProviderName == "" {
		return result, nil
	}

	query := c.db.WithContext(ctx).Model(&types.Group{}).
		Where("auth_provider_namespace = ? AND auth_provider_name = ?", authProviderNamespace, authProviderName)

	// Case-insensitive, compatible with SQLite and PostgreSQL.
	if opts.NameFilter != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+opts.NameFilter+"%")
	}

	// A cursor minted by the provider says nothing about a position in this table, so start over.
	usedCursor := cursor.Source == types.GroupSourceCache && (cursor.LastName != "" || cursor.LastID != "")
	if usedCursor {
		// Written out rather than as a row-value comparison, which SQLite and PostgreSQL do not
		// both support.
		query = query.Where("(name > ? OR (name = ? AND id > ?))", cursor.LastName, cursor.LastName, cursor.LastID)
	}

	result.Reset = opts.Cursor != "" && !usedCursor

	// One more than the page, so the presence of a further page is known without a second query.
	var groups []types.Group
	if err := query.Order("name, id").Limit(opts.Limit + 1).Find(&groups).Error; err != nil {
		return ListAuthGroupsResult{}, fmt.Errorf("failed to fetch groups from database: %w", err)
	}

	var next groupCursor
	if len(groups) > opts.Limit {
		groups = groups[:opts.Limit]
		last := groups[len(groups)-1]
		next = groupCursor{
			Source:            types.GroupSourceCache,
			FilterFingerprint: groupFilterFingerprint(opts.NameFilter),
			LastName:          last.Name,
			LastID:            last.ID,
		}
	}

	nextCursor, err := encodeGroupCursor(next)
	if err != nil {
		return ListAuthGroupsResult{}, err
	}

	if groups != nil {
		result.Groups = groups
	}
	result.NextCursor = nextCursor

	return result, nil
}

// ResolveAuthGroups looks up specific groups by ID, which is what the UI needs to render a name for
// a group that already has a role or policy attached.
//
// Note the shape: cache first, provider only for the remainder. Resolution runs on page loads, so
// making it an unconditional provider call would put an identity provider round trip, and its rate
// limit, in front of every admin screen.
func (c *Client) ResolveAuthGroups(ctx context.Context, authProviderURL, authProviderNamespace, authProviderName string, ids []string) ([]types.Group, error) {
	if len(ids) == 0 {
		return []types.Group{}, nil
	}

	var cached []types.Group
	query := c.db.WithContext(ctx).Where("id IN ?", ids)
	if authProviderNamespace != "" && authProviderName != "" {
		query = query.Where("auth_provider_namespace = ? AND auth_provider_name = ?", authProviderNamespace, authProviderName)
	}
	if err := query.Find(&cached).Error; err != nil {
		return nil, fmt.Errorf("failed to resolve groups from database: %w", err)
	}

	byID := make(map[string]types.Group, len(ids))
	for _, group := range cached {
		byID[group.ID] = group
	}

	if missing := missingGroupIDs(ids, byID); len(missing) > 0 && authProviderURL != "" {
		fetched, err := c.resolveAuthGroupsFromProvider(ctx, authProviderURL, authProviderNamespace, authProviderName, missing)
		if err != nil {
			// Resolution is best effort. A missing name degrades to the ID, which is still
			// identifiable, and failing the whole page over it would be worse.
			slog.Warn("failed to resolve groups from auth provider, falling back to ids",
				"authProviderName", authProviderName, "authProviderNamespace", authProviderNamespace,
				"groups", len(missing), "error", err)
		} else {
			for _, group := range fetched {
				byID[group.ID] = group
			}

			c.cacheResolvedGroups(ctx, fetched)
		}
	}

	resolved := make([]types.Group, 0, len(ids))
	for _, id := range ids {
		if group, ok := byID[id]; ok {
			resolved = append(resolved, group)
			continue
		}
		resolved = append(resolved, types.Group{
			ID:                    id,
			AuthProviderName:      authProviderName,
			AuthProviderNamespace: authProviderNamespace,
			Name:                  id,
		})
	}

	return resolved, nil
}

// missingGroupIDs returns the requested IDs that resolved is missing, in the order they were asked
// for and without duplicates.
func missingGroupIDs(ids []string, resolved map[string]types.Group) []string {
	missing := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))

	for _, id := range ids {
		if _, ok := resolved[id]; ok {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		missing = append(missing, id)
	}

	return missing
}

// resolveAuthGroupsFromProvider asks the auth provider to name the given group IDs.
//
// A provider that does not implement the route answers 404, which is reported as no results rather
// than as an error: github, google and local have no directory to ask, and an Obot running against
// an older provider image should degrade to IDs rather than log an outage on every page load.
func (c *Client) resolveAuthGroupsFromProvider(ctx context.Context, authProviderURL, authProviderNamespace, authProviderName string, ids []string) ([]types.Group, error) {
	u, err := url.Parse(authProviderURL + "/obot-get-auth-groups")
	if err != nil {
		return nil, fmt.Errorf("failed to parse auth provider URL for group resolution: %w", err)
	}

	q := u.Query()
	q.Set("ids", strings.Join(ids, ","))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build group resolution request: %w", err)
	}

	resp, err := groupProviderClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call auth provider group resolution: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		slog.Debug("auth provider does not support group resolution, using ids",
			"authProviderName", authProviderName, "authProviderNamespace", authProviderNamespace)
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("auth provider group resolution returned status %d: %s", resp.StatusCode, string(body))
	}

	var page struct {
		Items []auth.GroupInfo `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("failed to decode auth provider group resolution: %w", err)
	}

	groups := make([]types.Group, 0, len(page.Items))
	for _, info := range page.Items {
		if info.ID == "" {
			continue
		}
		groups = append(groups, types.Group{
			ID:                    info.ID,
			AuthProviderName:      authProviderName,
			AuthProviderNamespace: authProviderNamespace,
			Name:                  info.Name,
			IconURL:               info.IconURL,
		})
	}

	return groups, nil
}

// cacheResolvedGroups records groups resolved from the provider so the next caller is answered from
// the database.
func (c *Client) cacheResolvedGroups(ctx context.Context, groups []types.Group) {
	if len(groups) == 0 {
		return
	}

	if err := c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "icon_url"}),
	}).Create(&groups).Error; err != nil {
		slog.Warn("failed to cache groups resolved from the auth provider", "groups", len(groups), "error", err)
	}
}

// ListGroupIDsForUser lists the group IDs that the given user is a member of.
// This can include groups from multiple auth providers.
func (c *Client) ListGroupIDsForUser(ctx context.Context, userID uint) ([]string, error) {
	var groupIDs []string
	if err := c.db.WithContext(ctx).Table("group_memberships").Where("user_id = ?", userID).Pluck("group_id", &groupIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to list user group IDs: %w", err)
	}

	return groupIDs, nil
}

// GetUserGroupMemberships fetches group memberships for multiple users in a single query.
// Returns a map of userID to slice of groupIDs.
func (c *Client) GetUserGroupMemberships(ctx context.Context, userIDs []uint) (map[uint][]string, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	type Result struct {
		UserID  uint
		GroupID string
	}

	var results []Result
	err := c.db.WithContext(ctx).
		Table("group_memberships").
		Select("user_id, group_id").
		Where("user_id IN ?", userIDs).
		Find(&results).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch user group memberships: %w", err)
	}

	// Build map
	memberships := make(map[uint][]string, len(userIDs))
	for _, r := range results {
		memberships[r.UserID] = append(memberships[r.UserID], r.GroupID)
	}

	return memberships, nil
}

// ensureGroups ensures the groups that the identity is a member of exist and are up to date.
//
// It MUST be called outside of any open database transaction: the group fetch it performs is an
// HTTP call to the auth provider, and holding a pooled DB connection across that round-trip can
// deadlock in-process auth providers that share the single SQLite connection. The HTTP fetch and
// the database persistence are therefore separated into distinct phases below, with the
// persistence happening in its own short-lived transaction.
func (c *Client) ensureGroups(ctx context.Context, identity *types.Identity) error {
	if identity.AuthProviderName == "" || identity.AuthProviderNamespace == "" {
		// No auth provider info, so we can't fetch groups from the provider
		return nil
	}

	var (
		providerURL    = auth.ProviderURLFromContext(ctx)
		now            = time.Now()
		nextGroupCheck = identity.AuthProviderGroupsLastChecked.Add(groupCheckPeriod)
	)

	// Run one-time Okta group ID migration if this is an Okta auth provider.
	// This manages its own transactions internally and makes its own HTTP calls.
	if providerURL != "" && identity.AuthProviderName == "okta-auth-provider" {
		if err := c.runOktaGroupIDMigrationOnce(ctx, providerURL, identity.AuthProviderNamespace, identity.AuthProviderName); err != nil {
			slog.Warn("Okta group ID migration failed (will retry)", "error", err)
		}
	}

	if nextGroupCheck.After(now) || providerURL == "" {
		// Throttled (or no provider URL): just read the cached groups from the database.
		groups, err := c.listUserGroups(ctx, c.db.WithContext(ctx), identity)
		if err != nil {
			return fmt.Errorf("failed to list user groups: %w", err)
		}

		identity.AuthProviderGroups = groups
		return nil
	}

	// Fetch phase: call the auth provider over HTTP with no open transaction.
	groupLookupID := identity.GroupLookupID()
	providerGroups, err := c.fetchGroups(ctx, providerURL, identity.AuthProviderNamespace, identity.AuthProviderName, groupLookupID)
	if err != nil {
		return err
	}

	identity.AuthProviderGroups = providerGroups
	identity.AuthProviderGroupsLastChecked = now

	// Persist phase: upsert groups and reconcile memberships in a short-lived transaction.
	return c.persistGroups(ctx, identity)
}

// persistGroups persists the identity's freshly fetched AuthProviderGroups to the database and
// reconciles the group memberships. It opens its own transaction and must be called outside of any
// other open transaction. After the transaction commits, it emits any reconciliation events.
func (c *Client) persistGroups(ctx context.Context, identity *types.Identity) error {
	var membershipsChanged, groupsLost bool
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := make([]string, 0, len(identity.AuthProviderGroups))
		for _, group := range identity.AuthProviderGroups {
			ids = append(ids, group.ID)
		}

		var groups []types.Group
		if len(ids) > 0 {
			if err := tx.WithContext(ctx).
				Where("auth_provider_name = ? AND auth_provider_namespace = ? AND id IN ?", identity.AuthProviderName, identity.AuthProviderNamespace, ids).
				Find(&groups).Error; err != nil {
				return fmt.Errorf("failed to list auth provider groups: %w", err)
			}
		}

		existingGroups := make(map[string]types.Group, len(groups))
		for _, group := range groups {
			existingGroups[group.ID] = group
		}

		var toUpsert []types.Group
		for _, group := range identity.AuthProviderGroups {
			if existing, ok := existingGroups[group.ID]; ok && existing.Name == group.Name && existing.IconURL == group.IconURL {
				// The group already exists and is up to date, skip
				continue
			}
			toUpsert = append(toUpsert, group)
		}

		if len(toUpsert) > 0 {
			if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "id"},
				},
				DoUpdates: clause.AssignmentColumns([]string{"name", "icon_url"}),
			}).Create(&toUpsert).Error; err != nil {
				return fmt.Errorf("failed to upsert groups: %w", err)
			}
		}

		var err error
		membershipsChanged, groupsLost, err = c.ensureGroupMemberships(ctx, tx, identity)
		if err != nil {
			return fmt.Errorf("failed to update group memberships for identity: %w", err)
		}

		return nil
	}); err != nil {
		return err
	}

	// If memberships changed, trigger reconciliation for this user
	if membershipsChanged {
		if err := c.storageClient.Create(ctx, &v1.UserRoleChange{
			GenerateName: system.UserRoleChangePrefix,
			Namespace:    system.DefaultNamespace,
			Spec: v1.UserRoleChangeSpec{
				UserID: identity.UserID,
			},
		}); err != nil {
			slog.Warn("failed to create user role change event for user", "userID", identity.UserID, "error", err)
			// Don't fail authentication - membership update succeeded
		}
	}

	// If user lost groups, trigger MCP server cleanup
	if groupsLost {
		if err := c.storageClient.Create(ctx, &v1.UserGroupChange{
			GenerateName: system.UserGroupChangePrefix,
			Namespace:    system.DefaultNamespace,
			Spec: v1.UserGroupChangeSpec{
				UserID: identity.UserID,
			},
		}); err != nil {
			slog.Warn("failed to create user group change event for user", "userID", identity.UserID, "error", err)
			// Don't fail authentication - membership update succeeded
		}
	}

	return nil
}

// ensureGroupMemberships ensures the Identity is a member of the groups it references.
// Returns (membershipsChanged, groupsLost, error) where:
//   - membershipsChanged: true if user joined or left any groups
//   - groupsLost: true if user left at least one group
func (c *Client) ensureGroupMemberships(ctx context.Context, tx *gorm.DB, identity *types.Identity) (bool, bool, error) {
	// Get the existing memberships for this identity
	var memberships []types.GroupMemberships
	if err := tx.WithContext(ctx).
		Joins("JOIN groups ON group_memberships.group_id = groups.id").
		Where("group_memberships.user_id = ?", identity.UserID).
		Where("groups.auth_provider_namespace = ? AND groups.auth_provider_name = ?", identity.AuthProviderNamespace, identity.AuthProviderName).
		Find(&memberships).Error; err != nil {
		return false, false, fmt.Errorf("failed to get existing group memberships: %w", err)
	}

	existingMemberships := make(map[string]types.GroupMemberships, len(memberships))
	for _, membership := range memberships {
		existingMemberships[membership.GroupID] = membership
	}

	var toInsert []types.GroupMemberships
	for _, group := range identity.AuthProviderGroups {
		if _, ok := existingMemberships[group.ID]; ok {
			// The membership already exists, skip
			delete(existingMemberships, group.ID)
			continue
		}

		toInsert = append(toInsert, types.GroupMemberships{
			UserID:  identity.UserID,
			GroupID: group.ID,
		})
	}

	// Insert new memberships
	if len(toInsert) > 0 {
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&toInsert).Error; err != nil {
			return false, false, fmt.Errorf("failed to create group memberships: %w", err)
		}
	}

	toDelete := make([]types.GroupMemberships, 0, len(existingMemberships))
	for _, membership := range existingMemberships {
		toDelete = append(toDelete, membership)
	}

	if len(toDelete) > 0 {
		// Delete memberships that are no longer in the identity's auth provider groups
		if err := tx.WithContext(ctx).Delete(&toDelete).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, false, fmt.Errorf("failed to delete group memberships: %w", err)
		}
	}

	// Return true if any memberships were added or removed
	membershipsChanged := len(toInsert) > 0 || len(toDelete) > 0
	groupsLost := len(toDelete) > 0
	return membershipsChanged, groupsLost, nil
}

// deleteGroupMembershipsForUser deletes all group memberships for the given user.
func (c *Client) deleteGroupMembershipsForUser(ctx context.Context, tx *gorm.DB, userID uint) error {
	if err := tx.WithContext(ctx).Where("user_id = ?", userID).Delete(&types.GroupMemberships{}).Error; err != nil {
		return fmt.Errorf("failed to delete group memberships for user: %w", err)
	}
	return nil
}

// listUserGroups lists the groups that the user is a member of from the database.
func (*Client) listUserGroups(ctx context.Context, tx *gorm.DB, identity *types.Identity) ([]types.Group, error) {
	if identity == nil {
		return nil, fmt.Errorf("identity is nil")
	}
	if identity.UserID == 0 {
		return nil, fmt.Errorf("identity has no user id")
	}
	if identity.AuthProviderNamespace == "" || identity.AuthProviderName == "" {
		return nil, fmt.Errorf("identity missing auth provider info")
	}

	var groups []types.Group
	if err := tx.WithContext(ctx).
		Table("groups").
		Select("groups.*").
		Joins("JOIN group_memberships ON group_memberships.group_id = groups.id").
		Where("group_memberships.user_id = ?", identity.UserID).
		Where("groups.auth_provider_namespace = ? AND groups.auth_provider_name = ?", identity.AuthProviderNamespace, identity.AuthProviderName).
		Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("failed to list user groups: %w", err)
	}

	return groups, nil
}

// fetchGroups fetches the groups that the user is a member of from the auth provider.
func (*Client) fetchGroups(ctx context.Context, authProviderURL, authProviderNamespace, authProviderName, providerUserID string) ([]types.Group, error) {
	// Fetch groups from the auth provider, ignore errors so that auth providers that don't yet
	// implement group support don't block the user from logging in.
	var providerGroups []auth.GroupInfo

	// Get the SerializableRequest from context
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authProviderURL+"/obot-list-user-auth-groups", strings.NewReader(providerUserID))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, &FetchUserGroupsError{
			ProviderUserID: providerUserID,
			Message:        fmt.Sprintf("failed to fetch groups for user with ID %s: %v", providerUserID, err),
		}
	}

	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&providerGroups); err != nil {
			return nil, &FetchUserGroupsError{
				ProviderUserID: providerUserID,
				Message:        fmt.Sprintf("failed to decode groups for user with ID %s: %v", providerUserID, err),
			}
		}
	} else if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		if len(body) != 0 {
			return nil, &FetchUserGroupsError{
				ProviderUserID: providerUserID,
				Message:        string(body),
			}
		}

		return nil, &FetchUserGroupsError{
			ProviderUserID: providerUserID,
			Message:        resp.Status,
		}
	}

	var userGroups []types.Group
	for _, group := range providerGroups {
		userGroups = append(userGroups, types.Group{
			ID:                    group.ID,
			AuthProviderName:      authProviderName,
			AuthProviderNamespace: authProviderNamespace,
			Name:                  group.Name,
			IconURL:               group.IconURL,
		})
	}

	return userGroups, nil
}
