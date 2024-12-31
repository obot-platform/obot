package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func (c *Client) User(ctx context.Context, username string) (*types.User, error) {
	u := new(types.User)
	return u, c.db.WithContext(ctx).Where("username = ?", username).First(u).Error
}

func (c *Client) UserByID(ctx context.Context, id string) (*types.User, error) {
	u := new(types.User)
	return u, c.db.WithContext(ctx).Where("id = ?", id).First(u).Error
}

func (c *Client) UpdateProfileIconIfNeeded(ctx context.Context, user *types.User, authProviderName, authProviderNamespace, authProviderURL string) error {
	if authProviderName == "" || authProviderNamespace == "" || authProviderURL == "" {
		return nil
	}

	accessToken := accesstoken.GetAccessToken(ctx)
	if accessToken == "" {
		return nil
	}

	var (
		identity types.Identity
	)
	if err := c.db.WithContext(ctx).Where("user_id = ?", user.ID).
		Where("auth_provider_name = ?", authProviderName).
		Where("auth_provider_namespace = ?", authProviderNamespace).
		First(&identity).Error; err != nil {
		return err
	}

	if time.Until(identity.IconLastChecked) > -7*24*time.Hour {
		// Icon was checked less than 7 days ago.
		return nil
	}

	profileIconURL, err := c.fetchProfileIconURL(ctx, authProviderURL, accessToken)
	if err != nil {
		return err
	}

	user.IconURL = profileIconURL
	identity.IconURL = profileIconURL
	identity.IconLastChecked = time.Now()

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Updates(user).Error; err != nil {
			return err
		}
		return tx.Updates(&identity).Error
	})
}

func (c *Client) fetchProfileIconURL(ctx context.Context, authProviderURL, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authProviderURL+"/obot-get-icon-url", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch profile icon URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch profile icon URL: %s", resp.Status)
	}

	var body struct {
		IconURL string `json:"iconURL"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return body.IconURL, nil
}
