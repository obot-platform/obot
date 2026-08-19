package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// groupCheckPeriod defines how often the system checks for updates to group information from the auth provider.
	groupCheckPeriod = time.Minute * 10
)

// FetchUserGroupsError represents an error that occurs when fetching user groups from the auth provider.
// This error indicates a configuration issue with the auth provider that requires administrator intervention.
type FetchUserGroupsError struct {
	ProviderUserID string
	Message        string
}

func (e *FetchUserGroupsError) Error() string {
	return fmt.Sprintf("auth provider failed to check groups for user with ID %s: %s", e.ProviderUserID, e.Message)
}

type ListAuthGroupsOptions struct {
	// NameFilter, when set, restricts results to groups whose name matches it.
	NameFilter string

	Limit  int
	Offset int
}

type listAuthGroupsPage struct {
	Items []auth.GroupInfo `json:"items"`
	Total int              `json:"total"`
}

// ListAuthGroups returns one page of the auth provider's groups.
//
// The auth provider is the authoritative source. When it cannot be listed, the response falls back
// to the groups table, which only ever contains groups observed during a user sign-in and is
// therefore partial.
func (c *Client) ListAuthGroups(ctx context.Context, authProviderURL, authProviderNamespace, authProviderName string, opts ListAuthGroupsOptions) ([]types.Group, int, string, bool, error) {
	if authProviderURL != "" {
		groups, total, status, err := c.listAuthGroupsFromProvider(ctx, authProviderURL, authProviderNamespace, authProviderName, opts)
		switch {
		case err == nil:
			return groups, total, types.GroupSourceProvider, false, nil
		case status == http.StatusNotFound:
			// The provider does not implement group listing (e.g. github, google, local). The
			// cached groups are all there has ever been, so this is not a degraded response.
			slog.Debug("auth provider does not support group listing, using cached groups",
				"authProviderName", authProviderName, "authProviderNamespace", authProviderNamespace)
		default:
			// Fall back to cached groups so the UI keeps working when the provider is unreachable
			// or the credentials lack directory-wide read permission.
			slog.Warn("failed to list groups from auth provider, falling back to cached groups",
				"authProviderName", authProviderName, "authProviderNamespace", authProviderNamespace, "error", err)

			cached, cachedTotal, cacheErr := c.listAuthGroupsFromCache(ctx, authProviderNamespace, authProviderName, opts)
			if cacheErr != nil {
				return nil, 0, "", false, cacheErr
			}
			return cached, cachedTotal, types.GroupSourceCache, true, nil
		}
	}

	cached, total, err := c.listAuthGroupsFromCache(ctx, authProviderNamespace, authProviderName, opts)
	if err != nil {
		return nil, 0, "", false, err
	}

	return cached, total, types.GroupSourceCache, false, nil
}

// listAuthGroupsFromProvider asks the auth provider for a page of groups. It returns the provider's
// HTTP status alongside the error so the caller can tell "not implemented" apart from a failure.
func (c *Client) listAuthGroupsFromProvider(ctx context.Context, authProviderURL, authProviderNamespace, authProviderName string, opts ListAuthGroupsOptions) ([]types.Group, int, int, error) {
	u, err := url.Parse(authProviderURL + "/obot-list-auth-groups")
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to parse auth provider URL for group search: %w", err)
	}

	q := u.Query()
	if opts.NameFilter != "" {
		q.Set("name", opts.NameFilter)
	}
	// Always send a limit: it is what selects the paginated response shape, so a provider that
	// predates pagination still answers with a plain array and is handled below.
	q.Set("limit", strconv.Itoa(opts.Limit))
	q.Set("offset", strconv.Itoa(opts.Offset))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to build group listing request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to call auth provider group listing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, 0, resp.StatusCode, fmt.Errorf("auth provider group listing returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, resp.StatusCode, fmt.Errorf("failed to read auth provider group listing: %w", err)
	}

	var (
		page      listAuthGroupsPage
		infos     []auth.GroupInfo
		total     int
		paginated bool
	)
	if err := json.Unmarshal(body, &page); err == nil && page.Items != nil {
		infos, total, paginated = page.Items, page.Total, true
	} else if err := json.Unmarshal(body, &infos); err != nil {
		return nil, 0, resp.StatusCode, fmt.Errorf("failed to decode auth provider group listing: %w", err)
	}

	groups := make([]types.Group, 0, len(infos))
	for _, info := range infos {
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

	if !paginated {
		// An older provider ignored the limit and returned everything, so page it here instead.
		total = len(groups)
		groups = pageGroups(groups, opts.Limit, opts.Offset)
	}

	return groups, total, resp.StatusCode, nil
}

// listAuthGroupsFromCache pages over the groups table, which holds the groups seen during previous
// user sign-ins.
func (c *Client) listAuthGroupsFromCache(ctx context.Context, authProviderNamespace, authProviderName string, opts ListAuthGroupsOptions) ([]types.Group, int, error) {
	if authProviderNamespace == "" || authProviderName == "" {
		return []types.Group{}, 0, nil
	}

	query := c.db.WithContext(ctx).Model(&types.Group{}).
		Where("auth_provider_namespace = ? AND auth_provider_name = ?", authProviderNamespace, authProviderName)

	// Case-insensitive, compatible with SQLite and PostgreSQL.
	if opts.NameFilter != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+opts.NameFilter+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count groups in database: %w", err)
	}

	var groups []types.Group
	if err := query.Order("name").Limit(opts.Limit).Offset(opts.Offset).Find(&groups).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch groups from database: %w", err)
	}

	return groups, int(total), nil
}

// ResolveAuthGroups looks up specific groups by ID, which is what the UI needs to render a name for
// a group that already has a role or policy attached. Unknown IDs come back as a group whose name
// is the ID itself, so a caller renders something identifiable instead of dropping the row.
func (c *Client) ResolveAuthGroups(ctx context.Context, authProviderNamespace, authProviderName string, ids []string) ([]types.Group, error) {
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

	byID := make(map[string]types.Group, len(cached))
	for _, group := range cached {
		byID[group.ID] = group
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

// pageGroups returns the requested window of a listing.
func pageGroups(groups []types.Group, limit, offset int) []types.Group {
	if offset >= len(groups) {
		return []types.Group{}
	}
	return groups[offset:min(offset+limit, len(groups))]
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
		// Get the groups from the database
		var groups []types.Group
		if err := tx.WithContext(ctx).Where("auth_provider_name = ? AND auth_provider_namespace = ?", identity.AuthProviderName, identity.AuthProviderNamespace).Find(&groups).Error; err != nil {
			return fmt.Errorf("failed to list auth provider groups: %w", err)
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
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.UserRoleChangePrefix,
				Namespace:    system.DefaultNamespace,
			},
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
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.UserGroupChangePrefix,
				Namespace:    system.DefaultNamespace,
			},
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
