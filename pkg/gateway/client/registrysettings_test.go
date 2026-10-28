package client

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

func TestRegistrySettingsSharedAndFailClosed(t *testing.T) {
	c := newTestClient(t)
	replica := &Client{db: c.db}
	if enabled, err := replica.AnonymousRegistryAccessEnabled(t.Context()); err != nil || enabled {
		t.Fatalf("default = %v, %v", enabled, err)
	}

	for _, enabled := range []bool{false, true, true, false} {
		if err := c.SetAnonymousRegistryAccessEnabled(t.Context(), enabled); err != nil {
			t.Fatal(err)
		}

		if got, err := replica.AnonymousRegistryAccessEnabled(t.Context()); err != nil || got != enabled {
			t.Fatalf("replica = %v, %v; want %v", got, err, enabled)
		}
	}

	if _, err := c.SetProperty(t.Context(), registryAnonymousAccessEnabledKey, "corrupt"); err != nil {
		t.Fatal(err)
	}

	if enabled, err := replica.AnonymousRegistryAccessEnabled(t.Context()); err == nil || enabled {
		t.Fatal("malformed property did not fail closed")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if enabled, err := c.AnonymousRegistryAccessEnabled(ctx); err == nil || enabled {
		t.Fatal("failed lookup did not fail closed")
	}

	if err := c.SetAnonymousRegistryAccessEnabled(ctx, true); err == nil {
		t.Fatal("failed write reported success")
	}
}

func TestRegistrySettingsPropertyTimestamps(t *testing.T) {
	c := newTestClient(t)
	if err := c.SetAnonymousRegistryAccessEnabled(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	created, err := c.GetProperty(t.Context(), registryAnonymousAccessEnabledKey)
	if err != nil {
		t.Fatal(err)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("missing property timestamps: %+v", created)
	}

	// Give the existing row an older update time without relying on clock precision.
	oldUpdate := created.UpdatedAt.Add(-time.Hour)
	if err := c.db.WithContext(t.Context()).Model(&types.Property{}).Where("key = ?", registryAnonymousAccessEnabledKey).Update("updated_at", oldUpdate).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.SetAnonymousRegistryAccessEnabled(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	updated, err := c.GetProperty(t.Context(), registryAnonymousAccessEnabledKey)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) || !updated.UpdatedAt.After(oldUpdate) {
		t.Fatalf("unexpected timestamps after update: created=%+v, updated=%+v", created, updated)
	}
}

func TestRegistrySettingsConcurrentCreation(t *testing.T) {
	c := newTestClient(t)

	var wg sync.WaitGroup

	for range 12 {
		wg.Go(func() {
			if err := c.SetAnonymousRegistryAccessEnabled(t.Context(), false); err != nil {
				t.Error(err)
			}
		})
	}

	wg.Wait()

	var count int64
	if err := c.db.WithContext(t.Context()).Model(&types.Property{}).Where("key = ?", registryAnonymousAccessEnabledKey).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("property count = %d, %v", count, err)
	}

	if enabled, err := c.AnonymousRegistryAccessEnabled(t.Context()); err != nil || enabled {
		t.Fatalf("concurrent writes = %v, %v", enabled, err)
	}
}

func TestRegistrySettingsEncryptedProperty(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{propertyGroupResource: testTransformer{}}}
	if err := c.SetAnonymousRegistryAccessEnabled(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	var stored types.Property
	if err := c.db.WithContext(t.Context()).Where("key = ?", registryAnonymousAccessEnabledKey).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.Encrypted || stored.Value == "true" {
		t.Fatal("property was not encrypted")
	}
	replica := &Client{db: c.db, encryptionConfig: c.encryptionConfig}
	if enabled, err := replica.AnonymousRegistryAccessEnabled(t.Context()); err != nil || !enabled {
		t.Fatalf("encrypted property = %v, %v", enabled, err)
	}
	if err := c.db.WithContext(t.Context()).Model(&types.Property{}).Where("key = ?", registryAnonymousAccessEnabledKey).Update("value", "not-base64!").Error; err != nil {
		t.Fatal(err)
	}
	if enabled, err := replica.AnonymousRegistryAccessEnabled(t.Context()); err == nil || enabled {
		t.Fatal("corrupt encrypted property allowed access")
	}
}
