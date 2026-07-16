package localauth

import (
	"context"
	"fmt"
	"net/mail"

	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
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

// CreateUser creates a local user with the given email and plaintext password.
func (p *Provider) CreateUser(ctx context.Context, email, password string) (*types.LocalAuthUser, error) {
	parsed, err := mail.ParseAddress(client.NormalizeEmail(email))
	if err != nil {
		return nil, InvalidUserError{message: "a valid email address is required"}
	}
	// Use the bare address from the parse, not the raw input: mail.ParseAddress accepts
	// display-name forms like "Name <a@b>", and storing that whole string as the login email
	// would make the account impossible to sign in to. Re-normalize since parsing preserves case.
	email = client.NormalizeEmail(parsed.Address)

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

	user, err := p.gatewayClient.CreateLocalAuthUser(ctx, email, passwordHash)
	if err != nil {
		return nil, err
	}

	log.Infof("Created local auth user: id=%d", user.ID)

	return user, nil
}

// SetPassword sets a local user's password, which also signs them out everywhere.
func (p *Provider) SetPassword(ctx context.Context, id uint, password string) error {
	passwordHash, err := hashUserPassword(password)
	if err != nil {
		return err
	}

	if err := p.gatewayClient.SetLocalAuthUserPassword(ctx, id, passwordHash); err != nil {
		return err
	}

	log.Infof("Reset password for local auth user: id=%d", id)

	return nil
}

// DeleteUser removes a local user and their sessions. It does not delete the Obot user that the
// local user logged in as: that is managed from the Users page like any other user.
func (p *Provider) DeleteUser(ctx context.Context, id uint) error {
	if err := p.gatewayClient.DeleteLocalAuthUser(ctx, id); err != nil {
		return err
	}

	log.Infof("Deleted local auth user: id=%d", id)

	return nil
}

func hashUserPassword(password string) (string, error) {
	if len(password) < MinPasswordLength {
		return "", InvalidUserError{message: fmt.Sprintf("password must be at least %d characters", MinPasswordLength)}
	}

	return HashPassword(password)
}
