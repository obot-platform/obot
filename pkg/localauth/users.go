package localauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
)

// InvalidUserError is returned when a local user cannot be created or updated as requested.
// It is a user error, not a server error: the message is safe to return to the caller.
type InvalidUserError struct {
	message string
}

func (e InvalidUserError) Error() string {
	return e.message
}

func (p *Provider) Users(ctx context.Context) ([]types.LocalAuthUser, error) {
	return p.gatewayClient.LocalAuthUsers(ctx)
}

func (p *Provider) GetUser(ctx context.Context, id uint) (*types.LocalAuthUser, error) {
	return p.gatewayClient.LocalAuthUserByID(ctx, id)
}

// CreateUser creates a local user with the given email and plaintext password.
func normalizeUserEmail(email string) (string, error) {
	parsed, err := mail.ParseAddress(client.NormalizeEmail(email))
	if err != nil {
		return "", InvalidUserError{message: "a valid email address is required"}
	}
	// Use the bare address from the parse, not the raw input: mail.ParseAddress accepts
	// display-name forms like "Name <a@b>", and storing that whole string as the login email
	// would make the account impossible to sign in to. Re-normalize since parsing preserves case.
	return client.NormalizeEmail(parsed.Address), nil
}

// CreateUser creates a local user with the given email and plaintext password.
func (p *Provider) CreateUser(ctx context.Context, email, password string, requirePasswordChange bool) (*types.LocalAuthUser, error) {
	email, err := normalizeUserEmail(email)
	if err != nil {
		return nil, err
	}

	allowed, err := p.emailDomainAllowed(ctx, email)
	if err != nil {
		return nil, err
	} else if !allowed {
		return nil, InvalidUserError{message: fmt.Sprintf("email %q is not in the provider's allowed email domains", email)}
	}

	passwordHash, err := hashUserPassword(password)
	if err != nil {
		return nil, err
	}

	user, err := p.gatewayClient.CreateLocalAuthUser(ctx, email, passwordHash, requirePasswordChange)
	if err != nil {
		return nil, err
	}

	slog.Info("Created local auth user", "id", user.ID)

	return user, nil
}

// SetPassword sets a local user's password, which also signs them out everywhere.
func (p *Provider) SetPassword(ctx context.Context, id uint, password string, requirePasswordChange bool) error {
	passwordHash, err := hashUserPassword(password)
	if err != nil {
		return err
	}

	if err := p.gatewayClient.SetLocalAuthUserPassword(ctx, id, passwordHash, requirePasswordChange); err != nil {
		return err
	}

	slog.Info("Reset password for local auth user", "id", id)

	return nil
}

// ChangePassword lets an authenticated local user choose their own password. The current session
// is preserved, every other session is invalidated, and the login restriction is cleared.
func (p *Provider) ChangePassword(ctx context.Context, id uint, password, currentSessionID string) error {
	passwordHash, err := hashUserPassword(password)
	if err != nil {
		return err
	}

	if err := p.gatewayClient.CompleteLocalAuthUserPasswordChange(ctx, id, passwordHash, currentSessionID); err != nil {
		return err
	}

	slog.Info("Changed password for local auth user", "id", id)
	return nil
}

// EnsureInitialOwner configures the local provider and creates the initial owner exactly once. A
// pending setup token may be rotated, but a completed account is never rearmed from the environment.
func (p *Provider) EnsureInitialOwner(ctx context.Context, email, setupToken string, setupTokenExpiresAt time.Time) error {
	email, err := normalizeUserEmail(email)
	if err != nil {
		return err
	}
	if len(setupToken) < 32 {
		return InvalidUserError{message: "the initial owner setup token must be at least 32 characters"}
	}

	cred, credErr := p.gatewayClient.RevealCredential(ctx, []string{ProviderName, system.GenericAuthProviderCredentialContext}, ProviderName)
	if credErr != nil {
		if !errors.As(credErr, &client.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to read local auth provider configuration: %w", credErr)
		}
		_, domain, _ := strings.Cut(email, "@")
		if err := p.gatewayClient.UpsertCredential(ctx, types.Credential{
			Context: ProviderName,
			Name:    ProviderName,
			Secrets: map[string]string{EmailDomainsEnvVar: domain},
		}); err != nil {
			return fmt.Errorf("failed to configure local auth provider: %w", err)
		}
	}

	existing, lookupErr := p.gatewayClient.LocalAuthUserByEmail(ctx, email)
	if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to look up initial local auth owner: %w", lookupErr)
	}
	if lookupErr == nil && existing.SetupTokenHash == "" {
		// Setup already completed. Environment values are not password-reset credentials.
		slog.Warn("Initial local auth owner provisioning skipped because the email already belongs to a local account", "id", existing.ID)
		if credErr == nil && !emailDomainAllowed(cred.Secrets[EmailDomainsEnvVar], email) {
			slog.Warn("Initial local auth owner is outside the local provider's allowed email domains and cannot sign in until they are widened", "id", existing.ID)
		}
		return nil
	}

	// Only the accounts this call may still create or re-arm are rejected, so narrowing the allowed
	// domains later cannot fail startup for an owner already in use.
	if credErr == nil && !emailDomainAllowed(cred.Secrets[EmailDomainsEnvVar], email) {
		return InvalidUserError{message: fmt.Sprintf("initial owner email %q is not in the local provider's allowed email domains", email)}
	}

	if lookupErr == nil {
		newHash := hash.String(setupToken)
		if existing.SetupTokenHash != newHash {
			if err := p.gatewayClient.RefreshLocalAuthUserSetupToken(ctx, existing.ID, newHash, setupTokenExpiresAt); err != nil {
				return fmt.Errorf("failed to rotate initial owner setup token: %w", err)
			}
			slog.Info("Rotated setup token for initial local auth owner", "id", existing.ID)
		} else if existing.SetupTokenExpiresAt == nil || !existing.SetupTokenExpiresAt.After(time.Now()) {
			// The expiration is absolute: a restart with the same environment value must not revive
			// an expired token. Reissuing requires a freshly generated one.
			slog.Warn("Initial local auth owner setup token is expired; configure a new token value to reissue it", "id", existing.ID)
		}
		return nil
	}

	// The initial account has no usable password. It can only be activated with the setup token,
	// after which its restricted session must set a real one.
	randomPassword := make([]byte, 32)
	if _, err := rand.Read(randomPassword); err != nil {
		return fmt.Errorf("failed to generate disabled initial password: %w", err)
	}
	passwordHash, err := HashPassword(base64.RawURLEncoding.EncodeToString(randomPassword))
	if err != nil {
		return err
	}

	user, err := p.gatewayClient.CreateInitialLocalAuthUser(ctx, email, passwordHash, hash.String(setupToken), setupTokenExpiresAt)
	if errors.Is(err, client.ErrLocalAuthUserExists) {
		// Another replica won the create race, so reconcile its row rather than failing startup.
		existing, getErr := p.gatewayClient.LocalAuthUserByEmail(ctx, email)
		if getErr != nil {
			return fmt.Errorf("failed to load concurrently created initial owner: %w", getErr)
		}
		if existing.SetupTokenHash == "" {
			slog.Warn("Initial local auth owner provisioning skipped because a concurrently found account is already active", "id", existing.ID)
			return nil
		}
		newHash := hash.String(setupToken)
		if existing.SetupTokenHash != newHash {
			if refreshErr := p.gatewayClient.RefreshLocalAuthUserSetupToken(ctx, existing.ID, newHash, setupTokenExpiresAt); refreshErr != nil {
				return fmt.Errorf("failed to reconcile concurrently created initial owner: %w", refreshErr)
			}
		}
		return nil
	}
	if err != nil {
		return err
	}
	slog.Info("Created initial local auth owner awaiting activation", "id", user.ID)
	return nil
}

// DeleteUser removes a local user and their sessions. It does not delete the Obot user that the
// local user logged in as: that is managed from the Users page like any other user.
func (p *Provider) DeleteUser(ctx context.Context, id uint) error {
	if err := p.gatewayClient.DeleteLocalAuthUser(ctx, id); err != nil {
		return err
	}

	slog.Info("Deleted local auth user", "id", id)

	return nil
}

func hashUserPassword(password string) (string, error) {
	if len(password) < minPasswordLength {
		return "", InvalidUserError{message: fmt.Sprintf("password must be at least %d characters", minPasswordLength)}
	}

	return HashPassword(password)
}
