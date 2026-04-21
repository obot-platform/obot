package client

import (
	"context"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"gorm.io/gorm"
)

func TestCreateAndValidateServiceAccountAPIKey(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	created, err := c.CreateServiceAccountAPIKey(ctx, serviceaccounts.NetworkPolicyProvider, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to create service account key: %v", err)
	}

	got, err := c.ValidateStorageServiceAccountToken(ctx, created.Token)
	if err != nil {
		t.Fatalf("failed to validate service account key: %v", err)
	}

	if got.ServiceAccountName != serviceaccounts.NetworkPolicyProvider {
		t.Fatalf("expected service account %q, got %q", serviceaccounts.NetworkPolicyProvider, got.ServiceAccountName)
	}
}

func TestValidateServiceAccountAPIKeyRejectsRetiredKey(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	created, err := c.CreateServiceAccountAPIKey(ctx, serviceaccounts.NetworkPolicyProvider, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to create service account key: %v", err)
	}

	retireAt := time.Now().UTC().Add(-time.Minute)
	if err := c.db.WithContext(ctx).
		Model(&types.ServiceAccountAPIKey{}).
		Where("id = ?", created.ID).
		Update("retire_after", retireAt).Error; err != nil {
		t.Fatalf("failed to retire service account key: %v", err)
	}

	if _, err := c.ValidateStorageServiceAccountToken(ctx, created.Token); err == nil {
		t.Fatal("expected retired key to be rejected")
	}
}

func TestDeleteExpiredServiceAccountAPIKeys(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	created, err := c.CreateServiceAccountAPIKey(ctx, serviceaccounts.NetworkPolicyProvider, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to create service account key: %v", err)
	}

	retireAt := time.Now().UTC().Add(-time.Minute)
	if err := c.db.WithContext(ctx).
		Model(&types.ServiceAccountAPIKey{}).
		Where("id = ?", created.ID).
		Update("retire_after", retireAt).Error; err != nil {
		t.Fatalf("failed to retire service account key: %v", err)
	}

	if err := c.DeleteExpiredServiceAccountAPIKeys(ctx, serviceaccounts.NetworkPolicyProvider, time.Now().UTC()); err != nil {
		t.Fatalf("failed to delete expired keys: %v", err)
	}

	var count int64
	if err := c.db.WithContext(ctx).
		Model(&types.ServiceAccountAPIKey{}).
		Where("service_account_name = ?", serviceaccounts.NetworkPolicyProvider).
		Count(&count).Error; err != nil {
		t.Fatalf("failed to count service account keys: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected all expired keys to be deleted, got %d", count)
	}
}

func TestRetireOtherServiceAccountAPIKeys(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	first, err := c.CreateServiceAccountAPIKey(ctx, serviceaccounts.NetworkPolicyProvider, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatalf("failed to create first key: %v", err)
	}
	second, err := c.CreateServiceAccountAPIKey(ctx, serviceaccounts.NetworkPolicyProvider, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to create second key: %v", err)
	}

	retireAt := time.Now().UTC().Add(time.Hour)
	if err := c.RetireOtherServiceAccountAPIKeys(ctx, serviceaccounts.NetworkPolicyProvider, second.ID, retireAt); err != nil {
		t.Fatalf("failed to retire older keys: %v", err)
	}

	var retired types.ServiceAccountAPIKey
	if err := c.db.WithContext(ctx).Where("id = ?", first.ID).First(&retired).Error; err != nil {
		t.Fatalf("failed to reload retired key: %v", err)
	}
	if retired.RetireAfter == nil || !retired.RetireAfter.Equal(retireAt) {
		t.Fatalf("expected first key to retire at %v, got %v", retireAt, retired.RetireAfter)
	}

	var current types.ServiceAccountAPIKey
	if err := c.db.WithContext(ctx).Where("id = ?", second.ID).First(&current).Error; err != nil {
		t.Fatalf("failed to reload kept key: %v", err)
	}
	if current.RetireAfter != nil {
		t.Fatalf("expected kept key to remain active, got retire_after %v", current.RetireAfter)
	}
}

func TestValidateServiceAccountAPIKeyRejectsUnknownToken(t *testing.T) {
	c := newTestClient(t)
	if _, err := c.ValidateStorageServiceAccountToken(context.Background(), "osa1.bad.token"); err == nil {
		t.Fatal("expected unknown token to be rejected")
	} else if err != gorm.ErrRecordNotFound {
		t.Fatalf("expected gorm.ErrRecordNotFound, got %v", err)
	}
}
