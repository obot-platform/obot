package client

import (
	"context"
	"sync"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestModelProxySettingsSharedAndFailClosed(t *testing.T) {
	c := newTestClient(t)
	replica := &Client{db: c.db}
	if enabled, err := replica.ModelProxyEnabled(t.Context()); err != nil || !enabled {
		t.Fatalf("default = %v, %v", enabled, err)
	}

	for _, enabled := range []bool{false, true, true, false} {
		if err := c.SetModelProxyEnabled(t.Context(), enabled); err != nil {
			t.Fatal(err)
		}

		if got, err := replica.ModelProxyEnabled(t.Context()); err != nil || got != enabled {
			t.Fatalf("replica = %v, %v; want %v", got, err, enabled)
		}
	}

	if _, err := c.SetProperty(t.Context(), modelProxyEnabledKey, "corrupt"); err != nil {
		t.Fatal(err)
	}

	if enabled, err := replica.ModelProxyEnabled(t.Context()); err == nil || enabled {
		t.Fatal("malformed property did not fail closed")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if enabled, err := c.ModelProxyEnabled(ctx); err == nil || enabled {
		t.Fatal("failed lookup did not fail closed")
	}

	if err := c.SetModelProxyEnabled(ctx, true); err == nil {
		t.Fatal("failed write reported success")
	}
}

func TestModelProxySettingsConcurrentCreation(t *testing.T) {
	c := newTestClient(t)

	var wg sync.WaitGroup

	for range 12 {
		wg.Go(func() {
			if err := c.SetModelProxyEnabled(t.Context(), false); err != nil {
				t.Error(err)
			}
		})
	}

	wg.Wait()

	var count int64
	if err := c.db.WithContext(t.Context()).Model(&types.Property{}).Where("key = ?", modelProxyEnabledKey).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("property count = %d, %v", count, err)
	}

	if enabled, err := c.ModelProxyEnabled(t.Context()); err != nil || enabled {
		t.Fatalf("concurrent writes = %v, %v", enabled, err)
	}
}
