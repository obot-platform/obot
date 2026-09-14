package client

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestPropertyTransactionRollsBackAllChanges(t *testing.T) {
	client := newTestClient(t)
	ctx := t.Context()

	if _, err := client.SetProperty(ctx, "primary", "old-primary"); err != nil {
		t.Fatalf("seed primary property: %v", err)
	}
	if err := client.db.WithContext(ctx).Exec(`
		CREATE TRIGGER fail_primary_update
		BEFORE UPDATE OF value ON properties
		WHEN OLD.key = 'primary'
		BEGIN
			SELECT RAISE(FAIL, 'forced primary update failure');
		END
	`).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	err := client.Transaction(ctx, func(tx *gorm.DB) error {
		if _, err := client.SetPropertyTx(ctx, tx, "fallback", "community"); err != nil {
			return err
		}
		_, err := client.SetPropertyTx(ctx, tx, "primary", "new-primary")
		return err
	})
	if err == nil {
		t.Fatal("Transaction() error = nil, want forced failure")
	}

	primary, err := client.GetProperty(ctx, "primary")
	if err != nil {
		t.Fatalf("get primary property: %v", err)
	}
	if primary.Value != "old-primary" {
		t.Fatalf("primary property = %q, want old-primary", primary.Value)
	}
	if _, err := client.GetProperty(ctx, "fallback"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("fallback property error = %v, want record not found", err)
	}
}
