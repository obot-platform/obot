package producttelemetry

import (
	"errors"
	"testing"
	"time"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
)

func TestConsentPersistsTriStateAndChanges(t *testing.T) {
	client := newConsentTestGatewayClient(t)
	consent := NewConsent(client, false)

	value, err := consent.Get(t.Context())
	if err != nil {
		t.Fatalf("Get() undecided error = %v", err)
	}
	if value != nil {
		t.Fatalf("Get() undecided = %v, want nil", *value)
	}

	for _, want := range []bool{false, true, false} {
		if err := consent.Set(t.Context(), want); err != nil {
			t.Fatalf("Set(%t) error = %v", want, err)
		}
		got, err := consent.Get(t.Context())
		if err != nil {
			t.Fatalf("Get() after Set(%t) error = %v", want, err)
		}
		if got == nil || *got != want {
			t.Fatalf("Get() after Set(%t) = %v, want %t", want, got, want)
		}

		property, err := client.GetProperty(t.Context(), consentPropertyKey)
		if err != nil {
			t.Fatalf("GetProperty() after Set(%t) error = %v", want, err)
		}
		if property.Value != map[bool]string{false: "false", true: "true"}[want] {
			t.Fatalf("stored value after Set(%t) = %q", want, property.Value)
		}
	}
}

func TestConsentForceEnabledNeedsNoPersistence(t *testing.T) {
	consent := NewConsent(nil, true)

	got, err := consent.Get(t.Context())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || !*got {
		t.Fatalf("Get() = %v, want true", got)
	}
	if err := consent.Set(t.Context(), false); !errors.Is(err, errConsentForceEnabled) {
		t.Fatalf("Set(false) error = %v, want errConsentForceEnabled", err)
	}
}

func newConsentTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	return newConsentTestGatewayClients(t, 1)[0]
}

func newConsentTestGatewayClients(t *testing.T, count int) []*gatewayclient.Client {
	t.Helper()

	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("create storage services: %v", err)
	}
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("migrate gateway db: %v", err)
	}
	clients := make([]*gatewayclient.Client, 0, count)
	for range count {
		client := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, 10*time.Millisecond, 10, 90, 90, 90, true)
		t.Cleanup(func() { _ = client.Close() })
		clients = append(clients, client)
	}
	return clients
}
