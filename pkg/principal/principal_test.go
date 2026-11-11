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
	for _, tt := range []struct {
		name     string
		owner    Principal
		keyName  string
		wantName string
	}{
		{
			name:     "named user key",
			owner:    User(7),
			keyName:  "CLI token",
			wantName: "CLI token",
		},
		{
			name:     "unnamed user key",
			owner:    User(7),
			wantName: "ok1-7-42-*****",
		},
		{
			name:     "unnamed agent key",
			owner:    Agent(7),
			wantName: "oak1-7-42-*****",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := NewAPIKeyAttribution(42, tt.owner, tt.keyName)
			if got.ID != 42 || got.Name != tt.wantName {
				t.Fatalf("attribution = %#v, want ID 42 and name %q", got, tt.wantName)
			}
		})
	}
}

func TestMaskedAPIKeyName(t *testing.T) {
	for _, tt := range []struct {
		name  string
		owner string
		want  string
	}{
		{
			name:  "user",
			owner: User(7).String(),
			want:  "ok1-7-42-*****",
		},
		{
			name:  "agent",
			owner: Agent(7).String(),
			want:  "oak1-7-42-*****",
		},
		{
			// An owner that is not a principal's encoding is written as is, as it was before agents.
			name:  "not a principal",
			owner: "system",
			want:  "ok1-system-42-*****",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskedAPIKeyName(tt.owner, 42); got != tt.want {
				t.Fatalf("MaskedAPIKeyName(%q, 42) = %q, want %q", tt.owner, got, tt.want)
			}
		})
	}
}
