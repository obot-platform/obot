package client

import (
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
)

func TestInitialLocalAuthSetupCanResumeUntilPasswordIsSet(t *testing.T) {
	c := newTestClient(t)
	setupTokenHash := hash.String("a-high-entropy-owner-setup-token-value")
	user, err := c.CreateInitialLocalAuthUser(t.Context(), "owner@example.com", "disabled-password-hash", setupTokenHash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("creating initial owner: %v", err)
	}

	firstSession := hash.String("first-session")
	if _, err := c.ActivateLocalAuthUser(t.Context(), setupTokenHash, firstSession, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("activating first setup session: %v", err)
	}
	secondSession := hash.String("second-session")
	if _, err := c.ActivateLocalAuthUser(t.Context(), setupTokenHash, secondSession, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("resuming setup from the same link: %v", err)
	}

	if err := c.CompleteLocalAuthUserPasswordChange(t.Context(), user.ID, "chosen-password-hash", secondSession); err != nil {
		t.Fatalf("completing password setup: %v", err)
	}
	if _, _, err := c.LocalAuthSession(t.Context(), firstSession); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("first setup session error = %v, want record not found", err)
	}
	if _, currentUser, err := c.LocalAuthSession(t.Context(), secondSession); err != nil {
		t.Fatalf("loading preserved session: %v", err)
	} else if currentUser.RequirePasswordChange {
		t.Fatal("completed user's session still requires a password change")
	}
	if _, err := c.ActivateLocalAuthUser(t.Context(), setupTokenHash, hash.String("third-session"), time.Now().Add(time.Hour)); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("reusing completed setup link error = %v, want record not found", err)
	}
	if err := c.RefreshLocalAuthUserSetupToken(t.Context(), user.ID, hash.String("replacement-token"), time.Now().Add(time.Hour)); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("rearming completed account error = %v, want record not found", err)
	}
}

func TestRefreshLocalAuthSetupTokenRevokesSetupSessions(t *testing.T) {
	c := newTestClient(t)
	oldHash := hash.String("old-high-entropy-owner-setup-token")
	user, err := c.CreateInitialLocalAuthUser(t.Context(), "owner@example.com", "disabled-password-hash", oldHash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("creating initial owner: %v", err)
	}
	sessionID := hash.String("setup-session")
	if _, err := c.ActivateLocalAuthUser(t.Context(), oldHash, sessionID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("activating setup session: %v", err)
	}

	newHash := hash.String("new-high-entropy-owner-setup-token")
	if err := c.RefreshLocalAuthUserSetupToken(t.Context(), user.ID, newHash, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("rotating setup token: %v", err)
	}
	if _, _, err := c.LocalAuthSession(t.Context(), sessionID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("old setup session error = %v, want record not found", err)
	}
	if _, err := c.ActivateLocalAuthUser(t.Context(), oldHash, hash.String("old-token-session"), time.Now().Add(time.Hour)); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("old setup token error = %v, want record not found", err)
	}
	if _, err := c.ActivateLocalAuthUser(t.Context(), newHash, hash.String("new-token-session"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("activating rotated setup token: %v", err)
	}
}

func TestAdminPasswordResetDisarmsInitialSetupLink(t *testing.T) {
	c := newTestClient(t)
	setupTokenHash := hash.String("a-high-entropy-owner-setup-token-value")
	user, err := c.CreateInitialLocalAuthUser(t.Context(), "owner@example.com", "disabled-password-hash", setupTokenHash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("creating initial owner: %v", err)
	}

	// Administrator-created/reset passwords normally remain flagged for a required change. That
	// must not leave the independently delivered setup link valid.
	if err := c.SetLocalAuthUserPassword(t.Context(), user.ID, "administrator-reset-hash", true); err != nil {
		t.Fatalf("resetting password: %v", err)
	}
	if _, err := c.ActivateLocalAuthUser(t.Context(), setupTokenHash, hash.String("setup-session"), time.Now().Add(time.Hour)); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("using setup link after administrator reset error = %v, want record not found", err)
	}
}

func TestOnlyFirstSetupSessionCanCompletePasswordChange(t *testing.T) {
	c := newTestClient(t)
	setupTokenHash := hash.String("a-high-entropy-owner-setup-token-value")
	user, err := c.CreateInitialLocalAuthUser(t.Context(), "owner@example.com", "disabled-password-hash", setupTokenHash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("creating initial owner: %v", err)
	}

	firstSession := hash.String("first-racing-session")
	secondSession := hash.String("second-racing-session")
	if _, err := c.ActivateLocalAuthUser(t.Context(), setupTokenHash, firstSession, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("activating first session: %v", err)
	}
	if _, err := c.ActivateLocalAuthUser(t.Context(), setupTokenHash, secondSession, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("activating second session: %v", err)
	}

	if err := c.CompleteLocalAuthUserPasswordChange(t.Context(), user.ID, "first-password-hash", firstSession); err != nil {
		t.Fatalf("completing from first session: %v", err)
	}
	if err := c.CompleteLocalAuthUserPasswordChange(t.Context(), user.ID, "second-password-hash", secondSession); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("completing from second session error = %v, want record not found", err)
	}
	got, err := c.LocalAuthUserByID(t.Context(), user.ID)
	if err != nil {
		t.Fatalf("reading completed user: %v", err)
	}
	if got.PasswordHash != "first-password-hash" {
		t.Fatalf("password hash = %q, want first completion's hash", got.PasswordHash)
	}
}
