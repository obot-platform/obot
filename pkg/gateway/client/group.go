package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/obot-platform/obot/pkg/accesstoken"
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

// ListAuthGroups lists the auth provider groups for the given auth provider.
//
// It supports fuzzy finding group names using on the given nameFilter.
// It queries the auth provider for "live" group search from the auth provider, then combines the
// results with cached groups from the database.
// This allows admins to discover groups that authenticated users belong to for auth providers
// limited group search capabilities; e.g. there's not an effective way to perform a fuzzy search for
// GitHub teams or orgs by name.
func (c *Client) ListAuthGroups(ctx context.Context, authProviderURL, authProviderNamespace, authProviderName, nameFilter string) ([]types.Group, error) {
	// Fetch groups from the auth provider
	var providerGroups []auth.GroupInfo
	if authProviderURL != "" {
		u, err := url.Parse(authProviderURL + "/obot-list-auth-groups")
		if err != nil {
			log.Warnf("failed to parse auth provider URL for group search: %v", err)
		} else {
			// We ignore errors here so that clients can still search over cached groups where there
			// are issues fetching them from the auth provider.
			if nameFilter != "" {
				q := u.Query()
				q.Set("name", nameFilter)
				u.RawQuery = q.Encode()
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err == nil {
				if accessToken := accesstoken.GetAccessToken(ctx); accessToken != "" {
					req.Header.Set("Authorization", "Bearer "+accessToken)
				}

				resp, err := http.DefaultClient.Do(req)
				if err == nil {
					defer resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						_ = json.NewDecoder(resp.Body).Decode(&providerGroups)
					}
				}
			}
		}
	}

	// Fetch groups from the database if we have auth provider info
	var dbGroups []types.Group
	if authProviderNamespace != "" && authProviderName != "" {
		query := c.db.WithContext(ctx).Where("auth_provider_namespace = ? AND auth_provider_name = ?",
			authProviderNamespace, authProviderName)

		// Apply name filter if provided (case-insensitive, compatible with SQLite and PostgreSQL)
		if nameFilter != "" {
			query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+nameFilter+"%")
		}

		if err := query.Find(&dbGroups).Error; err != nil {
			return nil, fmt.Errorf("failed to fetch groups from database: %w", err)
		}
	}

	groups := make(map[string]types.Group, len(dbGroups))
	for _, group := range dbGroups {
		groups[group.ID] = group
	}

	// Add/merge provider groups
	for _, providerGroup := range providerGroups {
		if providerGroup.ID == "" {
			continue
		}

		if existing, ok := groups[providerGroup.ID]; ok {
			// Keep database timestamps but update other fields from provider
			if providerGroup.Name != "" {
				existing.Name = providerGroup.Name
			}
			if providerGroup.IconURL != nil {
				existing.IconURL = providerGroup.IconURL
			}
			groups[providerGroup.ID] = existing
			continue
		}

		groups[providerGroup.ID] = types.Group{
			ID:                    providerGroup.ID,
			AuthProviderName:      authProviderName,
			AuthProviderNamespace: authProviderNamespace,
			Name:                  providerGroup.Name,
			IconURL:               providerGroup.IconURL,
		}
	}

	result := make([]types.Group, 0, len(groups))
	for _, group := range groups {
		result = append(result, group)
	}

	return result, nil
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
func (c *Client) ensureGroups(ctx context.Context, tx *gorm.DB, identity *types.Identity) error {
	if identity.AuthProviderName == "" || identity.AuthProviderNamespace == "" {
		// No auth provider info, so we can't fetch groups from the provider
		return nil
	}

	var (
		providerURL    = auth.ProviderURLFromContext(ctx)
		token          = accesstoken.GetAccessToken(ctx)
		now            = time.Now()
		nextGroupCheck = identity.AuthProviderGroupsLastChecked.Add(groupCheckPeriod)
	)
	if nextGroupCheck.After(now) || providerURL == "" || token == "" {
		groups, err := c.listUserGroups(ctx, tx, identity)
		if err != nil {
			return fmt.Errorf("failed to list user groups: %w", err)
		}

		identity.AuthProviderGroups = groups
		return nil
	}

	// Fetch groups from the auth provider
	providerGroups, err := c.fetchGroups(ctx, providerURL, token, identity.AuthProviderNamespace, identity.AuthProviderName)
	if err != nil {
		return fmt.Errorf("failed to list user groups from provider: %w", err)
	}

	identity.AuthProviderGroups = providerGroups
	identity.AuthProviderGroupsLastChecked = now

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

	membershipsChanged, err := c.ensureGroupMemberships(ctx, tx, identity)
	if err != nil {
		return fmt.Errorf("failed to update group memberships for identity: %w", err)
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
			log.Warnf("failed to create user role change event for user %d: %v", identity.UserID, err)
			// Don't fail authentication - membership update succeeded
		}
	}

	return nil
}

// ensureGroupMemberships ensures the Identity is a member of the groups it references.
// Returns true if memberships changed (user joined or left any groups).
func (c *Client) ensureGroupMemberships(ctx context.Context, tx *gorm.DB, identity *types.Identity) (bool, error) {
	// Get the existing memberships for this identity
	var memberships []types.GroupMemberships
	if err := tx.WithContext(ctx).
		Joins("JOIN groups ON group_memberships.group_id = groups.id").
		Where("group_memberships.user_id = ?", identity.UserID).
		Where("groups.auth_provider_namespace = ? AND groups.auth_provider_name = ?", identity.AuthProviderNamespace, identity.AuthProviderName).
		Find(&memberships).Error; err != nil {
		return false, fmt.Errorf("failed to get existing group memberships: %w", err)
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
			return false, fmt.Errorf("failed to create group memberships: %w", err)
		}
	}

	toDelete := make([]types.GroupMemberships, 0, len(existingMemberships))
	for _, membership := range existingMemberships {
		toDelete = append(toDelete, membership)
	}

	if len(toDelete) > 0 {
		// Delete memberships that are no longer in the identity's auth provider groups
		if err := tx.WithContext(ctx).Delete(&toDelete).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, fmt.Errorf("failed to delete group memberships: %w", err)
		}
	}

	// Return true if any memberships were added or removed
	membershipsChanged := len(toInsert) > 0 || len(toDelete) > 0
	return membershipsChanged, nil
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

// fetchGroups fetches the groups that the owner of the access token is a member of from the auth provider.
func (*Client) fetchGroups(ctx context.Context, authProviderURL, accessToken, authProviderNamespace, authProviderName string) ([]types.Group, error) {
	// Fetch groups from the auth provider, ignore errors so that auth providers that don't yet
	// implement group support don't block the user from logging in.
	providerGroups := []auth.GroupInfo{}
	u, err := url.Parse(authProviderURL + "/obot-list-user-auth-groups")
	if err == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+accessToken)

			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					_ = json.NewDecoder(resp.Body).Decode(&providerGroups)
				}
			}
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
