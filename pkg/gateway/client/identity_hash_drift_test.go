package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
)

func TestEnsureIdentityRepairsUserHashDrift(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()
	verified := true
	existingUser := types.User{
		Username:       "00u-owner",
		HashedUsername: hash.String(""),
		Email:          "owner@example.test",
		HashedEmail:    hash.String(""),
		VerifiedEmail:  &verified,
		Role:           types2.RoleOwner,
		LastActiveDay:  time.Now().UTC().Truncate(24 * time.Hour),
	}
	if err := client.db.WithContext(ctx).Create(&existingUser).Error; err != nil {
		t.Fatalf("failed to create drifted user: %v", err)
	}

	identity := &types.Identity{
		AuthProviderName:      "okta-auth-provider",
		AuthProviderNamespace: "default",
		ProviderUserID:        "00u-owner",
		ProviderUsername:      "00u-owner",
		Email:                 "owner@example.test",
		UserID:                existingUser.ID,
	}

	var (
		resolvedUser *types.User
		created      bool
	)
	err := client.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ensureErr error
		resolvedUser, created, ensureErr = client.ensureIdentity(ctx, tx, identity, "UTC", types2.RoleOwner)
		return ensureErr
	})
	if err != nil {
		t.Fatalf("ensureIdentity returned error: %v", err)
	}
	if created {
		t.Fatal("ensureIdentity created a duplicate user instead of repairing the existing user")
	}
	if got, want := resolvedUser.HashedUsername, hash.String("00u-owner"); got != want {
		t.Fatalf("resolved hashed username = %q, want %q", got, want)
	}
	if got, want := resolvedUser.HashedEmail, hash.String("owner@example.test"); got != want {
		t.Fatalf("resolved hashed email = %q, want %q", got, want)
	}

	var storedUser types.User
	if err := client.db.WithContext(ctx).Where("id = ?", existingUser.ID).First(&storedUser).Error; err != nil {
		t.Fatalf("failed to reload repaired user: %v", err)
	}
	if got, want := storedUser.HashedUsername, hash.String("00u-owner"); got != want {
		t.Fatalf("stored hashed username = %q, want %q", got, want)
	}
	if got, want := storedUser.HashedEmail, hash.String("owner@example.test"); got != want {
		t.Fatalf("stored hashed email = %q, want %q", got, want)
	}
}

func TestUpdateUserUpdatesHashedUsername(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()
	existingUser := types.User{
		Username:       "00u-old",
		HashedUsername: hash.String("00u-old"),
		Email:          "owner@example.test",
		HashedEmail:    hash.String("owner@example.test"),
		Role:           types2.RoleOwner,
	}
	if err := client.db.WithContext(ctx).Create(&existingUser).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	updatedUser, err := client.UpdateUser(ctx, false, &types.User{Username: "00u-new"}, fmt.Sprintf("%d", existingUser.ID))
	if err != nil {
		t.Fatalf("UpdateUser returned error: %v", err)
	}
	if got, want := updatedUser.Username, "00u-new"; got != want {
		t.Fatalf("username = %q, want %q", got, want)
	}
	if got, want := updatedUser.HashedUsername, hash.String("00u-new"); got != want {
		t.Fatalf("hashed username = %q, want %q", got, want)
	}

	var storedUser types.User
	if err := client.db.WithContext(ctx).Where("id = ?", existingUser.ID).First(&storedUser).Error; err != nil {
		t.Fatalf("failed to reload updated user: %v", err)
	}
	if got, want := storedUser.HashedUsername, hash.String("00u-new"); got != want {
		t.Fatalf("stored hashed username = %q, want %q", got, want)
	}
}
