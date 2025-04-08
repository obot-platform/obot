package client

import (
	"context"
	"errors"
	"fmt"

	gcontext "github.com/obot-platform/obot/pkg/gateway/context"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type LogoutAllErr struct{}

func (e LogoutAllErr) Error() string {
	return "logout all is not supported in the current configuration"
}

func (c *Client) DeleteSessionsForUser(ctx context.Context, storageClient kclient.Client, identities []types.Identity, sessionID string) error {
	// Logout all sessions is only supported when using PostgreSQL.
	if c.db.WithContext(ctx).Dialector.Name() != "postgres" {
		return LogoutAllErr{}
	}

	logger := gcontext.GetLogger(ctx)
	var errs []error
	for _, identity := range identities {
		if identity.AuthProviderName == "" || identity.AuthProviderNamespace == "" {
			continue
		}

		var ref v1.ToolReference
		if err := storageClient.Get(ctx, kclient.ObjectKey{Namespace: identity.AuthProviderNamespace, Name: identity.AuthProviderName}, &ref); err != nil {
			errs = append(errs, fmt.Errorf("failed to get auth provider %q: %w", identity.AuthProviderName, err))
			continue
		}

		user := identity.ProviderUserID
		if identity.AuthProviderName == "github-auth-provider" && identity.AuthProviderNamespace == system.DefaultNamespace {
			// The GitHub auth provider stores the username as the user ID in the sessions table.
			// This is because of an annoying quirk of the oauth2-proxy code for GitHub,
			// where we do not know the real user ID until after the user has logged in and the session is created,
			// and we have to manually fetch it from the GitHub API.
			// The oauth2-proxy is only aware of the username, which is why that's in the sessions table.
			user = identity.ProviderUsername
		}

		emailHash := hash.String(identity.Email)
		userHash := hash.String(user)

		logger.Debug("deleting sessions for provider", "provider", identity.AuthProviderName)

		if meta, ok := ref.Status.Tool.Metadata["providerMeta"]; ok {
			tablePrefix := gjson.Get(meta, "postgresTablePrefix").String()
			if tablePrefix != "" {
				var err error
				if sessionID != "" {
					err = c.deleteSessionsForUserExceptCurrent(ctx, emailHash, userHash, tablePrefix, sessionID)
				} else {
					err = c.deleteSessionsForUser(ctx, emailHash, userHash, tablePrefix)
				}

				if err != nil {
					errs = append(errs, fmt.Errorf("failed to delete sessions for provider %q: %w", identity.AuthProviderName, err))
				} else {
					logger.Debug("deleted sessions for provider", "provider", identity.AuthProviderName)
				}
			}
		}
	}

	return errors.Join(errs...)
}

func (c *Client) deleteSessionsForUser(ctx context.Context, emailHash, userHash, tablePrefix string) error {
	return c.db.WithContext(ctx).Exec(
		"DELETE FROM "+tablePrefix+"sessions WHERE \"user\" = decode(?, 'hex') AND \"email\" = decode(?, 'hex')",
		userHash,
		emailHash,
	).Error
}

func (c *Client) deleteSessionsForUserExceptCurrent(ctx context.Context, emailHash, userHash, tablePrefix, currentSessionID string) error {
	return c.db.WithContext(ctx).Exec(
		"DELETE FROM "+tablePrefix+"sessions WHERE key NOT LIKE ? AND \"user\" = decode(?, 'hex') AND \"email\" = decode(?, 'hex')",
		currentSessionID+"%",
		userHash,
		emailHash,
	).Error
}
