package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, authProviderURL+"/obot-list-auth-groups", nil)
			if err == nil {
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

// ensureGroups ensures the groups that the identity is a member of exist and are up to date.
func (c *Client) ensureGroups(ctx context.Context, tx *gorm.DB, identity *types.Identity) error {
	if identity.AuthProviderName == "" || identity.AuthProviderNamespace == "" {
		// No auth provider info, so we can't fetch groups from the provider
		return nil
	}

	var (
		providerURL    = auth.ProviderURLFromContext(ctx)
		now            = time.Now()
		nextGroupCheck = identity.AuthProviderGroupsLastChecked.Add(groupCheckPeriod)
	)
	if nextGroupCheck.After(now) || providerURL == "" {
		groups, err := c.listUserGroups(ctx, tx, identity)
		if err != nil {
			return fmt.Errorf("failed to list user groups: %w", err)
		}

		identity.AuthProviderGroups = groups
		return nil
	}

	fmt.Printf("obot: fetching groups for user from auth provider\n")

	// Fetch groups from the auth provider
	providerGroups, err := c.fetchGroups(ctx, providerURL, identity.AuthProviderNamespace, identity.AuthProviderName, identity.Email)
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

	if err := c.ensureGroupMemberships(ctx, tx, identity); err != nil {
		return fmt.Errorf("failed to update group memberships for identity: %w", err)
	}

	return nil
}

// ensureGroupMemberships ensures the Identity is a member of the groups it references.
func (c *Client) ensureGroupMemberships(ctx context.Context, tx *gorm.DB, identity *types.Identity) error {
	// Get the existing memberships for this identity
	var memberships []types.GroupMemberships
	if err := tx.WithContext(ctx).
		Joins("JOIN groups ON group_memberships.group_id = groups.id").
		Where("group_memberships.user_id = ?", identity.UserID).
		Where("groups.auth_provider_namespace = ? AND groups.auth_provider_name = ?", identity.AuthProviderNamespace, identity.AuthProviderName).
		Find(&memberships).Error; err != nil {
		return fmt.Errorf("failed to get existing group memberships: %w", err)
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
			return fmt.Errorf("failed to create group memberships: %w", err)
		}
	}

	toDelete := make([]types.GroupMemberships, 0, len(existingMemberships))
	for _, membership := range existingMemberships {
		toDelete = append(toDelete, membership)
	}

	if len(toDelete) > 0 {
		// Delete memberships that are no longer in the identity's auth provider groups
		if err := tx.WithContext(ctx).Delete(&toDelete).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to delete group memberships: %w", err)
		}
	}

	return nil
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
	if err == nil {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				_ = json.NewDecoder(resp.Body).Decode(&providerGroups)
			} else {
				fmt.Printf("obot: got status: %d\n", resp.StatusCode)
				var body []byte
				body, err = ioutil.ReadAll(resp.Body)
				if err == nil {
					fmt.Printf("obot: got body: %s\n", string(body))
				}
			}
		} else {
			fmt.Printf("failed to fetch groups: %s\n", err)
		}
	} else {
		fmt.Printf("obot: failed to fetch groups: %s\n", err)
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
