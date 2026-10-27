package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/utils"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

const (

	// MCPOAuthPendingStateTTL bounds both pending attempts and temporary challenge batches.
	MCPOAuthPendingStateTTL = 30 * time.Minute
	pendingStateTTL         = MCPOAuthPendingStateTTL
)

var (
	mcpOAuthTokenGroupResource = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "mcpoauthtokens",
	}

	mcpOAuthPendingStateGroupResource = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "mcpoauthpendingstates",
	}
)

func (c *Client) GetMCPOAuthToken(ctx context.Context, userID, mcpID, url string) (*types.MCPOAuthToken, error) {
	var tokens []types.MCPOAuthToken
	err := c.db.WithContext(ctx).Where("mcp_id = ? AND user_id = ?", mcpID, userID).Find(&tokens).Error
	if err != nil {
		return nil, err
	}

	var token types.MCPOAuthToken
	for _, t := range tokens {
		if t.URL == url {
			token = t
			break
		}
	}

	if token.MCPID == "" {
		// We didn't find a token. If there is only one, then use that one.
		if len(tokens) != 1 {
			return nil, gorm.ErrRecordNotFound
		}

		token = tokens[0]
	}

	if err = c.decryptMCPOAuthToken(ctx, &token); err != nil {
		return nil, fmt.Errorf("failed to decrypt token: %w", err)
	}

	return &token, nil
}

// MCPOAuthDiscoveryRevision separates cached discovery by the caller's current grants.
// Removal, reauthorization, and scope changes affect discovery identity. Token
// values and expiry are excluded; read only grant metadata, never client secrets.
func (c *Client) MCPOAuthDiscoveryRevision(ctx context.Context, userID string, mcpIDs []string) (string, error) {
	var grants []struct {
		MCPID              string
		URL                string
		OAuthAuthRequestID string
		GrantID            string
		Scopes             string
	}
	if len(mcpIDs) > 0 {
		if err := c.db.WithContext(ctx).Model(&types.MCPOAuthToken{}).
			Select("MCPID", "URL", "OAuthAuthRequestID", "GrantID", "Scopes").
			Where("user_id = ? AND mcp_id IN ?", userID, mcpIDs).
			Order("mcp_id").Scan(&grants).Error; err != nil {
			return "", err
		}
	}

	return utils.Digest([]any{userID, grants}), nil
}

func (c *Client) ReplaceMCPOAuthToken(ctx context.Context, userID, mcpID, url, oauthAuthRequestID string, oauthConf *oauth2.Config, token *oauth2.Token, challenge ...string) error {
	var captured string
	if len(challenge) > 0 {
		captured = challenge[0]
	}

	return c.saveMCPOAuthToken(ctx, userID, mcpID, url, oauthAuthRequestID, oauthConf, token, nil, captured)
}

func (c *Client) RefreshMCPOAuthToken(ctx context.Context, userID, mcpID, url, grantID string, oauthConf *oauth2.Config, token *oauth2.Token) error {
	return c.saveMCPOAuthToken(ctx, userID, mcpID, url, "", oauthConf, token, &grantID, "")
}

func (c *Client) saveMCPOAuthToken(ctx context.Context, userID, mcpID, url, oauthAuthRequestID string, oauthConf *oauth2.Config, token *oauth2.Token, refreshGrantID *string, challenge string) error {
	t := &types.MCPOAuthToken{
		GrantID:            rand.Text(),
		UpstreamChallenge:  challenge,
		UserID:             userID,
		MCPID:              mcpID,
		URL:                url,
		OAuthAuthRequestID: oauthAuthRequestID,
		AccessToken:        token.AccessToken,
		TokenType:          token.TokenType,
		RefreshToken:       token.RefreshToken,
		Expiry:             token.Expiry,
		ExpiresIn:          token.ExpiresIn,
		ClientID:           oauthConf.ClientID,
		ClientSecret:       oauthConf.ClientSecret,
		Endpoint:           oauthConf.Endpoint,
		RedirectURL:        oauthConf.RedirectURL,
		Scopes:             strings.Join(oauthConf.Scopes, " "),
	}

	if err := c.encryptMCPOAuthToken(ctx, t); err != nil {
		return fmt.Errorf("failed to encrypt token: %w", err)
	}

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Insert first so concurrent callbacks also serialize when no grant exists yet.
		if refreshGrantID == nil {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(t)
			if result.Error != nil || result.RowsAffected == 1 {
				return result.Error
			}
		}

		var current types.MCPOAuthToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("URL", "AuthURL", "TokenURL", "Scopes", "GrantID", "OAuthAuthRequestID", "UpstreamChallenge").
			Where("user_id = ? AND mcp_id = ?", userID, mcpID).Take(&current).Error; err != nil {
			return err
		}

		// A callback or refresh started before another grant must not erase its permissions.
		if current.URL == url && current.AuthURL == oauthConf.Endpoint.AuthURL && current.TokenURL == oauthConf.Endpoint.TokenURL {
			for scope := range strings.FieldsSeq(current.Scopes) {
				if !slices.Contains(oauthConf.Scopes, scope) {
					return fmt.Errorf("upstream OAuth permissions changed; start a new authorization")
				}
			}
		}

		if refreshGrantID != nil {
			if current.GrantID != *refreshGrantID || current.URL != url {
				return fmt.Errorf("upstream OAuth grant changed during refresh")
			}

			t.GrantID = current.GrantID
			t.OAuthAuthRequestID = current.OAuthAuthRequestID
			t.UpstreamChallenge = current.UpstreamChallenge
		}

		return tx.Save(t).Error
	}); err != nil {
		return err
	}

	return c.triggerMCPOAuthTokenChange(ctx, mcpID)
}

// CopyMCPOAuthTokens preserves a migrated connection's upstream authorization.
// Existing destination tokens take precedence, including tokens refreshed after
// a partially completed migration.
func (c *Client) CopyMCPOAuthTokens(ctx context.Context, userID, sourceID, targetID string) error {
	var tokens []types.MCPOAuthToken
	if err := c.db.WithContext(ctx).Where("mcp_id = ? AND user_id = ?", sourceID, userID).Find(&tokens).Error; err != nil {
		return err
	}
	if len(tokens) == 0 || sourceID == targetID {
		return nil
	}
	for i := range tokens {
		if err := c.decryptMCPOAuthToken(ctx, &tokens[i]); err != nil {
			return fmt.Errorf("failed to decrypt source MCP OAuth token: %w", err)
		}
		tokens[i].MCPID = targetID
		if err := c.encryptMCPOAuthToken(ctx, &tokens[i]); err != nil {
			return fmt.Errorf("failed to encrypt destination MCP OAuth token: %w", err)
		}
	}
	if err := c.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&tokens).Error; err != nil {
		return err
	}
	return c.triggerMCPOAuthTokenChange(ctx, targetID)
}

func (c *Client) DeleteMCPOAuthTokenForURL(ctx context.Context, userID, mcpID, mcpURL string) error {
	if err := c.db.WithContext(ctx).Delete(&types.MCPOAuthToken{}, "user_id = ? AND mcp_id = ? AND (url = ? OR url = ?)", userID, mcpID, mcpURL, "").Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return c.triggerMCPOAuthTokenChange(ctx, mcpID)
}

func (c *Client) DeleteMCPOAuthTokens(ctx context.Context, userID, mcpID string) error {
	if err := c.db.WithContext(ctx).Delete(&types.MCPOAuthToken{}, "user_id = ? AND mcp_id = ?", userID, mcpID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return c.triggerMCPOAuthTokenChange(ctx, mcpID)
}

func (c *Client) DeleteMCPOAuthTokenForAllUsers(ctx context.Context, mcpID string) error {
	if err := c.db.WithContext(ctx).Delete(&types.MCPOAuthToken{}, "mcp_id = ?", mcpID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return c.triggerMCPOAuthTokenChange(ctx, mcpID)
}

func (c *Client) triggerMCPOAuthTokenChange(ctx context.Context, mcpID string) error {
	if mcpID == "" || c.mcpOAuthTokenTrigger == nil {
		return nil
	}
	return c.mcpOAuthTokenTrigger(ctx, mcpID)
}

// Pending state methods

func (c *Client) CreateMCPOAuthPendingState(ctx context.Context, userID, mcpID, mcpURL, tunnelName, oauthAuthRequestID, state, verifier, resourceURL string, oauthConf *oauth2.Config, upstreamChallenge ...string) error {
	hashedState := fmt.Sprintf("%x", sha256.Sum256([]byte(state)))
	ps := &types.MCPOAuthPendingState{
		HashedState:        hashedState,
		State:              state,
		Verifier:           verifier,
		UserID:             userID,
		MCPID:              mcpID,
		URL:                mcpURL,
		TunnelName:         tunnelName,
		ResourceURL:        resourceURL,
		OAuthAuthRequestID: oauthAuthRequestID,
		ClientID:           oauthConf.ClientID,
		ClientSecret:       oauthConf.ClientSecret,
		AuthURL:            oauthConf.Endpoint.AuthURL,
		TokenURL:           oauthConf.Endpoint.TokenURL,
		AuthStyle:          oauthConf.Endpoint.AuthStyle,
		RedirectURL:        oauthConf.RedirectURL,
		Scopes:             strings.Join(oauthConf.Scopes, " "),
	}

	if len(upstreamChallenge) > 0 {
		ps.UpstreamChallenge = upstreamChallenge[0]
	}

	if err := c.encryptMCPOAuthPendingState(ctx, ps); err != nil {
		return fmt.Errorf("failed to encrypt pending state: %w", err)
	}

	return c.db.WithContext(ctx).Create(ps).Error
}

func (c *Client) GetMCPOAuthPendingState(ctx context.Context, state string) (*types.MCPOAuthPendingState, error) {
	hashedState := fmt.Sprintf("%x", sha256.Sum256([]byte(state)))
	ps := new(types.MCPOAuthPendingState)
	if err := c.db.WithContext(ctx).Where("hashed_state = ?", hashedState).First(ps).Error; err != nil {
		return nil, err
	}

	if err := c.decryptMCPOAuthPendingState(ctx, ps); err != nil {
		return nil, fmt.Errorf("failed to decrypt pending state: %w", err)
	}

	return ps, nil
}

// ConsumeMCPOAuthPendingState elects one callback across replicas before code exchange.
// Decrypt first so a storage/decryption failure leaves the attempt available to its owner.
func (c *Client) ConsumeMCPOAuthPendingState(ctx context.Context, state string) (*types.MCPOAuthPendingState, error) {
	pending, err := c.GetMCPOAuthPendingState(ctx, state)
	if err != nil {
		return nil, err
	}

	result := c.db.WithContext(ctx).Delete(&types.MCPOAuthPendingState{}, "hashed_state = ?", pending.HashedState)
	if result.Error != nil {
		return nil, result.Error
	}

	if result.RowsAffected != 1 {
		return nil, gorm.ErrRecordNotFound
	}

	return pending, nil
}

// FindMCPOAuthPendingState retrieves the same live attempt after a transaction URL save failed.
func (c *Client) FindMCPOAuthPendingState(ctx context.Context, userID, mcpID, mcpURL, requestID, challenge string) (*types.MCPOAuthPendingState, error) {
	pending := new(types.MCPOAuthPendingState)
	binding := &types.MCPOAuthPendingState{
		UserID:             userID,
		MCPID:              mcpID,
		URL:                mcpURL,
		OAuthAuthRequestID: requestID,
		UpstreamChallenge:  challenge,
	}
	if err := c.db.WithContext(ctx).
		Where(binding, "UserID", "MCPID", "URL", "OAuthAuthRequestID", "UpstreamChallenge").
		Where("created_at > ?", time.Now().Add(-pendingStateTTL)).
		First(pending).Error; err != nil {
		return nil, err
	}

	if err := c.decryptMCPOAuthPendingState(ctx, pending); err != nil {
		return nil, fmt.Errorf("failed to decrypt pending state: %w", err)
	}

	return pending, nil
}

func (c *Client) DeleteMCPOAuthPendingState(ctx context.Context, hashedState string) error {
	return c.db.WithContext(ctx).Delete(&types.MCPOAuthPendingState{}, "hashed_state = ?", hashedState).Error
}

func (c *Client) CleanupExpiredMCPOAuthPendingStates(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	if err := c.db.WithContext(ctx).Delete(&types.MCPOAuthChallenge{}, "expires_at < ?", time.Now()).Error; err != nil {
		return err
	}

	if err := c.db.WithContext(ctx).Delete(&types.MCPOAuthAttemptBudget{}, "expires_at < ?", time.Now()).Error; err != nil {
		return err
	}

	return c.db.WithContext(ctx).Delete(&types.MCPOAuthPendingState{}, "created_at < ?", cutoff).Error
}

func (c *Client) runPendingStateCleanup(ctx context.Context) {
	timer := time.NewTimer(pendingStateTTL)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if err := c.CleanupExpiredMCPOAuthPendingStates(ctx, pendingStateTTL); err != nil {
			slog.Error("Failed to cleanup expired MCP OAuth pending states", "error", err)
		}

		timer.Reset(pendingStateTTL)
	}
}

// Encryption for MCPOAuthToken

func (c *Client) encryptMCPOAuthToken(ctx context.Context, token *types.MCPOAuthToken) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[mcpOAuthTokenGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		b    []byte
		err  error
		errs []error

		dataCtx = mcpOAuthTokenCtx(token)
	)
	if b, err = transformer.TransformToStorage(ctx, []byte(token.AccessToken), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		token.AccessToken = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(token.RefreshToken), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		token.RefreshToken = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(token.ClientID), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		token.ClientID = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(token.ClientSecret), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		token.ClientSecret = base64.StdEncoding.EncodeToString(b)
	}

	token.Encrypted = true

	return errors.Join(errs...)
}

func (c *Client) decryptMCPOAuthToken(ctx context.Context, token *types.MCPOAuthToken) error {
	if !token.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[mcpOAuthTokenGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		out, decoded []byte
		n            int
		err          error
		errs         []error

		dataCtx = mcpOAuthTokenCtx(token)
	)

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(token.AccessToken)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(token.AccessToken))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			token.AccessToken = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(token.RefreshToken)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(token.RefreshToken))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			token.RefreshToken = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(token.ClientID)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(token.ClientID))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			token.ClientID = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(token.ClientSecret)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(token.ClientSecret))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			token.ClientSecret = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func mcpOAuthTokenCtx(token *types.MCPOAuthToken) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s", mcpOAuthTokenGroupResource.String(), token.MCPID))
}

// Encryption for MCPOAuthPendingState

func (c *Client) encryptMCPOAuthPendingState(ctx context.Context, ps *types.MCPOAuthPendingState) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[mcpOAuthPendingStateGroupResource]
	if transformer == nil {
		// Fall back to using the token transformer if no specific one is configured
		transformer = c.encryptionConfig.Transformers[mcpOAuthTokenGroupResource]
		if transformer == nil {
			return nil
		}
	}

	var (
		b    []byte
		err  error
		errs []error

		dataCtx = mcpOAuthPendingStateCtx(ps)
	)
	if b, err = transformer.TransformToStorage(ctx, []byte(ps.State), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		ps.State = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(ps.Verifier), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		ps.Verifier = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(ps.ClientID), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		ps.ClientID = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(ps.ClientSecret), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		ps.ClientSecret = base64.StdEncoding.EncodeToString(b)
	}

	ps.Encrypted = true

	return errors.Join(errs...)
}

func (c *Client) decryptMCPOAuthPendingState(ctx context.Context, ps *types.MCPOAuthPendingState) error {
	if !ps.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[mcpOAuthPendingStateGroupResource]
	if transformer == nil {
		transformer = c.encryptionConfig.Transformers[mcpOAuthTokenGroupResource]
		if transformer == nil {
			return nil
		}
	}

	var (
		out, decoded []byte
		n            int
		err          error
		errs         []error

		dataCtx = mcpOAuthPendingStateCtx(ps)
	)

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(ps.State)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(ps.State))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			ps.State = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(ps.Verifier)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(ps.Verifier))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			ps.Verifier = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(ps.ClientID)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(ps.ClientID))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			ps.ClientID = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(ps.ClientSecret)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(ps.ClientSecret))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			ps.ClientSecret = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func mcpOAuthPendingStateCtx(ps *types.MCPOAuthPendingState) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s", mcpOAuthPendingStateGroupResource.String(), ps.MCPID))
}
