package types

import (
	"strings"
	"testing"
)

func TestValidateTunnelName(t *testing.T) {
	for _, name := range []string{"a", "office", "home-lab", "tunnel-123"} {
		t.Run("valid "+name, func(t *testing.T) {
			if err := ValidateTunnelName(name); err != nil {
				t.Fatalf("ValidateTunnelName(%q) error = %v", name, err)
			}
		})
	}

	for _, name := range []string{"", "Office", "home_lab", "-office", "office-", strings.Repeat("a", 64)} {
		t.Run("invalid "+name, func(t *testing.T) {
			if err := ValidateTunnelName(name); err == nil {
				t.Fatalf("ValidateTunnelName(%q) returned no error", name)
			}
		})
	}
}
