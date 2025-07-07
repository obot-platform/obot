package client

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var mcpOAuthTokenGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "mcpoauthtokens",
}

func (c *Client) GetMCPOAuthToken(ctx context.Context, mcpID string) (*types.MCPOAuthToken, error) {
	token := new(types.MCPOAuthToken)
	err := c.db.WithContext(ctx).Where("mcp_id = ?", mcpID).First(token).Error
	if err != nil {
		return nil, err
	}

	if err = c.decryptMCPOAuthToken(ctx, token); err != nil {
		return nil, fmt.Errorf("failed to decrypt token: %w", err)
	}

	return token, nil
}

func (c *Client) GetMCPOAuthTokenByState(ctx context.Context, state string) (*types.MCPOAuthToken, error) {
	token := new(types.MCPOAuthToken)
	err := c.db.WithContext(ctx).Where("hashed_state = ?", fmt.Sprintf("%x", sha256.Sum256([]byte(state)))).First(token).Error
	if err != nil {
		return nil, err
	}

	if err = c.decryptMCPOAuthToken(ctx, token); err != nil {
		return nil, fmt.Errorf("failed to decrypt token: %w", err)
	}

	return token, nil
}

func (c *Client) ReplaceMCPOAuthToken(ctx context.Context, mcpID, state, verifier string, oauthConf *oauth2.Config, token *oauth2.Token) error {
	t := &types.MCPOAuthToken{
		MCPID:        mcpID,
		State:        state,
		Verifier:     verifier,
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
	}

	if state != "" {
		t.HashedState = &[]string{fmt.Sprintf("%x", sha256.Sum256([]byte(state)))}[0]
	}

	if err := c.encryptMCPOAuthToken(ctx, t); err != nil {
		return fmt.Errorf("failed to encrypt token: %w", err)
	}

	return c.db.WithContext(ctx).Save(t).Error
}

func (c *Client) DeleteMCPOAuthToken(ctx context.Context, mcpID string) error {
	if err := c.db.WithContext(ctx).Delete(&types.MCPOAuthToken{MCPID: mcpID}).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

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
	if b, err = transformer.TransformToStorage(ctx, []byte(token.State), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		token.State = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(token.Verifier), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		token.Verifier = base64.StdEncoding.EncodeToString(b)
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

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(token.State)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(token.State))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			token.State = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(token.Verifier)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(token.Verifier))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			token.Verifier = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func mcpOAuthTokenCtx(token *types.MCPOAuthToken) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s", mcpOAuthTokenGroupResource.String(), token.MCPID))
}
