package principal

import (
	"testing"

	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestAPIKeyAttribution(t *testing.T) {
	keyUser := &kuser.DefaultInfo{Extra: map[string][]string{
		APIKeyIDExtra:   {"42"},
		APIKeyNameExtra: {"CLI token"},
	}}

	got, ok := APIKeyAttributionFromUser(keyUser)
	if !ok {
		t.Fatal("expected API-key attribution")
	}
	if got.ID != 42 || got.Name != "CLI token" {
		t.Fatalf("attribution = %#v, want ID 42 and name CLI token", got)
	}

	unnamed := &kuser.DefaultInfo{Extra: map[string][]string{APIKeyIDExtra: {"7"}}}
	got, ok = APIKeyAttributionFromUser(unnamed)
	if !ok || got.ID != 7 || got.Name != "" {
		t.Fatalf("unnamed attribution = %#v, %v; want ID 7 and empty name", got, ok)
	}

	for _, invalid := range []kuser.Info{
		nil,
		&kuser.DefaultInfo{},
		&kuser.DefaultInfo{Extra: map[string][]string{APIKeyIDExtra: {"not-a-number"}}},
		&kuser.DefaultInfo{Extra: map[string][]string{APIKeyIDExtra: {"0"}}},
	} {
		if got, ok := APIKeyAttributionFromUser(invalid); ok {
			t.Fatalf("invalid principal produced attribution %#v", got)
		}
	}
}

func TestNewAPIKeyAttributionResolvesDisplayName(t *testing.T) {
	named := NewAPIKeyAttribution(42, 7, "CLI token")
	if named.ID != 42 || named.Name != "CLI token" {
		t.Fatalf("named attribution = %#v, want key name", named)
	}

	unnamedKey := NewAPIKeyAttribution(42, 7, "")
	if unnamedKey.ID != 42 || unnamedKey.Name != "ok1-7-42-*****" {
		t.Fatalf("unnamed attribution = %#v, want masked key identifier", unnamedKey)
	}
}

func TestMaskedAPIKeyName(t *testing.T) {
	if got := MaskedAPIKeyName("7", 42); got != "ok1-7-42-*****" {
		t.Fatalf("masked API key name = %q", got)
	}
}
